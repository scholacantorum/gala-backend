package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/guest"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServePayments handles processing payments.
func ServePayments(w *request.ResponseWriter, r *request.Request) {
	type payBodyType struct {
		PayerID      db.ID   `json:"payer"`
		PurchaseIDs  []db.ID `json:"purchases"`
		StripeSource string  `json:"stripeSource"`
		CardSource   string  `json:"cardSource"`
		OtherMethod  string  `json:"otherMethod"`
		Total        int     `json:"total"`
	}
	var (
		body        payBodyType
		payer       *model.Guest
		purchase    *model.Purchase
		purchases   []*model.Purchase
		total       int
		status      int
		description string
		je          model.JournalEntry
		onum        int
		err         error
		now         = time.Now().Format(time.RFC3339)
		seenPID     = map[db.ID]bool{}
	)
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("ServePayments JSON decode %s", err)
		goto ERROR
	}
	if payer = model.FetchGuest(r.Tx, body.PayerID); payer == nil {
		log.Print("ServePayments no such payer")
		goto ERROR
	}
	switch {
	case body.StripeSource != "":
		if body.StripeSource != payer.StripeSource || body.CardSource != "" || body.OtherMethod != "" {
			log.Print("ServePayments StripeSource error")
			goto ERROR
		}
	case body.CardSource != "":
		if body.OtherMethod != "" || payer.Email == "" {
			log.Print("ServePayments CardSource error")
			goto ERROR
		}
	case body.OtherMethod == "":
		log.Print("ServePayments no payment type")
		goto ERROR
	}
	if len(body.PurchaseIDs) == 0 {
		log.Print("ServePayments no purchase IDs")
		goto ERROR
	}
	purchases = make([]*model.Purchase, len(body.PurchaseIDs))
	for idx, pid := range body.PurchaseIDs {
		if seenPID[pid] {
			log.Print("ServePayment duplicate purchase IDs")
			goto ERROR
		}
		seenPID[pid] = true
		if purchase = model.FetchPurchase(r.Tx, pid); purchase == nil {
			log.Print("ServePayment no such purchase")
			goto ERROR
		}
		if purchase.PaymentTimestamp != "" || purchase.PayerID != body.PayerID {
			log.Print("ServePayment purchase already paid or wrong payer")
			goto ERROR
		}
		purchases[idx] = purchase
		total += purchase.Amount
	}
	if body.Total != total {
		log.Print("ServePayment total mismatch")
		goto ERROR
	}
	switch {
	case body.StripeSource != "":
		onum, status, description = chargeExistingCard(payer, "cardOnFile", total)
	case body.CardSource != "":
		onum, status, description = chargeNewCard(r, &je, payer, body.CardSource, total)
	default:
		onum, status, description = 0, 200, body.OtherMethod
	}
	if status != 200 {
		log.Printf("ServePayment charge failed %d %s", status, description)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		fmt.Fprint(w, description)
		return
	}
	for _, purchase = range purchases {
		purchase.PaymentDescription = description
		purchase.ScholaOrder = onum
		purchase.PaymentTimestamp = now
		purchase.Save(r.Tx, &je)
	}
	je.MarkGuest(payer.ID)
	journal.Log(r, &je)
	if onum != 0 {
		sendChargeReceipt(r, onum, payer, purchases)
	}
	w.CommitNoContent(r)
	return

ERROR:
	w.WriteHeader(http.StatusInternalServerError)
}

func chargeNewCard(
	r *request.Request, je *model.JournalEntry, payer *model.Guest, cardSource string, total int,
) (onum, status int, description string) {
	if payer.StripeCustomer != "" {
		status, description = guest.UpdateCustomer(payer, payer.Name, payer.Email, cardSource)
	} else {
		status, description = guest.CreateCustomer(payer, payer.Name, payer.Email, cardSource)
	}
	if status != 200 {
		return 0, status, description
	}
	payer.UseCard = true
	payer.Save(r.Tx, je)
	return chargeExistingCard(payer, "cardEntry", total)
}

func chargeExistingCard(payer *model.Guest, payType string, total int) (onum, status int, description string) {
	type responsedata struct {
		Error    string `json:"error"`
		ID       int    `json:"id"`
		Payments []struct {
			Method string `json:"method"`
		} `json:"payments"`
	}
	var (
		resp   *http.Response
		rdata  responsedata
		err    error
		params = make(url.Values)
	)
	params.Set("auth", config.Get("ordersAPIKey"))
	params.Set("source", "gala")
	params.Set("name", payer.Name)
	params.Set("email", payer.Email)
	params.Set("address", payer.Address)
	params.Set("city", payer.City)
	params.Set("state", payer.State)
	params.Set("zip", payer.Zip)
	params.Set("phone", payer.Phone)
	params.Set("customer", payer.StripeCustomer)
	params.Set("line1.product", "gala-purchase")
	params.Set("line1.quantity", "1")
	params.Set("line1.price", strconv.Itoa(total))
	params.Set("payment1.type", "card")
	params.Set("payment1.subtype", payType)
	params.Set("payment1.method", payer.StripeSource)
	params.Set("payment1.amount", strconv.Itoa(total))
	resp, err = http.PostForm(config.Get("ordersURL")+"/payapi/order", params)
	if err != nil {
		log.Printf("Post gala purchase to orders failed: %s", err)
		return 0, 500, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("Post gala purchase to orders failed: %d %s", resp.StatusCode, resp.Status)
		by, _ := io.ReadAll(resp.Body)
		return 0, resp.StatusCode, string(by)
	}
	if err = json.NewDecoder(resp.Body).Decode(&rdata); err != nil {
		log.Printf("Can't parse orders response: %s", err)
		return 0, 500, err.Error()
	}
	if rdata.Error != "" {
		return 0, 400, rdata.Error
	}
	if len(rdata.Payments) != 1 {
		log.Printf("Order has %d payments, expected 1", len(rdata.Payments))
		return 0, 500, "wrong payment count"
	}
	return rdata.ID, 200, rdata.Payments[0].Method
}

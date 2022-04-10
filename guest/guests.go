package guest

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServeGuests handles requests starting with /guests.
func ServeGuests(w *request.ResponseWriter, r *request.Request) {
	var (
		head string
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		serveGuests(w, r)
	case "checkin-forms":
		serveCheckinForms(w, r)
	case "program-labels":
		serveProgramLabels(w, r)
	case "receipts":
		serveReceipts(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// serveGuest handles requests to /guests.
func serveGuests(w *request.ResponseWriter, r *request.Request) {
	switch r.Method {
	case http.MethodPost:
		addGuest(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// addGuest handles a POST /guests request.
func addGuest(w *request.ResponseWriter, r *request.Request) {
	type addGuestBody struct {
		model.Guest
		Ticket     string
		NumGuests  int
		CardSource string
		PayingFor  []db.ID
	}
	var (
		body          addGuestBody
		je            model.JournalEntry
		purchase      model.Purchase
		pfid          db.ID
		err           error
		bodyPayingFor = map[db.ID]bool{}
	)

	// Gather all of the data.
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("saveGuest JSON decode %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.PartyID != 0 || (body.CardSource != "" && (body.Email == "" || body.PayerID != 0)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.PayerID != 0 { // Make sure the proposed payer exists and no one is paying for them.
		if payer := model.FetchGuest(r.Tx, body.PayerID); payer == nil || payer.PayerID != 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	for _, pfid = range body.PayingFor {
		// Make sure the proposed payee exists.
		pf := model.FetchGuest(r.Tx, pfid)
		if pf == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Make sure they aren't paying for someone else.
		model.FetchGuests(r.Tx, func(g *model.Guest) { bodyPayingFor[pfid] = false }, `payer=?`, pfid)
		if !bodyPayingFor[pfid] {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	if body.Address == "" || body.City == "" || body.State == "" || body.Zip == "" {
		body.Address, body.City, body.State, body.Zip = "", "", "", "" // all or none
	}
	body.Sortname = sortname(body.Name)
	body.UseCard = body.CardSource != ""

	// If we have a card for the new guest, create a Stripe customer with
	// that card.
	if body.CardSource != "" {
		var params = make(url.Values)
		params.Set("auth", config.Get("ordersAPIKey"))
		params.Set("name", body.Name)
		params.Set("email", body.Email)
		params.Set("card", body.CardSource)
		resp, err := http.PostForm(config.Get("ordersURL")+"/payapi/customer", params)
		if err != nil {
			log.Printf("error creating customer: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			return
		}
		var response struct {
			Customer    string `json:"customer"`
			Method      string `json:"method"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			log.Printf("error creating customer: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
		body.StripeCustomer = response.Customer
		body.StripeSource = response.Method
		body.StripeDescription = response.Description
	}

	// Update the database and generate the journal.
	body.Save(r.Tx, &je)
	for _, pfid = range body.PayingFor {
		g := model.FetchGuest(r.Tx, pfid)
		g.PayerID = body.ID
		g.Save(r.Tx, &je)
	}
	purchase = model.Purchase{
		GuestID: body.ID,
		PayerID: body.ID,
		ItemID:  1,
		Amount:  model.FetchItem(r.Tx, 1).Amount,
	}
	if body.Ticket != "" {
		purchase.PaymentTimestamp = time.Now().Format(time.RFC3339)
		purchase.PaymentDescription = body.Ticket
	}
	purchase.Save(r.Tx, &je)

	// If they have guests, register them too.
	for i := 1; i <= body.NumGuests; i++ {
		var g = model.Guest{
			Name:     fmt.Sprintf("%s Guest #%d", body.Name, i),
			PartyID:  body.PartyID,
			Requests: body.Requests,
			Sortname: fmt.Sprintf("%s Guest #%d", body.Sortname, i),
		}
		g.Save(r.Tx, &je)
		purchase.GuestID = g.ID
		purchase.ID = 0
		purchase.Save(r.Tx, &je)
	}
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

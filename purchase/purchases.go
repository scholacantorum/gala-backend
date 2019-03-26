package purchase

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServePurchases handles requests starting with /purchases.
func ServePurchases(w *request.ResponseWriter, r *request.Request) {
	var (
		head string
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		servePurchases(w, r)
	case "export":
		serveExportPurchases(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// servePurchase handles requests to /purchases.
func servePurchases(w *request.ResponseWriter, r *request.Request) {
	switch r.Method {
	case http.MethodPost:
		addPurchase(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// addPurchase handles a POST /purchases request.
func addPurchase(w *request.ResponseWriter, r *request.Request) {
	var (
		body  model.Purchase
		je    model.JournalEntry
		guest *model.Guest
		payer *model.Guest
		err   error
	)
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("savePurchase JSON decode %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.Amount <= 0 || body.ID != 0 || body.PaymentTimestamp != "" || body.PaymentDescription != "" || body.PayerID != 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if guest = model.FetchGuest(r.Tx, body.GuestID); guest == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if item := model.FetchItem(r.Tx, body.ItemID); item == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if payer = model.FetchGuest(r.Tx, guest.PayerID); payer == nil {
		payer = guest
	}
	body.PayerID = payer.ID
	body.Save(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

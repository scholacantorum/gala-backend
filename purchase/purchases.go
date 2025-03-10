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
	var head string
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		servePurchases(w, r)
	case "export":
		serveExportPurchases(w, r)
	case "winners":
		serveAuctionWinners(w, r)
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
		isFAN bool
		isDup bool
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
	} else if item.Value == 0 {
		isFAN = true
	}
	if payer = model.FetchGuest(r.Tx, guest.PayerID); payer == nil {
		payer = guest
	}
	body.PayerID = payer.ID
	// We only accept a single fund-a-need at each level from each guest.
	// That way we can enter bidder numbers during FAN without worrying
	// about some of them being duplicates.
	if isFAN {
		model.FetchPurchases(r.Tx, func(p *model.Purchase) {
			if p.Unbid {
				// Reuse the existing purchase record and turn
				// off the Unbid flag.
				body.ID = p.ID
			} else {
				// Already have this purchase.
				isDup = true
			}
		}, "item=? AND guest=?", body.ItemID, body.GuestID)
	}
	// Registrations and donations don't need to be "picked up", so we'll
	// treat them as if they have been.
	if body.ItemID == 1 || isFAN {
		body.PickedUp = true
	}
	if !isDup {
		body.Save(r.Tx, &je)
		journal.Log(r, &je)
	}
	w.CommitNoContent(r)
}

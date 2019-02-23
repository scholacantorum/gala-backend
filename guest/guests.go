package guest

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/gstripe"
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
		CardSource string
		PayingFor  []db.ID
	}
	var (
		body          addGuestBody
		je            model.JournalEntry
		purchase      model.Purchase
		errmsg        string
		status        int
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

	// Find the Stripe customer if any, or create one if we have a card.
	status, errmsg = gstripe.FindOrCreateCustomer(&body.Guest, body.CardSource)
	if status != 200 {
		w.WriteHeader(status)
		fmt.Fprint(w, errmsg)
		return
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
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

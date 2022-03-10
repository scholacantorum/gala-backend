package guest

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/gstripe"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServeGuest handles requests starting with /guest.
func ServeGuest(w *request.ResponseWriter, r *request.Request) {
	var (
		head  string
		gid   int
		guest *model.Guest
		err   error
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	if gid, err = strconv.Atoi(head); err != nil || gid < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if guest = model.FetchGuest(r.Tx, db.ID(gid)); guest == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		serveGuest(w, r, guest)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// serveGuest handles requests to /guest/${gid}.
func serveGuest(w *request.ResponseWriter, r *request.Request, guest *model.Guest) {
	switch r.Method {
	case http.MethodDelete:
		deleteGuest(w, r, guest)
	case http.MethodPut:
		saveGuest(w, r, guest)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// deleteGuest handles a DELETE /guest/${gid} request.
func deleteGuest(w *request.ResponseWriter, r *request.Request, guest *model.Guest) {
	var (
		hasPurchases bool
		je           model.JournalEntry
	)
	// Disallow if the guest has or is paying for purchases.
	model.FetchPurchases(r.Tx, func(p *model.Purchase) { hasPurchases = true }, `payer=?1 OR guest=?1`, guest.ID)
	if hasPurchases {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If the guest is default paying for anyone, undo that.
	model.FetchGuests(r.Tx, func(g *model.Guest) {
		g.PayerID = 0
		g.Save(r.Tx, &je)
	}, `payer=?`, guest.ID)
	guest.Delete(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

// saveGuest handles a PUT /guest/${gid} request.
func saveGuest(w *request.ResponseWriter, r *request.Request, guest *model.Guest) {
	type saveGuestBody struct {
		model.Guest
		CardSource            string  `json:"cardSource"`
		PayingFor             []db.ID `json:"payingFor"`
		PayingForPurchasesAdd []db.ID `json:"payingForPurchasesAdd,omitempty"`
		TableID               db.ID   `json:"table"`
		X                     int     `json:"x"`
		Y                     int     `json:"y"`
	}
	var (
		body          saveGuestBody
		je            model.JournalEntry
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
	if body.PayingForPurchasesAdd != nil {
		addPayingForPurchases(w, r, guest, body.PayingForPurchasesAdd)
		return
	}
	if body.ID != guest.ID || body.Name == "" || (body.CardSource != "" && body.Email == "") ||
		(body.UseCard && body.PayerID != 0) || (body.CardSource != "" && body.PayerID != 0) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.PayerID != 0 {
		// Make sure the proposed payer exists and no one is paying for them.
		if payer := model.FetchGuest(r.Tx, body.PayerID); payer == nil || payer.PayerID != 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	for _, pfid = range body.PayingFor {
		bodyPayingFor[pfid] = true
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
	if body.CardSource != "" {
		body.UseCard = true
	}
	if body.PartyID != guest.PartyID && body.PartyID != 0 {
		if party := model.FetchParty(r.Tx, body.PartyID); party == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// If the name or email has changed, and a payment method was provided,
	// we want a new Stripe customer.
	if (guest.Name == body.Name && guest.Email == body.Email) || (body.CardSource == "" && body.PayerID == 0) {
		body.StripeCustomer = guest.StripeCustomer
	} else {
		body.StripeCustomer = ""
	}
	if body.StripeCustomer != "" { // Update the existing customer.
		status, errmsg = gstripe.UpdateCustomer(&body.Guest, body.CardSource)
	} else {
		status, errmsg = gstripe.FindOrCreateCustomer(&body.Guest, body.CardSource)
	}
	if status != 200 {
		w.WriteHeader(status)
		fmt.Fprint(w, errmsg)
		return
	}

	// Update the database and generate the journal.
	if body.Name != guest.Name {
		// preserve the unusual sortname of "John Doe Guest #1"
		guest.Sortname = sortname(body.Name)
	}
	guest.Name = body.Name
	guest.Email = body.Email
	guest.Address = body.Address
	guest.City = body.City
	guest.State = body.State
	guest.Zip = body.Zip
	guest.Phone = body.Phone
	guest.Requests = body.Requests
	guest.PartyID = body.PartyID
	guest.Bidder = body.Bidder
	guest.PayerID = body.PayerID
	guest.StripeCustomer = body.StripeCustomer
	guest.StripeSource = body.StripeSource
	guest.StripeDescription = body.StripeDescription
	guest.UseCard = body.UseCard
	guest.Entree = body.Entree
	guest.Save(r.Tx, &je)
	model.FetchGuests(r.Tx, func(g *model.Guest) {
		if !bodyPayingFor[g.ID] && g.PayerID == guest.ID {
			g.PayerID = 0
			g.Save(r.Tx, &je)
		} else if bodyPayingFor[g.ID] && g.PayerID != guest.ID {
			g.PayerID = guest.ID
			g.Save(r.Tx, &je)
		}
	}, "")
	if body.TableID != 0 {
		party := model.FetchParty(r.Tx, guest.PartyID)
		party.TableID = body.TableID
		party.Save(r.Tx, &je)
	}
	if body.X != 0 || body.Y != 0 {
		party := model.FetchParty(r.Tx, guest.PartyID)
		table := model.FetchTable(r.Tx, party.TableID)
		table.X = body.X
		table.Y = body.Y
		table.Save(r.Tx, &je)
	}
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

func addPayingForPurchases(w *request.ResponseWriter, r *request.Request, payer *model.Guest, purchases []db.ID) {
	var (
		je       model.JournalEntry
		purchase *model.Purchase
	)
	for _, pid := range purchases {
		if purchase = model.FetchPurchase(r.Tx, pid); purchase == nil {
			log.Print("addPayingForPurchases no such purchase")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if purchase.PaymentTimestamp != "" {
			log.Print("addPayingForPurchases purchase already paid")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		purchase.PayerID = payer.ID
		purchase.Save(r.Tx, &je)
	}
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

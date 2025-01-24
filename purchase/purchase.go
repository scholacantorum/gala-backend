package purchase

import (
	"net/http"
	"strconv"

	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServePurchase handles requests starting with /purchase.
func ServePurchase(w *request.ResponseWriter, r *request.Request) {
	var (
		head     string
		pid      int
		purchase *model.Purchase
		err      error
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	if pid, err = strconv.Atoi(head); err != nil || pid < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if purchase = model.FetchPurchase(r.Tx, db.ID(pid)); purchase == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		servePurchase(w, r, purchase)
	case "pickup":
		servePickup(w, r, purchase)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// servePurchase handles requests to /purchase/${iid}.
func servePurchase(w *request.ResponseWriter, r *request.Request, purchase *model.Purchase) {
	switch r.Method {
	case http.MethodDelete:
		deletePurchase(w, r, purchase)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// deletePurchase handles a DELETE /purchase/${iid} request.
func deletePurchase(w *request.ResponseWriter, r *request.Request, purchase *model.Purchase) {
	var je model.JournalEntry

	if purchase.PaymentTimestamp != "" {
		w.WriteHeader(http.StatusConflict)
		return
	}
	purchase.Delete(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

// servePickup handles requests to /purchase/${iid}/pickup.
func servePickup(w *request.ResponseWriter, r *request.Request, purchase *model.Purchase) {
	if head, _ := request.ShiftPath(r.URL.Path); head != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPost:
		pickUpPurchase(w, r, purchase)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// pickUpPurchase handles a POST /purchase/${iid}/pickup request.
func pickUpPurchase(w *request.ResponseWriter, r *request.Request, purchase *model.Purchase) {
	var je model.JournalEntry

	if purchase.PaymentTimestamp == "" || purchase.PickedUp {
		w.WriteHeader(http.StatusConflict)
		return
	}
	purchase.PickedUp = true
	purchase.Save(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

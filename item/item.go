package item

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServeItem handles requests starting with /item.
func ServeItem(w *request.ResponseWriter, r *request.Request) {
	var (
		head string
		iid  int
		item *model.Item
		err  error
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	if iid, err = strconv.Atoi(head); err != nil || iid < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if item = model.FetchItem(r.Tx, db.ID(iid)); item == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		serveItem(w, r, item)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// serveItem handles requests to /item/${iid}.
func serveItem(w *request.ResponseWriter, r *request.Request, item *model.Item) {
	switch r.Method {
	case http.MethodDelete:
		deleteItem(w, r, item)
	case http.MethodPut:
		saveItem(w, r, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// deleteItem handles a DELETE /item/${iid} request.
func deleteItem(w *request.ResponseWriter, r *request.Request, item *model.Item) {
	var (
		hasPurchases bool
		je           model.JournalEntry
	)

	model.FetchPurchases(r.Tx, func(p *model.Purchase) { hasPurchases = true }, `item=?`, item.ID)
	if hasPurchases {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	item.Delete(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

// saveItem handles a PUT /item/${iid} request.
func saveItem(w *request.ResponseWriter, r *request.Request, item *model.Item) {
	var (
		body model.Item
		je   model.JournalEntry
		err  error
	)
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("saveItem JSON decode %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.ID != item.ID || body.Name == "" || body.Amount < 0 || body.Value < 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	item.Name = body.Name
	item.Amount = body.Amount
	item.Value = body.Value
	item.Save(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

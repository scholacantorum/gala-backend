package item

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServeItems handles requests starting with /items.
func ServeItems(w *request.ResponseWriter, r *request.Request) {
	var (
		head string
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		serveItems(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// serveItem handles requests to /items.
func serveItems(w *request.ResponseWriter, r *request.Request) {
	switch r.Method {
	case http.MethodPost:
		addItem(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// addItem handles a POST /items request.
func addItem(w *request.ResponseWriter, r *request.Request) {
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
	if body.Name == "" || body.Amount < 0 || body.Value < 0 || body.ID != 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body.Save(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

package table

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

// ServeTable handles requests starting with /table.
func ServeTable(w *request.ResponseWriter, r *request.Request) {
	var (
		head  string
		pid   int
		table *model.Table
		err   error
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	if head == "reposition" {
		serveRepositionTables(w, r)
		return
	}
	if pid, err = strconv.Atoi(head); err != nil || pid < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if table = model.FetchTable(r.Tx, db.ID(pid)); table == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		serveTable(w, r, table)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// serveTable handles requests to /table/${pid}.
func serveTable(w *request.ResponseWriter, r *request.Request, table *model.Table) {
	switch r.Method {
	case http.MethodPut:
		saveTable(w, r, table)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// saveTable handles a PUT /table/${pid} request.
func saveTable(w *request.ResponseWriter, r *request.Request, table *model.Table) {
	var (
		body model.Table
		from *model.Table
		je   model.JournalEntry
		err  error
	)
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("saveTable JSON decode %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.ID != table.ID || body.Number < 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.Number != table.Number && body.Number != 0 {
		model.FetchTables(r.Tx, func(t *model.Table) { from = t }, `num=?`, body.Number)
		if from != nil {
			from.Number = 0
			from.Save(r.Tx, &je)
		}
	}
	body.Save(r.Tx, &je)
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

// serveRepositionTables handles POST /table/reposition.  It takes a JSON array
// of tables — really {id,x,y} tuples — and updates the (x,y) coordinates of
// each of the identified tables.
func serveRepositionTables(w *request.ResponseWriter, r *request.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var (
		body []*model.Table
		je   model.JournalEntry
		err  error
	)
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("serveRepositionTables bad JSON %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, t := range body {
		var table = model.FetchTable(r.Tx, t.ID)

		if table == nil {
			log.Print("serveRepositionTables no such table")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		table.X = t.X
		table.Y = t.Y
		table.Save(r.Tx, &je)
	}
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

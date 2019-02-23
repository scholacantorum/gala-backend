package party

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

// ServeParty handles requests starting with /party.
func ServeParty(w *request.ResponseWriter, r *request.Request) {
	var (
		head  string
		pid   int
		party *model.Party
		err   error
	)
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	if pid, err = strconv.Atoi(head); err != nil || pid < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if party = model.FetchParty(r.Tx, db.ID(pid)); party == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	switch head {
	case "":
		serveParty(w, r, party)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// serveParty handles requests to /party/${pid}.
func serveParty(w *request.ResponseWriter, r *request.Request, party *model.Party) {
	switch r.Method {
	case http.MethodPut:
		saveParty(w, r, party)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// saveParty handles a PUT /party/${pid} request.
func saveParty(w *request.ResponseWriter, r *request.Request, party *model.Party) {
	type savePartyBody struct {
		model.Party
		X int `json:"x"` // position of newly created table
		Y int `json:"y"`
	}
	var (
		body savePartyBody
		je   model.JournalEntry
		err  error
	)
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("saveParty JSON decode %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.ID != party.ID {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.TableID != party.TableID && body.TableID != 0 {
		if table := model.FetchTable(r.Tx, body.TableID); table == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	body.Save(r.Tx, &je)
	if body.X != 0 || body.Y != 0 {
		table := model.FetchTable(r.Tx, body.TableID)
		table.X = body.X
		table.Y = body.Y
		table.Save(r.Tx, &je)
	}
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

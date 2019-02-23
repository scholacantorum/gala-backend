package journal

import (
	"encoding/json"
	"net/http"

	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServeAll handles requests starting with /all.
func ServeAll(w *request.ResponseWriter, r *request.Request) {
	var (
		head string
		msg  message
		je   model.JournalEntry
		err  error
	)

	if head, r.URL.Path = request.ShiftPath(r.URL.Path); head != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	msg.Seq = getJournalSequence(r)
	model.FetchTables(r.Tx, func(t *model.Table) { je.MarkTable(t.ID) }, "")
	model.FetchParties(r.Tx, func(p *model.Party) { je.MarkParty(p.ID) }, "")
	model.FetchGuests(r.Tx, func(g *model.Guest) { je.MarkGuest(g.ID) }, "")
	model.FetchItems(r.Tx, func(i *model.Item) { je.MarkItem(i.ID) }, "")
	model.FetchPurchases(r.Tx, func(p *model.Purchase) { je.MarkPurchase(p.ID) }, "")
	je.MarkBidderToGuest()
	je.Populate(r.Tx)
	if msg.Data, err = json.Marshal(&je); err != nil {
		panic(err)
	}
	json.NewEncoder(w).Encode(msg)
}

func getJournalSequence(r *request.Request) (seq int) {
	var err error

	if err = r.Tx.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM journal`).Scan(&seq); err != nil {
		panic(err)
	}
	return seq
}

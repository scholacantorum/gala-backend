package model

import (
	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/db"
)

// A JournalEntry describes one transactional change to the data set.
type JournalEntry struct {
	Tables        map[db.ID]*Table    `json:"tables,omitempty"`
	Parties       map[db.ID]*Party    `json:"parties,omitempty"`
	Guests        map[db.ID]*Guest    `json:"guests,omitempty"`
	Items         map[db.ID]*Item     `json:"items,omitempty"`
	Purchases     map[db.ID]*Purchase `json:"purchases,omitempty"`
	BidderToGuest map[int]db.ID       `json:"bidderToGuest,omitempty"`
}

// MarkTable marks a table as having been changed or deleted.
func (j *JournalEntry) MarkTable(id db.ID) {
	if j.Tables == nil {
		j.Tables = make(map[db.ID]*Table)
	}
	j.Tables[id] = nil
}

// MarkParty marks a party as having been changed or deleted.
func (j *JournalEntry) MarkParty(id db.ID) {
	if j.Parties == nil {
		j.Parties = make(map[db.ID]*Party)
	}
	j.Parties[id] = nil
}

// MarkGuest marks a guest as having been changed or deleted.
func (j *JournalEntry) MarkGuest(id db.ID) {
	if j.Guests == nil {
		j.Guests = make(map[db.ID]*Guest)
	}
	j.Guests[id] = nil
}

// MarkItem marks an item as having been changed or deleted.
func (j *JournalEntry) MarkItem(id db.ID) {
	if j.Items == nil {
		j.Items = make(map[db.ID]*Item)
	}
	j.Items[id] = nil
}

// MarkPurchase marks a purchase as having been changed or deleted.
func (j *JournalEntry) MarkPurchase(id db.ID) {
	if j.Purchases == nil {
		j.Purchases = make(map[db.ID]*Purchase)
	}
	j.Purchases[id] = nil
}

// MarkBidderToGuest marks the need to recalculate the bidder-to-guest mapping.
func (j *JournalEntry) MarkBidderToGuest() {
	j.BidderToGuest = make(map[int]db.ID)
}

// Populate fetches the new data for all of the changed objects.
func (j *JournalEntry) Populate(tx *sqlx.Tx) {
	for tid := range j.Tables {
		if j.Tables[tid] = FetchTable(tx, tid); j.Tables[tid] != nil {
			j.Tables[tid].Populate(tx)
		}
	}
	for pid := range j.Parties {
		if j.Parties[pid] = FetchParty(tx, pid); j.Parties[pid] != nil {
			j.Parties[pid].Populate(tx)
		}
	}
	for gid := range j.Guests {
		if j.Guests[gid] = FetchGuest(tx, gid); j.Guests[gid] != nil {
			j.Guests[gid].Populate(tx)
		}
	}
	for iid := range j.Items {
		if j.Items[iid] = FetchItem(tx, iid); j.Items[iid] != nil {
			j.Items[iid].Populate(tx)
		}
	}
	for pid := range j.Purchases {
		if j.Purchases[pid] = FetchPurchase(tx, pid); j.Purchases[pid] != nil {
			j.Purchases[pid].Populate(tx)
		}
	}
	if j.BidderToGuest != nil {
		FetchGuests(tx, func(g *Guest) {
			if g.PayerID == 0 || j.BidderToGuest[g.Bidder] == 0 {
				j.BidderToGuest[g.Bidder] = g.ID
			}
		}, `bidder!=0`)
	}
}

package model

import (
	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/db"
)

// updateBidderNumbers ensures that all guests have bidder numbers appropriate
// for their tables.  It does not change bidder numbers unless they are wrong
// for their tables.
func updateBidderNumbers(tx *sqlx.Tx, je *JournalEntry) {
	FetchTables(tx, func(table *Table) {
		updateBidderNumbersAtTable(tx, je, table)
	}, "")
}
func updateBidderNumbersAtTable(tx *sqlx.Tx, je *JournalEntry, table *Table) {
	var (
		toadjust []*Guest
		bidders  = make(map[db.ID]int)
		used     = make(map[int]bool)
	)
	FetchPartiesAtTable(tx, table.ID, func(p *Party) {
		FetchGuestsInParty(tx, p.ID, func(g *Guest) {
			switch {
			case table.Number == 0 && g.Bidder == 0: // no change needed
				break
			case table.Number == 0 && g.Bidder != 0: // remove bidder number since not at table
				g.Bidder = 0
				g.Save(tx, je)
				je.MarkBidderToGuest()
			case g.Bidder/16 == tableNumberToBidderBase(table.Number): // valid bidder number for table
				bidders[g.ID] = g.Bidder
				used[g.Bidder] = true
			default: // bidder number doesn't match table
				gcopy := *g
				toadjust = append(toadjust, &gcopy)
			}
		})
	})
	for _, g := range toadjust { // assign to self-payers first
		if g.PayerID == 0 {
			g.Bidder = updateBidderNumberNextAvail(used, table.Number)
			g.Save(tx, je)
			bidders[g.ID] = g.Bidder
		}
	}
	for _, g := range toadjust { // then to non-self-payers
		if g.PayerID != 0 {
			if b := bidders[g.PayerID]; b != 0 {
				// Their payer is at the same table; share
				// bidder numbers.
				g.Bidder = b
			} else {
				g.Bidder = updateBidderNumberNextAvail(used, table.Number)
			}
			g.Save(tx, je)
		}
	}
	if len(toadjust) != 0 {
		je.MarkBidderToGuest()
	}
}
func updateBidderNumberNextAvail(used map[int]bool, table int) (bidder int) {
	for bidder = tableNumberToBidderBase(table) * 16; used[bidder]; bidder++ {
	}
	used[bidder] = true
	return bidder
}
func tableNumberToBidderBase(tnum int) int {
	return (tnum/10)*16 + (tnum % 10)
}

package model

import (
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/db"
)

// Guest represents a single guest at the gala.  See db/schema.sql for details.
type Guest struct {
	ID                 db.ID   `json:"id" db:"id"`
	Name               string  `json:"name" db:"name"`
	Sortname           string  `json:"sortname" db:"sortname"`
	Email              string  `json:"email" db:"email"`
	Address            string  `json:"address" db:"address"`
	City               string  `json:"city" db:"city"`
	State              string  `json:"state" db:"state"`
	Zip                string  `json:"zip" db:"zip"`
	Phone              string  `json:"phone" db:"phone"`
	Requests           string  `json:"requests" db:"requests"`
	PartyID            db.ID   `json:"party" db:"party"`
	Bidder             int     `json:"bidder" db:"bidder"`
	StripeCustomer     string  `json:"-" db:"stripeCustomer"`
	StripeSource       string  `json:"stripeSource" db:"stripeSource"`
	StripeDescription  string  `json:"stripeDescription" db:"stripeDescription"`
	UseCard            bool    `json:"useCard" db:"useCard"`
	PayerID            db.ID   `json:"payer" db:"payer"`
	PayingFor          []db.ID `json:"payingFor" db:"-"`
	Purchases          []db.ID `json:"purchases" db:"-"`
	PayingForPurchases []db.ID `json:"payingForPurchases" db:"-"`
	AllPaid            bool    `json:"allPaid" db:"-"`
}

// Save saves a guest to the database.  It also adds the guest to the JSON
// journal.
func (g *Guest) Save(tx *sqlx.Tx, je *JournalEntry) {
	var (
		res     sql.Result
		obidder int
		opayer  db.ID
		oparty  db.ID
		nid     int64
		err     error
	)
	if g.ID != 0 {
		err = tx.QueryRow(`SELECT bidder, payer, party FROM guest WHERE id=?`, g.ID).Scan(&obidder, &opayer, &oparty)
		if err != nil {
			panic(err)
		}
	}
	if g.PartyID == 0 {
		var party Party
		party.Save(tx, je)
		g.PartyID = party.ID
	}
	res, err = tx.Exec(`
INSERT OR REPLACE INTO guest (id, name, sortname, email, address, city, state, zip, phone, requests, party, bidder, stripeCustomer,
    stripeSource, stripeDescription, useCard, payer) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		g.ID, g.Name, g.Sortname, g.Email, g.Address, g.City, g.State, g.Zip, g.Phone, g.Requests, g.PartyID, g.Bidder,
		g.StripeCustomer, g.StripeSource, g.StripeDescription, g.UseCard, g.PayerID)
	if err != nil {
		panic(err)
	}
	if g.ID == 0 {
		if nid, err = res.LastInsertId(); err != nil {
			panic(err)
		} else {
			g.ID = db.ID(nid)
		}
	}
	je.MarkGuest(g.ID)
	if opayer != 0 && opayer != g.PayerID {
		je.MarkGuest(opayer)
	}
	if g.PayerID != 0 && opayer != g.PayerID {
		je.MarkGuest(g.PayerID)
	}
	if obidder != g.Bidder {
		je.MarkBidderToGuest()
	}
	if oparty != 0 && oparty != g.PartyID {
		FetchParty(tx, oparty).deleteIfEmpty(tx, je)
		je.MarkParty(oparty)
	}
	if oparty != g.PartyID {
		je.MarkParty(g.PartyID)
	}
}

// Populate adds computed data to the guest entry prior to its inclusion in a
// journal entry.
func (g *Guest) Populate(tx *sqlx.Tx) {
	g.PayingFor = []db.ID{}
	FetchGuests(tx, func(g2 *Guest) {
		g.PayingFor = append(g.PayingFor, g2.ID)
	}, `payer=?`, g.ID)
	g.Purchases = []db.ID{}
	FetchPurchases(tx, func(p *Purchase) {
		g.Purchases = append(g.Purchases, p.ID)
	}, `guest=?`, g.ID)
	g.PayingForPurchases = []db.ID{}
	g.AllPaid = true
	FetchPurchases(tx, func(p *Purchase) {
		g.PayingForPurchases = append(g.PayingForPurchases, p.ID)
		if p.PaymentTimestamp == "" {
			g.AllPaid = false
		}
	}, `payer=?`, g.ID)
}

// Delete deletes a guest.  It also adds the deletion to the JSON journal.
func (g *Guest) Delete(tx *sqlx.Tx, je *JournalEntry) {
	tx.MustExec(`DELETE FROM guest WHERE id=?`, g.ID)
	je.MarkGuest(g.ID)
	if g.PayerID != 0 {
		je.MarkGuest(g.PayerID)
	}
	if g.Bidder != 0 {
		je.MarkBidderToGuest()
	}
	FetchParty(tx, g.PartyID).deleteIfEmpty(tx, je)
	je.MarkParty(g.PartyID)
}

// FetchGuest returns the guest with the specified ID.  It returns nil if the
// guest does not exist.
func FetchGuest(tx *sqlx.Tx, id db.ID) (g *Guest) {
	g = new(Guest)
	switch err := tx.Get(g, `SELECT * FROM guest WHERE id=?`, id); err {
	case nil:
		return g
	case sql.ErrNoRows:
		return nil
	default:
		panic(err)
	}
}

// FetchGuests calls the supplied function with each guest that matches the
// supplied criteria.  The guests are retrieved in no particular order.
func FetchGuests(tx *sqlx.Tx, fn func(*Guest), criteria string, args ...interface{}) {
	var (
		g    Guest
		rows *sqlx.Rows
		err  error
	)
	if criteria != "" {
		rows, err = tx.Queryx(`SELECT * FROM guest WHERE `+criteria, args...)
	} else {
		rows, err = tx.Queryx(`SELECT * FROM guest`)
	}
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		if err = rows.StructScan(&g); err != nil {
			panic(err)
		}
		fn(&g)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}
}

// FetchGuestsInParty calls the supplied function with each guest in the
// specified party.  They are returned in creation order; given the way
// registration works, that probably means the host is first and the rest are
// listed in the order the host listed them.  (That may not hold after parties
// are rearranged.)
func FetchGuestsInParty(tx *sqlx.Tx, party db.ID, fn func(*Guest)) {
	var (
		g    Guest
		rows *sqlx.Rows
		err  error
	)
	if rows, err = tx.Queryx(`SELECT * FROM guest WHERE party=? ORDER BY id`, party); err != nil {
		panic(err)
	}
	for rows.Next() {
		if err = rows.StructScan(&g); err != nil {
			panic(err)
		}
		fn(&g)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}
}

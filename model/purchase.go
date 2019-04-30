package model

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/db"
)

// Purchase represents a purchase of an Item.  See db/schema.sql for details.
type Purchase struct {
	ID                 db.ID  `json:"id" db:"id"`
	GuestID            db.ID  `json:"guest" db:"guest"`
	PayerID            db.ID  `json:"payer" db:"payer"`
	ItemID             db.ID  `json:"item" db:"item"`
	Amount             int    `json:"amount" db:"amount"`
	PaymentTimestamp   string `json:"paymentTimestamp" db:"paymentTimestamp"`
	PaymentDescription string `json:"paymentDescription" db:"paymentDescription"`
	ScholaOrder        int    `json:"scholaOrder" db:"scholaOrder"`
	HaveCard           bool   `json:"haveCard" db:"-"`
}

// Save saves a purchase to the database.  It also adds the purchase to the JSON
// journal.
func (p *Purchase) Save(tx *sqlx.Tx, je *JournalEntry) {
	var (
		res              sql.Result
		ogid, opid, oiid db.ID
		nid              int64
		err              error
	)
	if p.ID != 0 {
		err = tx.QueryRow(`SELECT guest, payer, item FROM purchase WHERE id=?`, p.ID).Scan(&ogid, &opid, &oiid)
		if err != nil {
			panic(err)
		}
	}
	res, err = tx.Exec(`
INSERT OR REPLACE INTO purchase (id, guest, payer, item, amount, paymentTimestamp, paymentDescription, scholaOrder) VALUES (?,?,?,?,?,?,?,?)`,
		p.ID, p.GuestID, p.PayerID, p.ItemID, p.Amount, p.PaymentTimestamp, p.PaymentDescription, p.ScholaOrder)
	if err != nil {
		panic(err)
	}
	if p.ID == 0 {
		if nid, err = res.LastInsertId(); err != nil {
			panic(err)
		} else {
			p.ID = db.ID(nid)
		}
	}
	je.MarkPurchase(p.ID)
	if ogid != 0 && ogid != p.GuestID {
		je.MarkGuest(ogid)
	}
	if ogid != p.GuestID {
		je.MarkGuest(p.GuestID)
	}
	if opid != 0 && opid != p.PayerID {
		je.MarkGuest(opid)
	}
	if opid != p.PayerID {
		je.MarkGuest(p.PayerID)
	}
	if oiid != 0 && oiid != p.ItemID {
		je.MarkItem(oiid)
	}
	if oiid != p.ItemID {
		je.MarkItem(p.ItemID)
	}
}

// Populate adds computed data to the purchase prior to its inclusion in a
// journal entry.
func (p *Purchase) Populate(tx *sqlx.Tx) {
	p.HaveCard = FetchGuest(tx, p.PayerID).UseCard
}

// Delete deletes a purchase.  It also adds the deletion to the JSON journal.
func (p *Purchase) Delete(tx *sqlx.Tx, je *JournalEntry) {
	tx.MustExec(`DELETE FROM purchase WHERE id=?`, p.ID)
	je.MarkPurchase(p.ID)
	je.MarkGuest(p.GuestID)
	je.MarkGuest(p.PayerID)
	je.MarkItem(p.ItemID)
}

// FetchPurchase returns the purchase with the specified ID.  It returns nil if
// the purchase does not exist.
func FetchPurchase(tx *sqlx.Tx, id db.ID) (p *Purchase) {
	p = new(Purchase)
	switch err := tx.Get(p, `SELECT * FROM purchase WHERE id=?`, id); err {
	case nil:
		return p
	case sql.ErrNoRows:
		return nil
	default:
		panic(err)
	}
}

// FetchPurchases calls the supplied function with each purchase that matches
// the supplied criteria.  The purchases are retrieved in order of creation.
func FetchPurchases(tx *sqlx.Tx, fn func(*Purchase), criteria string, args ...interface{}) {
	var (
		p    Purchase
		rows *sqlx.Rows
		err  error
	)
	if criteria != "" {
		rows, err = tx.Queryx(fmt.Sprintf(`SELECT * FROM purchase WHERE %s ORDER BY id`, criteria), args...)
	} else {
		rows, err = tx.Queryx(`SELECT * FROM purchase ORDER BY id`)
	}
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		if err = rows.StructScan(&p); err != nil {
			panic(err)
		}
		fn(&p)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}
}

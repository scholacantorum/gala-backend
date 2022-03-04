package model

import (
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/db"
)

// Item represents a item that can be purchased, or a donation level.  See
// db/schema.sql for details.
type Item struct {
	ID        db.ID   `json:"id" db:"id"`
	Name      string  `json:"name" db:"name"`
	Amount    int     `json:"amount" db:"amount"`
	Value     int     `json:"value" db:"value"`
	Purchases []db.ID `json:"purchases" db:"-"`
}

// Save saves an item to the database.  It also adds the item to the JSON
// journal.
func (i *Item) Save(tx *sqlx.Tx, je *JournalEntry) {
	var (
		res sql.Result
		nid int64
		err error
	)
	res, err = tx.Exec(`INSERT OR REPLACE INTO item (id, name, amount, value) VALUES (?,?,?,?,?)`,
		i.ID, i.Name, i.Amount, i.Value)
	if err != nil {
		panic(err)
	}
	if i.ID == 0 {
		if nid, err = res.LastInsertId(); err != nil {
			panic(err)
		} else {
			i.ID = db.ID(nid)
		}
	}
	je.MarkItem(i.ID)
}

// Populate adds computed data to the item prior to its inclusion in a journal
// entry.
func (i *Item) Populate(tx *sqlx.Tx) {
	i.Purchases = []db.ID{}
	FetchPurchases(tx, func(p *Purchase) {
		i.Purchases = append(i.Purchases, p.ID)
	}, `item=?`, i.ID)
}

// Delete deletes an item.  It also adds the deletion to the JSON journal.
func (i *Item) Delete(tx *sqlx.Tx, je *JournalEntry) {
	tx.MustExec(`DELETE FROM item WHERE id=?`, i.ID)
	je.MarkItem(i.ID)
}

// FetchItem returns the item with the specified ID.  It returns nil if the
// item does not exist.
func FetchItem(tx *sqlx.Tx, id db.ID) (i *Item) {
	i = new(Item)
	switch err := tx.Get(i, `SELECT * FROM item WHERE id=?`, id); err {
	case nil:
		return i
	case sql.ErrNoRows:
		return nil
	default:
		panic(err)
	}
}

// FetchItems calls the supplied function with each item that matches the
// supplied criteria.  The items are retrieved in no particular order.
func FetchItems(tx *sqlx.Tx, fn func(*Item), criteria string, args ...interface{}) {
	var (
		i    Item
		rows *sqlx.Rows
		err  error
	)
	if criteria != "" {
		rows, err = tx.Queryx(`SELECT * FROM item WHERE `+criteria, args...)
	} else {
		rows, err = tx.Queryx(`SELECT * FROM item`)
	}
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		if err = rows.StructScan(&i); err != nil {
			panic(err)
		}
		fn(&i)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}
}

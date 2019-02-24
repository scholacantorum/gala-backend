package model

import (
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/db"
)

// Table represents a table, or potential table, at the event.  See
// db/schema.sql for details.
type Table struct {
	ID      db.ID   `json:"id" db:"id"`
	X       int     `json:"x" db:"x"`
	Y       int     `json:"y" db:"y"`
	Number  int     `json:"number" db:"num"`
	Parties []db.ID `json:"parties" db:"-"`
}

// Save saves a table to the database.  It also adds the table to the JSON
// journal.
func (t *Table) Save(tx *sqlx.Tx, je *JournalEntry) {
	var (
		res   sql.Result
		otnum int
		nid   int64
		err   error
	)
	if t.ID != 0 {
		if err = tx.QueryRow(`SELECT num FROM gtable WHERE id=?`, t.ID).Scan(&otnum); err != nil {
			panic(err)
		}
	}
	res, err = tx.Exec(`INSERT OR REPLACE INTO gtable (id, x, y, num) VALUES (?,?,?,?)`, t.ID, t.X, t.Y, t.Number)
	if err != nil {
		panic(err)
	}
	if t.ID == 0 {
		if nid, err = res.LastInsertId(); err != nil {
			panic(err)
		} else {
			t.ID = db.ID(nid)
		}
	}
	je.MarkTable(t.ID)
	if t.Number != otnum {
		updateBidderNumbers(tx, je)
	}
}

// Populate adds computed data to the table prior to its inclusion in a journal
// entry.
func (t *Table) Populate(tx *sqlx.Tx) {
	t.Parties = []db.ID{}
	FetchPartiesAtTable(tx, t.ID, func(p *Party) {
		t.Parties = append(t.Parties, p.ID)
	})
}

// NextPlace returns the next unused place number at this table.
func (t *Table) NextPlace(tx *sqlx.Tx) (place int) {
	var err error

	if err = tx.QueryRow(`SELECT MAX(place) FROM party WHERE gtable=?`, t.ID).Scan(&place); err != nil {
		panic(err)
	}
	return place + 1
}

// Delete deletes a table.  It also adds the deletion to the JSON journal.
func (t *Table) Delete(tx *sqlx.Tx, je *JournalEntry) {
	tx.MustExec(`DELETE FROM gtable WHERE id=?`, t.ID)
	je.MarkTable(t.ID)
}

// deleteIfEmpty deletes a table if it has no parties.
func (t *Table) deleteIfEmpty(tx *sqlx.Tx, je *JournalEntry) {
	var (
		dummy int
		err   error
	)
	switch err = tx.QueryRow(`SELECT id FROM party WHERE gtable=? LIMIT 1`, t.ID).Scan(&dummy); err {
	case nil:
		break
	case sql.ErrNoRows:
		t.Delete(tx, je)
	default:
		panic(err)
	}
}

// FetchTable returns the table with the specified ID.  It returns nil if the
// table does not exist.
func FetchTable(tx *sqlx.Tx, id db.ID) (t *Table) {
	t = new(Table)
	switch err := tx.Get(t, `SELECT * FROM gtable WHERE id=?`, id); err {
	case nil:
		return t
	case sql.ErrNoRows:
		return nil
	default:
		panic(err)
	}
}

// FetchTables calls the supplied function with each table that matches the
// supplied criteria.  The tables are retrieved in no particular order.
func FetchTables(tx *sqlx.Tx, fn func(*Table), criteria string, args ...interface{}) {
	var (
		t    Table
		rows *sqlx.Rows
		err  error
	)
	if criteria != "" {
		rows, err = tx.Queryx(`SELECT * FROM gtable WHERE `+criteria, args...)
	} else {
		rows, err = tx.Queryx(`SELECT * FROM gtable`)
	}
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		if err = rows.StructScan(&t); err != nil {
			panic(err)
		}
		fn(&t)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}
}

// NextTableNumber returns the next unused table number.
func NextTableNumber(tx *sqlx.Tx) (number int) {
	var err error

	if err = tx.QueryRow(`SELECT MAX(num) FROM gtable`).Scan(&number); err != nil {
		panic(err)
	}
	return number + 1
}

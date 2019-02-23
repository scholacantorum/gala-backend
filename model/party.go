package model

import (
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/db"
)

// Party represents a party that should be seated together.  See db/schema.sql
// for details.
type Party struct {
	ID      db.ID   `json:"id" db:"id"`
	TableID db.ID   `json:"table" db:"gtable"`
	Place   int     `json:"place" db:"place"`
	Guests  []db.ID `json:"guests" db:"-"`
}

// Save saves a party to the database.  It also adds the party to the JSON
// journal.
func (p *Party) Save(tx *sqlx.Tx, je *JournalEntry) {
	var (
		res      sql.Result
		otableID db.ID
		ntable   *Table
		nid      int64
		err      error
	)
	if p.ID != 0 {
		if err = tx.QueryRow(`SELECT gtable FROM party WHERE id=?`, p.ID).Scan(&otableID); err != nil {
			panic(err)
		}
	}
	if p.TableID == 0 {
		ntable = new(Table)
		ntable.Save(tx, je)
		p.TableID = ntable.ID
	} else if p.TableID != otableID {
		ntable = FetchTable(tx, p.TableID)
		p.Place = ntable.NextPlace(tx)
	}
	res, err = tx.Exec(`INSERT OR REPLACE INTO party (id, gtable, place) VALUES (?,?,?)`, p.ID, p.TableID, p.Place)
	if err != nil {
		panic(err)
	}
	if p.ID == 0 {
		if nid, err = res.LastInsertId(); err != nil {
			panic(err)
		} else {
			p.ID = db.ID(nid)
		}
	} else if p.TableID != otableID {
		FetchTable(tx, otableID).deleteIfEmpty(tx, je)
	}
	je.MarkParty(p.ID)
	if otableID != 0 && otableID != p.TableID {
		je.MarkTable(otableID)
	}
	if otableID != p.TableID {
		je.MarkTable(p.TableID)
	}
}

// Populate adds computed data to the party prior to its inclusion in a journal
// entry.
func (p *Party) Populate(tx *sqlx.Tx) {
	p.Guests = []db.ID{}
	FetchGuestsInParty(tx, p.ID, func(g *Guest) {
		p.Guests = append(p.Guests, g.ID)
	})
}

// Delete deletes a party.  It also adds the deletion to the JSON journal.
func (p *Party) Delete(tx *sqlx.Tx, je *JournalEntry) {
	tx.MustExec(`DELETE FROM party WHERE id=?`, p.ID)
	je.MarkParty(p.ID)
	FetchTable(tx, p.TableID).deleteIfEmpty(tx, je)
	je.MarkTable(p.TableID)
}

// deleteIfEmpty deletes a party if it has no guests.
func (p *Party) deleteIfEmpty(tx *sqlx.Tx, je *JournalEntry) {
	var (
		dummy int
		err   error
	)
	switch err = tx.QueryRow(`SELECT id FROM guest WHERE party=? LIMIT 1`, p.ID).Scan(&dummy); err {
	case nil:
		break
	case sql.ErrNoRows:
		p.Delete(tx, je)
	default:
		panic(err)
	}
}

// FetchParty returns the party with the specified ID.  It returns nil if the
// party does not exist.
func FetchParty(tx *sqlx.Tx, id db.ID) (p *Party) {
	p = new(Party)
	switch err := tx.Get(p, `SELECT * FROM party WHERE id=?`, id); err {
	case nil:
		return p
	case sql.ErrNoRows:
		return nil
	default:
		panic(err)
	}
}

// FetchParties calls the supplied function with each party that matches the
// supplied criteria.  The parties are retrieved in no particular order.
func FetchParties(tx *sqlx.Tx, fn func(*Party), criteria string, args ...interface{}) {
	var (
		p    Party
		rows *sqlx.Rows
		err  error
	)
	if criteria != "" {
		rows, err = tx.Queryx(`SELECT * FROM party WHERE `+criteria, args...)
	} else {
		rows, err = tx.Queryx(`SELECT * FROM party`)
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

// FetchPartiesAtTable calls the supplied function with each party seated at the
// specified table, in the order in which they were seated there.
func FetchPartiesAtTable(tx *sqlx.Tx, table db.ID, fn func(*Party)) {
	var (
		p    Party
		rows *sqlx.Rows
		err  error
	)
	if rows, err = tx.Queryx(`SELECT * FROM party WHERE gtable=? ORDER BY place`, table); err != nil {
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

package db

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	_ "modernc.org/sqlite"
)

// Open opens the database and returns a handle to it.
func Open(path string) (*sqlx.DB, error) {
	dburl := "file:" + path + "?cache=shared&mode=rw&_busy_timeout=1000&_txlock=immediate&_foreign_keys=1"
	return sqlx.Connect("sqlite", dburl)
}

// Time is a wrapper around time.Time that stores in the database as integer
// seconds since epoch.
type Time struct {
	time.Time
}

// Value converts the time to integer-seconds-since-epoch, for storage into the
// database.
func (t Time) Value() (driver.Value, error) {
	if t.IsZero() {
		return int64(0), nil
	}
	return t.Unix(), nil
}

// Scan converts the integer-seconds-since-epoch from the database into a Time.
func (t *Time) Scan(value interface{}) error {
	tt, ok := value.(int64)
	if !ok {
		return fmt.Errorf("scanning %T into db.Time, should be int64", value)
	}
	if tt == 0 {
		*t = Time{}
	} else {
		*t = Time{time.Unix(tt, 0).In(time.Local)}
	}
	return nil
}

// ID is a wrapper around int that stores 0 in the database as NULL.
type ID int

// Value converts the ID to database format.
func (id ID) Value() (driver.Value, error) {
	if id == 0 {
		return nil, nil
	}
	return int64(id), nil
}

// Scan converts the ID from database format.
func (id *ID) Scan(value interface{}) error {
	switch value := value.(type) {
	case nil:
		*id = 0
	case int64:
		*id = ID(value)
	default:
		return fmt.Errorf("scanning %T into db.ID, should be int64 or nil", value)
	}
	return nil
}

// IDStr is a wrapper around string that stores "" in the database as NULL.
type IDStr string

// Value converts the ID to database format.
func (id IDStr) Value() (driver.Value, error) {
	if id == "" {
		return nil, nil
	}
	return string(id), nil
}

// Scan converts the ID from database format.
func (id *IDStr) Scan(value interface{}) error {
	switch value := value.(type) {
	case nil:
		*id = ""
	case string:
		*id = IDStr(value)
	case []byte:
		*id = IDStr(string(value))
	default:
		return fmt.Errorf("scanning %T into db.IDStr, should be string or nil", value)
	}
	return nil
}

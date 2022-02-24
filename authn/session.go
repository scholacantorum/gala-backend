package authn

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"time"

	"github.com/scholacantorum/gala-backend/request"
)

// ValidSession checks for a valid session token in the request.  If there is
// one, it populates the user and session data into r and returns true.  If
// not, it returns false.
func ValidSession(r *request.Request) bool {
	var err error

	if r.SessionToken == "" {
		return false
	}
	if _, err = r.Tx.Exec(`DELETE FROM session WHERE expires<?`, time.Now().Unix()); err != nil {
		panic(err)
	}
	err = r.Tx.QueryRow(`SELECT user FROM session WHERE token=?`, r.SessionToken).Scan(&r.UserID)
	if err == sql.ErrNoRows {
		r.SessionToken = ""
		return false
	}
	if err != nil {
		panic(err)
	}
	if err = r.Tx.QueryRow(`SELECT username FROM user WHERE id=?`, r.UserID).Scan(&r.Username); err != nil {
		panic(err)
	}
	return true
}

// CreateSession creates a session for the current user and adds the
// corresponding cookie to the response.
func CreateSession(w *request.ResponseWriter, r *request.Request) {
	var token string
	var err error

	token = RandomToken()
	if _, err = r.Tx.Exec(`INSERT INTO session (token, user, expires) VALUES (?,?,?)`,
		token, r.UserID, time.Now().Add(6*time.Hour)); err != nil {
		panic(err)
	}
	w.Header().Set("Auth", token)
}

// RandomToken returns a random token string, used for various purposes.
func RandomToken() string {
	var (
		tokenb [24]byte
		err    error
	)
	if _, err = rand.Read(tokenb[:]); err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(tokenb[:])
}

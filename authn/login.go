package authn

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/scholacantorum/gala-backend/request"
)

// ServeLogin handles requests to /login.
func ServeLogin(w *request.ResponseWriter, r *request.Request) {
	var head, username, password string

	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	if head != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	username = r.FormValue("username")
	password = r.FormValue("password")
	if !login(w, r, username, password) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err := r.Tx.Commit(); err != nil {
		panic(err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func login(w *request.ResponseWriter, r *request.Request, username, password string) bool {
	var uid int
	var err error

	if username == "" || password == "" {
		return false
	}
	err = r.Tx.QueryRow(`SELECT id FROM user WHERE username=?`, username).Scan(&uid)
	if err == sql.ErrNoRows {
		log.Printf("login-fail username=%q no such user", username)
		return false
	}
	if err != nil {
		panic(err)
	}
	if !CheckPassword(r.Tx, uid, password) {
		log.Printf("login-fail username=%q password mismatch", username)
		return false
	}
	r.UserID = uid
	r.Username = username
	CreateSession(w, r)
	return true
}

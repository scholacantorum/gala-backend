package request

import (
	"net/http"

	"github.com/jmoiron/sqlx"
)

// Request is a wrapper around http.Request with our web site's request-specific
// data.
type Request struct {
	*http.Request
	SessionToken string
	UserID       int
	Username     string
	Tx           *sqlx.Tx
}

// NewRequest wraps an http.Request into a request.Request.
func NewRequest(httpr *http.Request) *Request {
	var rr Request
	rr.Request = httpr
	rr.SessionToken = httpr.Header.Get("Auth")
	return &rr
}

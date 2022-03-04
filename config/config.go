package config

import (
	"net/http"

	"github.com/scholacantorum/gala-backend/private"
	stripe "github.com/stripe/stripe-go"
)

var CheckWebSocketOrigin func(r *http.Request) bool
var DatabaseFile string
var EmailTo []string
var RegisterAllowOrigin string
var ScholaOrderNumberURL string
var Sendmail string
var OrdersURL string

var TicketSKU = "ticket-2022-04-22"
var GalaTitle = "An evening of Jazz in the Big Easy"
var GalaDate = "Friday, April 22, 2022"
var GalaStartTime = "6:30pm"
var GalaVenue = "Villa Ragusa"
var GalaAddress = "35 South Second Street, Campbell"
var GalaMapURL = "https://www.google.com/maps/place/Villa+Ragusa/@37.286705,-121.9462738,15z/data=!4m5!3m4!1s0x0:0x657b9c0fec66d741!8m2!3d37.2867704!4d-121.9461649"
var GalaGuestInfoDeadline = "April 13"

func init() {
	// For live mode:
	CheckWebSocketOrigin = func(r *http.Request) bool {
		return r.Header.Get("Origin") == "https://gala.scholacantorum.org"
	}
	//stripe.Key = private.StripeLiveSecretKey
	stripe.Key = private.StripeTestSecretKey
	DatabaseFile = "/home/scmvwork/gala.db"
	// EmailTo = []string{"info@scholacantorum.org", "admin@scholacantorum.org"}
	EmailTo = []string{"admin@scholacantorum.org"}
	// RegisterAllowOrigin = "https://scholacantorum.org"
	RegisterAllowOrigin = "https://new.scholacantorum.org"
	ScholaOrderNumberURL = "https://scholacantorum.org/backend/allocate-order-number"
	Sendmail = "/home/scsvwork/bin/send-email"
	OrdersURL = "https://orders-test.scholacantorum.org"

	// For test mode:
	// CheckWebSocketOrigin = func(_ *http.Request) bool { return true }
	// DatabaseFile = "gala.db"
	// EmailTo = []string{"admin@scholacantorum.org"}
	// RegisterAllowOrigin = "https://new.scholacantorum.org"
	// ScholaOrderNumberURL = "https://new.scholacantorum.org/backend/allocate-order-number"
	// stripe.Key = private.StripeLiveSecretKey //  private.StripeTestSecretKey
	// Sendmail = "/home/scsv/bin/send-email"

	// For development mode:
	// RegisterAllowOrigin = "*"
	// Sendmail = "/Users/stever/go/bin/send-email"
}

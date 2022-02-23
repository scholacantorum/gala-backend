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

var TicketSKU = "ticket-2019-04-26"
var GalaTitle = "¡Fiesta! An Evening in Old California"
var GalaDate = "Friday, April 26, 2019"
var GalaStartTime = "6:30pm"
var GalaVenue = "Fremont Hills Country Club"
var GalaAddress = "12889 Viscaino Place, Los Altos Hills"
var GalaMapURL = "https://www.google.com/maps/place/Fremont+Hills+Country+Club/@37.3767118,-122.1628322,14z/data=!4m5!3m4!1s0x808fb069fe170007:0x1f4c3ba1f465e704!8m2!3d37.3767114!4d-122.1453173"
var GalaGuestInfoDeadline = "April 15"

func init() {
	// For live mode:
	stripe.Key = private.StripeLiveSecretKey
	DatabaseFile = "/home/scmv/gala.db"
	EmailTo = []string{"info@scholacantorum.org", "admin@scholacantorum.org"}
	RegisterAllowOrigin = "https://scholacantorum.org"
	ScholaOrderNumberURL = "https://scholacantorum.org/backend/allocate-order-number"
	Sendmail = "/home/scsv/bin/send-email"

	// For test mode:
	// CheckWebSocketOrigin = func(_ *http.Request) bool { return true }
	// DatabaseFile = "gala.db"
	// EmailTo = []string{"admin@scholacantorum.org"}
	// RegisterAllowOrigin = "https://new.scholacantorum.org"
	// ScholaOrderNumberURL = "https://new.scholacantorum.org/backend/allocate-order-number"
	// stripe.Key = private.StripeTestSecretKey
	// Sendmail = "/home/scsv/bin/send-email"

	// For development mode:
	// RegisterAllowOrigin = "*"
	// Sendmail = "/Users/stever/go/bin/send-email"
}

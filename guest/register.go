package guest

import (
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/gstripe"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// ServeRegister handles requests starting with /register.  The only supported
// one is POST /register, which is called by the public Schola web site when
// someone registers for the Gala.
func ServeRegister(w *request.ResponseWriter, r *request.Request) {
	var (
		status  int
		message string
		je      model.JournalEntry
	)
	if head, _ := request.ShiftPath(r.URL.Path); head != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if config.RegisterAllowOrigin != "*" && r.Header.Get("Origin") != config.RegisterAllowOrigin {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", config.RegisterAllowOrigin)
	w.Header().Set("Content-Type", "text/plain")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	status, message = publicRegister(r, &je)
	if status != 200 {
		w.WriteHeader(status)
		fmt.Fprint(w, message)
		return
	}
	journal.Log(r, &je)
	w.CommitNoContent(r)
}

func publicRegister(r *request.Request, je *model.JournalEntry) (status int, errmsg string) {
	var (
		host        model.Guest
		scholaOrder int    // Schola order number
		tprice      int    // ticket item price (cents)
		tsku        string // ticket item SKU
		qty         int    // quantity ordered
	)

	// Get customer.  This also ensures we are connected to Stripe.
	if qty, status, errmsg = publicRegisterCustomer(r, &host); status != 200 {
		return status, errmsg
	}

	// Next get ticket data (price and SKU).
	tprice = model.FetchItem(r.Tx, 1).Amount
	tsku = config.TicketSKU

	// Get an order number from the public site back end.
	if scholaOrder = gstripe.GetScholaOrderNumber(); scholaOrder == 0 {
		return 500, ""
	}

	// Issue the credit card charge.
	status, errmsg = gstripe.ChargeStripe(&host, "cardEntry", "Gala Registration", tsku, scholaOrder, qty, tprice*qty)
	if status != 200 {
		return status, errmsg
	}

	// Register the guests and record the purchases.
	publicRegisterGuests(r, je, &host, errmsg, scholaOrder, tprice, qty)

	// Finally, send the email.
	publicEmailRegistrations(r, &host, scholaOrder, qty, tprice)
	return 200, ""
}

func publicRegisterCustomer(r *request.Request, guest *model.Guest) (qty, status int, errmsg string) {
	var (
		err        error
		cardSource = r.FormValue("cardSource")
	)
	guest.Name = strings.TrimSpace(r.FormValue("name1"))
	guest.Email = strings.TrimSpace(r.FormValue("email1"))
	guest.Address = strings.TrimSpace(r.FormValue("address"))
	guest.City = strings.TrimSpace(r.FormValue("city"))
	guest.State = strings.TrimSpace(r.FormValue("state"))
	guest.Zip = strings.TrimSpace(r.FormValue("zip"))
	if qty, err = strconv.Atoi(r.FormValue("qty")); err != nil || qty < 1 {
		log.Print("register-fail invalid qty")
		return 0, 500, ""
	}
	if guest.Name == "" || guest.Email == "" || guest.Address == "" || guest.City == "" || guest.State == "" ||
		guest.Zip == "" || cardSource == "" {
		log.Print("register-fail missing details")
		return 0, 500, ""
	}
	status, errmsg = gstripe.FindOrCreateCustomer(guest, cardSource)
	return qty, status, errmsg
}

func publicRegisterGuests(r *request.Request, je *model.JournalEntry, host *model.Guest, card string, scholaOrder, tprice, qty int) {
	var (
		guest    model.Guest
		purchase model.Purchase
	)

	// Set up the purchases.
	purchase = model.Purchase{
		ItemID:             1,
		Amount:             tprice,
		PaymentDescription: card,
		ScholaOrder:        scholaOrder,
		PaymentTimestamp:   time.Now().Format(time.RFC3339),
	}

	// Register the host.
	host.Sortname = sortname(host.Name)
	host.Requests = strings.TrimSpace(r.FormValue("requests"))
	host.Save(r.Tx, je)
	purchase.PayerID = host.ID
	purchase.GuestID = host.ID
	purchase.Save(r.Tx, je)

	// Register the other guests.
	guest.PartyID = host.PartyID
	guest.Requests = host.Requests
	for i := 2; i <= qty; i++ {
		guest.Name = strings.TrimSpace(r.FormValue(fmt.Sprintf("name%d", i)))
		if guest.Name == "" {
			guest.Name = fmt.Sprintf("%s Guest %d", host.Name, i-1)
			guest.Sortname = fmt.Sprintf("%s Guest %d", host.Sortname, i-1)
		} else {
			guest.Sortname = sortname(guest.Name)
		}
		guest.Email = strings.TrimSpace(r.FormValue(fmt.Sprintf("email%d", i)))
		guest.ID = 0 // force new creation
		guest.Save(r.Tx, je)
		purchase.GuestID = guest.ID
		purchase.ID = 0 // force new creation
		purchase.Save(r.Tx, je)
	}
}

var knownSuffixes = []string{" jr", " jr.", " sr", " sr.", " iii", " md", " m.d."}

func sortname(name string) string {
	var suffix string
	name = strings.TrimSpace(name)
	if idx := strings.IndexByte(name, ','); idx >= 0 {
		name, suffix = name[:idx], name[idx:]
	} else {
		lower := strings.ToLower(name)
		for _, s := range knownSuffixes {
			if strings.HasSuffix(lower, s) {
				name, suffix = name[:len(name)-len(s)], name[len(name)-len(s):]
				break
			}
		}
	}
	name = strings.TrimSpace(name)
	if idx := strings.LastIndexByte(name, ' '); idx >= 0 {
		return name[idx+1:] + ", " + name[:idx] + suffix
	}
	return name + suffix
}

func publicEmailRegistrations(r *request.Request, host *model.Guest, scholaOrder, qty, tprice int) {
	var (
		emailTo   []string
		cmd       *exec.Cmd
		pipe      io.WriteCloser
		missingNE bool
		err       error
	)

	emailTo = append([]string{}, config.EmailTo...)
	emailTo = append(emailTo, host.Email)
	cmd = exec.Command("/home/scsv/bin/send-email", emailTo...)
	if pipe, err = cmd.StdinPipe(); err != nil {
		log.Printf("register: can't pipe to send-email: %s", err)
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		log.Printf("register: can't start send-email: %s", err)
		return
	}

	fmt.Fprintf(pipe, `From: Schola Cantorum Web Site <admin@scholacantorum.org>
To: %s <%s>
Reply-To: info@scholacantorum.org
Subject: Schola Cantorum Order #%d

<p>Dear %s,</p><p>We confirm your registration for `,
		host.Name, host.Email, scholaOrder, html.EscapeString(host.Name))
	if qty > 1 {
		fmt.Fprintf(pipe, "%d seats at ", qty)
	}
	fmt.Fprintf(pipe, `%s, on %s, for `, config.GalaTitle, config.GalaDate)
	if qty > 1 {
		fmt.Fprintf(pipe, `$%d each`, tprice/100)
	} else {
		fmt.Fprintf(pipe, `$%d`, tprice/100)
	}
	fmt.Fprintf(pipe, `. This event starts at %s at %s, %s (see <a href="%s">map</a>).`,
		config.GalaStartTime, config.GalaVenue, config.GalaAddress, config.GalaMapURL)
	if qty > 1 {
		fmt.Fprintf(pipe, ` The total charge to your card was $%d.`, tprice*qty/100)
	}
	fmt.Fprint(pipe, ` Thank you for your support of Schola Cantorum!</p>`)
	for i := 2; i <= qty; i++ {
		if strings.TrimSpace(r.FormValue(fmt.Sprintf("name%d", i))) == "" ||
			strings.TrimSpace(r.FormValue(fmt.Sprintf("email%d", i))) == "" {
			missingNE = true
			break
		}
	}
	if missingNE {
		if qty > 2 {
			fmt.Fprintf(pipe, `<p>Please notify us of your guests’ names and email addresses, no later than %s.  You can reply to this email with that information, or call our office at (650) 254-1700.</p>`, config.GalaGuestInfoDeadline)
		} else {
			fmt.Fprintf(pipe, `<p>Please notify us of your guest’s name and email address, no later than %s.  You can reply to this email with that information, or call our office at (650) 254-1700.</p>`, config.GalaGuestInfoDeadline)
		}
	}
	fmt.Fprint(pipe, `<p>Sincerely yours,<br>Schola Cantorum</p><p>Web: <a href="https://scholacantorum.org">scholacantorum.org</a><br>Email: <a href="mailto:info@scholacantorum.org">info@scholacantorum.org</a><br>Phone: (650) 254-1700</p>`)
	pipe.Close()
}

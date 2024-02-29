package guest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
	"github.com/scholacantorum/gala-backend/sendmail"
)

type orderInfo struct {
	id       int
	customer string
	card     string
	pmtmeth  string
	name     string
	email    string
	total    int
}

// ServeRegister handles requests starting with /register.  The only supported
// one is POST /register, which is called by the public Schola web site when
// someone registers for the Gala.
func ServeRegister(w *request.ResponseWriter, r *request.Request) {
	var (
		origin  string
		oinfo   *orderInfo
		message string
		guests  []*model.Guest
		je      model.JournalEntry
		missing bool
	)
	if head, _ := request.ShiftPath(r.URL.Path); head != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	origin = config.Get("registerOrigin")
	if origin != "*" && r.Header.Get("Origin") != origin {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// Charge the order in Schola's ordering system.
	if oinfo, message = chargePublicRegister(r); message != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	if oinfo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Save the registration(s) in our database.
	guests, missing = publicRegister(r, oinfo, &je)
	journal.Log(r, &je)
	if err := r.Tx.Commit(); err != nil {
		panic(err)
	}
	// Send the registration confirmation email.
	publicRegisterReceipt(oinfo, guests, missing)
	// The registration form is expecting to get an ID back; that's its
	// indication of success.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"id":%d}`, oinfo.id)
}

func chargePublicRegister(r *request.Request) (*orderInfo, string) {
	type responsedata struct {
		Error    string `json:"error"`
		ID       int    `json:"id"`
		Customer string `json:"customer"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Payments []struct {
			Method   string `json:"method"`
			StripePM string `json:"stripePM"`
			Amount   int    `json:"amount"`
		} `json:"payments"`
	}
	var (
		resp  *http.Response
		rdata responsedata
		err   error
	)
	r.ParseMultipartForm(1048576) // just in case not already done
	r.Form.Set("saveForReuse", "true")
	resp, err = http.PostForm(config.Get("ordersURL")+"/payapi/order", r.Form)
	if err != nil {
		log.Printf("Post registration form to orders failed: %s", err)
		return nil, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("Post registration form to orders failed: %d %s", resp.StatusCode, resp.Status)
		return nil, ""
	}
	if err = json.NewDecoder(resp.Body).Decode(&rdata); err != nil {
		log.Printf("Can't parse orders response: %s", err)
		return nil, ""
	}
	if rdata.Error != "" {
		return nil, rdata.Error
	}
	if len(rdata.Payments) != 1 {
		log.Printf("Order has %d payments, expected 1", len(rdata.Payments))
		return nil, ""
	}
	return &orderInfo{
		id:       rdata.ID,
		customer: rdata.Customer,
		card:     rdata.Payments[0].Method,
		pmtmeth:  rdata.Payments[0].StripePM,
		name:     rdata.Name,
		email:    rdata.Email,
		total:    rdata.Payments[0].Amount,
	}, ""
}

func publicRegister(r *request.Request, oinfo *orderInfo, je *model.JournalEntry) (guests []*model.Guest, missing bool) {
	var (
		host     model.Guest
		guest    *model.Guest
		purchase model.Purchase
	)

	// Set up the purchases.
	purchase = model.Purchase{
		ItemID:             1,
		PaymentDescription: oinfo.card,
		ScholaOrder:        oinfo.id,
		PaymentTimestamp:   time.Now().Format(time.RFC3339),
	}

	// Register the host.
	if host.Name = strings.TrimSpace(r.FormValue("line1.guestName")); host.Name == "" {
		host.Name = strings.TrimSpace(r.FormValue("name"))
	}
	if host.Email = strings.TrimSpace(r.FormValue("line1.guestEmail")); host.Email == "" {
		host.Email = strings.TrimSpace(r.FormValue("email"))
	}
	host.Address = strings.TrimSpace(r.FormValue("address"))
	host.City = strings.TrimSpace(r.FormValue("city"))
	host.State = strings.TrimSpace(r.FormValue("state"))
	host.Zip = strings.TrimSpace(r.FormValue("zip"))
	host.Phone = strings.TrimSpace(r.FormValue("phone"))
	host.Entree = strings.TrimSpace(r.FormValue("line1.option"))
	if host.Entree == "" {
		missing = true
	}
	host.Sortname = sortname(host.Name)
	host.Requests = strings.TrimSpace(r.FormValue("cNote"))
	host.StripeCustomer = oinfo.customer
	host.StripeSource = oinfo.pmtmeth
	host.StripeDescription = oinfo.card
	host.Save(r.Tx, je)
	guests = append(guests, &host)
	purchase.PayerID = host.ID
	purchase.GuestID = host.ID
	purchase.Amount, _ = strconv.Atoi(r.FormValue("line1.price"))
	purchase.Save(r.Tx, je)

	// Register the other guests.
	for i := 2; true; i++ {
		guest = new(model.Guest)
		guest.PartyID = host.PartyID
		guest.Requests = host.Requests
		prefix := fmt.Sprintf("line%d.", i)
		if r.FormValue(prefix+"product") == "" {
			break
		}
		guest.Name = strings.TrimSpace(r.FormValue(prefix + "guestName"))
		if guest.Name == "" {
			guest.Name = fmt.Sprintf("%s Guest %d", host.Name, i-1)
			guest.Sortname = fmt.Sprintf("%s Guest %d", host.Sortname, i-1)
			missing = true
		} else {
			guest.Sortname = sortname(guest.Name)
		}
		guest.Email = strings.TrimSpace(r.FormValue(prefix + "guestEmail"))
		guest.Entree = strings.TrimSpace(r.FormValue(prefix + "option"))
		if guest.Entree == "" {
			missing = true
		}
		guest.ID = 0 // force new creation
		guest.Save(r.Tx, je)
		guests = append(guests, guest)
		purchase.GuestID = guest.ID
		purchase.Amount, _ = strconv.Atoi(r.FormValue(prefix + "price"))
		purchase.ID = 0 // force new creation
		purchase.Save(r.Tx, je)
	}
	return guests, missing
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

func publicRegisterReceipt(oinfo *orderInfo, guests []*model.Guest, missing bool) {
	var (
		message sendmail.Message
		addr    mail.Address
		tb      bytes.Buffer
		hb      bytes.Buffer
		tw      *tabwriter.Writer
	)
	message.From = "Schola Cantorum <admin@scholacantorum.org>"
	addr.Name = oinfo.name
	addr.Address = oinfo.email
	message.To = []string{addr.String()}
	message.SendTo = []string{addr.Address, "admin@scholacantorum.org"}
	if bcc := config.Get("receiptBCC"); bcc != "" {
		message.SendTo = append(message.SendTo, strings.Split(bcc, ",")...)
	}
	message.Subject = fmt.Sprintf("Schola Cantorum Order #%d", oinfo.id)
	message.Images = [][]byte{sendmail.ScholaLogoPNG}

	io.WriteString(&tb, `Dear Fabulous Schola Supporter,

We are overjoyed that you will be joining us for our annual party and
fundraiser, “Springtime Renaissance”, on Saturday, May 18, 2024, at Saratoga
Country Club, 21990 Prospect Road, Saratoga.  The festivities commence at 6:00pm
and will continue until 10:00pm.

`)
	io.WriteString(&hb, `<!DOCTYPE html><html><head><style>p{margin:0}p+p,table+p,pre+p{margin-top:1em}table{border-collapse:collapse;margin-top:0.75em}td,th{text-align:left;padding:0.25em 1em 0 0;line-height:1}th{font-weight:normal;text-decoration:underline}pre{margin:0}</style><body style="margin:0"><div style="width:600px;margin:0 auto"><div style="margin-bottom:24px"><img src="cid:IMG0" alt="[Schola Cantorum]" style="border-width:0"></div><p>Dear Fabulous Schola Supporter,</p><p>We are overjoyed that you will be joining us for our annual party and fundraiser, “Springtime Renaissance”, on Saturday, May 18, 2024, at Saratoga Country Club, 21990 Prospect Road, Saratoga (see <a href="https://www.google.com/maps/place/Saratoga+Country+Club/@37.284146,-122.0706404,14z/data=!4m6!3m5!1s0x808fb4c4b0258435:0x39980b6fabeaf7de!8m2!3d37.284146!4d-122.052616!16s%2Fg%2F1tgx6vjd?entry=ttu">map</a>).  The festivities commence at 6:00pm and will continue until 10:00pm.</p>`)
	switch len(guests) {
	case 1:
		io.WriteString(&tb, "You have purchased one ticket for $175:\n\n")
		io.WriteString(&hb, "<p>You have purchased one ticket for $175:</p>")
	case 10:
		io.WriteString(&tb, "You have purchased a table for the following 10 guests at $175 per person:\n\n")
		io.WriteString(&hb, "<p>You have purchased a table for the following 10 guests at $175 per person:</p>")
	default:
		fmt.Fprintf(&tb, "You have purchased %d tickets for the following guests at $175 per person:\n\n", len(guests))
		fmt.Fprintf(&hb, "<p>You have purchased %d tickets for the following guests at $175 per person:</p>", len(guests))
	}
	tw = tabwriter.NewWriter(&tb, 0, 0, 2, ' ', 0)
	io.WriteString(tw, "\tGuest Name\tEntrée\n")
	io.WriteString(&hb, "<table><tr><th><th>Guest Name<th>Entrée</tr>")
	for i, g := range guests {
		entree := entreeName(g.Entree)
		fmt.Fprintf(tw, "%d.\t%s\t%s\n", i+1, g.Name, entree)
		fmt.Fprintf(&hb, "<tr><td>%d.<td>%s<td>%s</tr>", i+1, html.EscapeString(g.Name), html.EscapeString(entree))
	}
	tw.Flush()
	io.WriteString(&tb, "\n")
	io.WriteString(&hb, "</table>")
	if guests[0].Requests != "" {
		fmt.Fprintf(&tb, "Special Requests:\n%s\n\n", guests[0].Requests)
		fmt.Fprintf(&hb, "<p><u>Special Requests</u></p><pre>%s</pre>", html.EscapeString(guests[0].Requests))
	}
	if missing {
		io.WriteString(&tb, `We need all guest names and entree choices no later than May 5.  We would
also like to know of any dietary restrictions or seating requests.  To supply
those, or to correct any errors, please reply to this email.  You can also call
the Schola office at (650) 254–1700.

`)
		io.WriteString(&hb, `<p>We need all guest names and entree choices no later than May 5.  We would also like to know of any dietary restrictions or seating requests.  To supply those, or to correct any errors, please reply to this email.  You can also call the Schola office at (650)&nbsp;254–1700.</p>`)
	} else {
		io.WriteString(&tb, `If you need to make any corrections, or add any dietary restrictions or seating
requests, please do so by May 5.  You can reply to this email, or call the
Schola Office at (650) 254–1700.

`)
		io.WriteString(&hb, `<p>If you need to make any corrections, or add any dietary restrictions or seating requests, please do so by May 5.  You can reply to this email, or call the Schola Office at (650)&nbsp;254–1700.</p>`)
	}
	fmt.Fprintf(&tb, "For your records, you paid a total of $%d on %s by %s.\n\n", oinfo.total/100, time.Now().Format("January 2, 2006"), oinfo.card)
	fmt.Fprintf(&hb, `<p>For your records, you paid a total of $%d on %s by %s.</p>`, oinfo.total/100, time.Now().Format("January 2, 2006"), oinfo.card)
	io.WriteString(&tb, `Reservations will be held at the door; no tickets will be mailed to you.  When
you arrive, please check in at the registration table, get your program, and
provide your credit card number for purchases made at the event. There will
be complimentary champagne, wine, and soft drinks for all guests.

`)
	io.WriteString(&hb, `<p>Reservations will be held at the door; no tickets will be mailed to you.  When you arrive, please check in at the registration table, get your program, and provide your credit card number for purchases made at the event.  There will be complimentary champagne, wine, and soft drinks for all guests.</p>`)
	io.WriteString(&tb, "Musically yours,\nSchola Cantorum Silicon Valley\n\nWeb: scholacantorum.org\nEmail: info@scholacantorum.org\nPhone: (650) 254–1700\n")
	io.WriteString(&hb, `<p>Musically yours,<br>Schola Cantorum Silicon Valley<p>Web: <a href="https://scholacantorum.org">scholacantorum.org</a><br>Email: <a href="mailto:info@scholacantorum.org">info@scholacantorum.org</a><br>Phone: <a href="tel:16502541700">(650) 254–1700</a></p></div></body></html>`)
	message.Text = tb.String()
	message.HTML = hb.String()
	message.Send()
}

func entreeName(code string) string {
	switch code {
	case "":
		return "(not yet selected)"
	case "filet":
		return "Filet Mignon"
	case "bass":
		return "Sea Bass"
	case "gnocchi":
		return "Gnocchi"
	default:
		return code
	}
}

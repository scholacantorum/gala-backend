package guest

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

type orderInfo struct {
	id       int
	customer string
	card     string
	pmtmeth  string
}

// ServeRegister handles requests starting with /register.  The only supported
// one is POST /register, which is called by the public Schola web site when
// someone registers for the Gala.
func ServeRegister(w *request.ResponseWriter, r *request.Request) {
	var (
		origin  string
		oinfo   *orderInfo
		message string
		je      model.JournalEntry
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
	publicRegister(r, oinfo, &je)
	journal.Log(r, &je)
	if err := r.Tx.Commit(); err != nil {
		panic(err)
	}
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
		Payments []struct {
			Method   string `json:"method"`
			StripePM string `json:"stripePM"`
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
	return &orderInfo{rdata.ID, rdata.Customer, rdata.Payments[0].Method, rdata.Payments[0].StripePM}, ""
}

func validatePublicRegister(r *request.Request) (guests []*model.Guest) {
	var guest *model.Guest

	guest = &model.Guest{
		Name:     strings.TrimSpace(r.FormValue("line1.guestName")),
		Email:    strings.TrimSpace(r.FormValue("line1.guestEmail")),
		Address:  strings.TrimSpace(r.FormValue("address")),
		City:     strings.TrimSpace(r.FormValue("city")),
		State:    strings.TrimSpace(r.FormValue("state")),
		Zip:      strings.TrimSpace(r.FormValue("zip")),
		Phone:    strings.TrimSpace(r.FormValue("phone")),
		Requests: strings.TrimSpace(r.FormValue("cNote")),
		Entree:   strings.TrimSpace(r.FormValue("line1.option")),
	}
	if guest.Name == "" {
		guest.Name = r.FormValue("name")
	}
	if guest.Email == "" {
		guest.Email = r.FormValue("email")
	}
	guests = append(guests, guest)
	for idx := 2; true; idx++ {
		prefix := fmt.Sprintf("line%d.", idx)
		if r.FormValue(prefix+"product") == "" {
			break
		}
		guest = &model.Guest{
			Name:   strings.TrimSpace(prefix + "guestName"),
			Email:  strings.TrimSpace(prefix + "guestEmail"),
			Entree: strings.TrimSpace(prefix + "option"),
		}
		guests = append(guests, guest)
	}
	return guests
}

func publicRegister(r *request.Request, oinfo *orderInfo, je *model.JournalEntry) {
	var (
		host     model.Guest
		guest    model.Guest
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
	host.Sortname = sortname(host.Name)
	host.Requests = strings.TrimSpace(r.FormValue("cNote"))
	host.StripeCustomer = oinfo.customer
	host.StripeSource = oinfo.pmtmeth
	host.StripeDescription = oinfo.card
	host.Save(r.Tx, je)
	purchase.PayerID = host.ID
	purchase.GuestID = host.ID
	purchase.Amount, _ = strconv.Atoi(r.FormValue("line1.price"))
	purchase.Save(r.Tx, je)

	// Register the other guests.
	guest.PartyID = host.PartyID
	guest.Requests = host.Requests
	for i := 2; true; i++ {
		prefix := fmt.Sprintf("line%d.", i)
		if r.FormValue(prefix+"product") == "" {
			break
		}
		guest.Name = strings.TrimSpace(r.FormValue(prefix + "guestName"))
		if guest.Name == "" {
			guest.Name = fmt.Sprintf("%s Guest %d", host.Name, i-1)
			guest.Sortname = fmt.Sprintf("%s Guest %d", host.Sortname, i-1)
		} else {
			guest.Sortname = sortname(guest.Name)
		}
		guest.Email = strings.TrimSpace(r.FormValue(prefix + "guestEmail"))
		guest.Entree = strings.TrimSpace(r.FormValue(prefix + "option"))
		guest.ID = 0 // force new creation
		guest.Save(r.Tx, je)
		purchase.GuestID = guest.ID
		purchase.Amount, _ = strconv.Atoi(r.FormValue(prefix + "price"))
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

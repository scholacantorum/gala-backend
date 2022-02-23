package purchase

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

func serveExportPurchases(w *request.ResponseWriter, r *request.Request) {
	var (
		cw *csv.Writer
	)
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="gala-payments.csv"`)
	cw = csv.NewWriter(w)
	cw.UseCRLF = true
	cw.Write([]string{"Patron", "Email", "Address", "City", "State", "Zip", "Reg-Count", "Reg-Total", "Donations", "Auction-Items", "Auction-Paid", "Auction-Value"})
	model.FetchGuests(r.Tx, func(g *model.Guest) {
		var (
			regcount     int
			regtotal     int
			auctionItems []string
			auctionPaid  int
			auctionValue int
			donations    int
			unpaid       string
			item         *model.Item
		)
		model.FetchPurchases(r.Tx, func(p *model.Purchase) {
			if p.PaymentTimestamp == "" {
				unpaid = "NOT FULLY PAID"
			}
			item = model.FetchItem(r.Tx, p.ItemID)
			if item.IsRegistration() {
				regcount++
				regtotal += p.Amount
				return
			}
			if item.Value != 0 {
				auctionItems = append(auctionItems, item.Name)
				auctionPaid += p.Amount
				auctionValue += item.Value
			} else {
				donations += p.Amount
			}
		}, "payer=?", g.ID)
		if donations+auctionValue+regtotal == 0 {
			return
		}
		cw.Write([]string{g.Name, g.Email, g.Address, g.City, g.State, g.Zip, strconv.Itoa(regcount),
			strconv.Itoa(regtotal / 100), strconv.Itoa(donations / 100), strings.Join(auctionItems, ", "),
			strconv.Itoa(auctionPaid / 100), strconv.Itoa(auctionValue / 100), unpaid})
	}, "")
	cw.Flush()
}

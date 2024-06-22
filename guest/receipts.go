package guest

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"

	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

func serveReceipts(w *request.ResponseWriter, r *request.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Gala Receipts</title><style type="text/css"><!--
.printthis {
  color: red;
  font-size: 24px;
  font-weight: bold;
  margin: 24px;
}
.receipt {
  border-top: 1px solid black;
  padding: 24px 12px;
}
.valueNote {
  font-style: italic;
  padding-left: 2em;
  padding-bottom: 12pt;
}
.footer {
  display: none;
}
@media print {
  html, body {
    margin: 0;
  }
  .printthis {
    display: none;
  }
  .receipt {
    border-top: none;
    padding: 36pt;
    page-break-before: always;
  }
  .footer {
    display: block;
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    margin-bottom: 27px;
    font-size: 12px;
    text-align: center;
  }
}
--></style></head><body>
<p class="printthis">When printed, this page will have one receipt on each sheet of paper.</p>
<div class="footer">
650-B Fremont Avenue, Suite 321 • Los Altos CA 94024 • ScholaCantorum.org • (650) 254-1700<br>
Info@ScholaCantorum.org • Schola Cantorum is a 501(c)(3) nonprofit organization, tax ID 94-2597822
</div>
`)
	var payers []*model.Guest
	var payerPurchases = map[db.ID][]*model.Purchase{}
	model.FetchGuests(r.Tx, func(g *model.Guest) {
		var purchases []*model.Purchase
		model.FetchPurchases(r.Tx, func(p *model.Purchase) {
			copy := *p
			purchases = append(purchases, &copy)
		}, `payer=?`, g.ID)
		if len(purchases) != 0 {
			copy := *g
			payerPurchases[g.ID] = purchases
			payers = append(payers, &copy)
		}
	}, "1 ORDER BY sortname")
	sort.Slice(payers, func(i, j int) bool { return payers[i].Sortname < payers[j].Sortname })
	for _, p := range payers {
		emitReceipt(w, r, p, payerPurchases[p.ID])
	}
	fmt.Fprint(w, `</body></html>
`)
	w.Close()
}

func emitReceipt(w *request.ResponseWriter, r *request.Request, payer *model.Guest, purchases []*model.Purchase) {
	type purchase struct {
		ItemID   db.ID
		Item     string
		Amount   int
		Value    int
		Note     string
		Date     string
		Method   string
		Quantity int
	}
	var receiptData struct {
		Payer                string
		PurchaseTypes        string
		ShowTotalValue       bool
		TotalValue           int
		TotalAmount          int
		Purchases            []*purchase
		ShowRegistrationNote bool
		ShowPurchaseNote     bool
		ShowDonationNote     bool
	}
	var (
		purchasesCount int
		donations      int
		pledges        int
		types          []string
	)

	// Fill in the template data.
	receiptData.Payer = payer.Name
	receiptData.ShowTotalValue = true
	for _, p := range purchases {
		var item = model.FetchItem(r.Tx, p.ItemID)
		var purchase = purchase{
			ItemID:   p.ItemID,
			Item:     item.Name,
			Amount:   p.Amount / 100,
			Value:    item.Value / 100,
			Quantity: 1,
		}
		if item.ID == 1 { // registration
			purchase.Note = "*"
			receiptData.ShowRegistrationNote = true
		} else if item.Value != 0 {
			purchase.Note = "†"
			receiptData.ShowPurchaseNote = true
		} else {
			purchase.Note = "§"
			receiptData.ShowDonationNote = true
		}
		if item.Value > p.Amount {
			receiptData.ShowTotalValue = false
		}
		if p.PaymentTimestamp == "" {
			purchase.Amount = 0
			pledges++
		} else {
			purchase.Date = p.PaymentTimestamp[0:10]
			purchase.Method = p.PaymentDescription
			if item.Value != 0 {
				purchasesCount++
			} else {
				donations++
			}
		}
		receiptData.Purchases = append(receiptData.Purchases, &purchase)
		receiptData.TotalAmount += purchase.Amount
		receiptData.TotalValue += purchase.Value
	}
	// Combine multiple registrations with the same payment into a single
	// line item.
	for i := 1; i < len(receiptData.Purchases); {
		a := receiptData.Purchases[i-1]
		b := receiptData.Purchases[i]
		if a.Note == "*" && b.Note == "*" && a.Date == b.Date && a.Method == b.Method {
			a.Amount += b.Amount
			a.Value += b.Value
			a.Quantity += b.Quantity
			a.Item = fmt.Sprintf("Registrations (%d)", a.Quantity)
			receiptData.Purchases = append(receiptData.Purchases[:i], receiptData.Purchases[i+1:]...)
			// and don't increment i
		} else {
			i++
		}
	}
	// Generate the English phrase to describe what they did.
	if purchasesCount > 1 {
		types = append(types, "purchases")
	}
	if purchasesCount == 1 {
		types = append(types, "purchase")
	}
	if donations > 1 {
		types = append(types, "donations")
	}
	if donations == 1 {
		types = append(types, "donation")
	}
	if pledges > 1 {
		types = append(types, "pledges")
	}
	if pledges == 1 {
		types = append(types, "pledge")
	}
	switch len(types) {
	case 1:
		receiptData.PurchaseTypes = types[0]
	case 2:
		receiptData.PurchaseTypes = fmt.Sprintf("%s and %s", types[0], types[1])
	case 3:
		receiptData.PurchaseTypes = fmt.Sprintf("%s, %s, and %s", types[0], types[1], types[2])
	}
	// Suppress the total line if there's only one item.
	if purchasesCount+donations+pledges < 2 {
		receiptData.TotalAmount = 0
	}

	// Render the email template.
	payerTemplate.Execute(w, &receiptData)
}

var payerTemplate = template.Must(template.New("payer").Parse(`
<div class="receipt">
<img src="https://gala.scholacantorum.org/receipt-logo.png" style="height:72px;margin-bottom:36px">
<p>Schola Cantorum confirms the following {{ .PurchaseTypes }} from <b>{{ .Payer }}</b>:</p>
<table style="margin-bottom:12pt">
  <thead>
    <tr>
      <th style="text-align:left;vertical-align:bottom">Item</th>
      <th style="text-align:right;vertical-align:bottom;padding-left:1em">Estimated<br>Value<br>Received</th>
      <th style="text-align:right;vertical-align:bottom;padding-left:1em">Amount<br>Paid</th>
      <th style="text-align:left;vertical-align:bottom;padding-left:1em">Payment<br>Date</th>
      <th style="text-align:left;vertical-align:bottom;padding-left:1em">Payment<br>Method</th>
    </tr>
  </thead>
  <tbody>
    {{ range .Purchases }}
      <tr>
        <td>{{ .Item }}</td>
        <td style="text-align:right">${{ .Value }}<sup>{{ .Note }}</sup></td>
        {{ if .Amount }}
          <td style="text-align:right">${{ .Amount }}</td>
          <td style="padding-left:1em;white-space:nowrap">{{ .Date }}</td>
          <td style="padding-left:1em;white-space:nowrap">{{ .Method }}</td>
        {{ else }}
          <td colspan="3" style="padding-left:1em"><i>not yet paid</i></td>
        {{ end }}
      </tr>
    {{ end }}
    {{ if .TotalAmount }}
      {{ if .ShowTotalValue }}
        <tr>
          <td style="font-weight:bold;text-align:right">TOTAL</td>
  	  <td style="font-weight:bold;text-align:right;border-top:thin solid black">${{ .TotalValue }}</td>
  	  <td style="font-weight:bold;text-align:right;border-top:thin solid black">${{ .TotalAmount }}</td>
        </tr>
      {{ else }}
        <tr>
          <td colspan="2" style="font-weight:bold;text-align:right">TOTAL</td>
  	  <td style="font-weight:bold;text-align:right;border-top:thin solid black">${{ .TotalAmount }}</td>
        </tr>
      {{ end }}
    {{ end }}
  </tbody>
</table>
{{ if .ShowRegistrationNote }}
  <div><sup>*</sup> The "estimated value received" for registration is our good faith estimate.  It may not reflect the fair market value.</div>
{{ end }}
{{ if .ShowPurchaseNote }}
  <div><sup>†</sup> The "estimated value received" for auction items is provided by the donor.  It may not reflect the fair market value.</div>
{{ end }}
{{ if .ShowDonationNote }}
  <div><sup>§</sup> No goods or services were received in return for this donation.</div>
{{ end }}
<p>
  Schola Cantorum is a 501(c)(3) tax-exempt organization.
  Our federal tax ID number is 94‑2597822.
</p>
<p>Thank you for your support of Schola Cantorum!</p>
</div>
`))

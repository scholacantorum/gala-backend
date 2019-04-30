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
@media print {
  .printthis {
    display: none;
  }
  .receipt {
    border-top: none;
    padding: 144pt 36pt 36pt;
    page-break-before: always;
  }
}
--></style></head><body>
<p class="printthis">Please load the printer with letterhead paper and print this page.</p>
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
		Item   string
		Bidder string
		Amount int
		Value  int
		Date   string
		Method string
	}
	var receiptData struct {
		Payer           string
		Date            string
		Method          string
		MultipleBidders bool
		HasValues       bool
		ShowTotalValue  bool
		TotalValue      int
		TotalAmount     int
		Purchases       []purchase
		Pledges         []purchase
	}
	var (
		bidder           string
		multiplePayments bool
	)

	// Fill in the template data.
	receiptData.Payer = payer.Name
	receiptData.ShowTotalValue = true
	for _, p := range purchases {
		var item = model.FetchItem(r.Tx, p.ItemID)
		var purchase = purchase{
			Item:   item.Name,
			Bidder: model.FetchGuest(r.Tx, p.GuestID).Name,
			Amount: p.Amount / 100,
			Value:  item.Value / 100,
		}
		if bidder == "" {
			bidder = purchase.Bidder
		} else if bidder != purchase.Bidder {
			receiptData.MultipleBidders = true
		}
		if item.Value != 0 {
			receiptData.HasValues = true
		}
		if p.PaymentTimestamp == "" {
			receiptData.Pledges = append(receiptData.Pledges, purchase)
		} else {
			purchase.Date = p.PaymentTimestamp[0:10]
			purchase.Method = p.PaymentDescription
			if p.ScholaOrder != 0 {
				purchase.Method = fmt.Sprintf("%s (Schola order #%d)", purchase.Method, p.ScholaOrder)
			}
			receiptData.Purchases = append(receiptData.Purchases, purchase)
			if item.Value > p.Amount {
				receiptData.ShowTotalValue = false
			}
			receiptData.TotalAmount += purchase.Amount
			receiptData.TotalValue += purchase.Value
		}
	}
	for i := len(receiptData.Purchases) - 2; i >= 0; i-- {
		a := receiptData.Purchases[i]
		b := receiptData.Purchases[i+1]
		if a.Date == b.Date && a.Method == b.Method {
			receiptData.Purchases[i+1].Date = ""
			receiptData.Purchases[i+1].Method = ""
		} else {
			multiplePayments = true
		}
	}
	if len(receiptData.Purchases) > 0 && !multiplePayments {
		p := receiptData.Purchases[0]
		receiptData.Date = p.Date
		receiptData.Method = p.Method
		receiptData.Purchases[0].Date = ""
		receiptData.Purchases[0].Method = ""
	}

	// Render the email template.
	payerTemplate.Execute(w, &receiptData)
}

var paidTableTemplate = `
<table>
  <thead>
    <tr>
      <th style="text-align:left">Item</th>
      {{ if .MultipleBidders }}
	<th style="text-align:left;padding-left:1em">Bidder</th>
      {{ end }}
      <th style="text-align:right;padding-left:1em">Amount Paid</th>
      {{ if .HasValues }}
        <th style="text-align:right;padding-left:1em">Estimated Value</th>
      {{ end }}
    </tr>
  </thead>
  <tbody>
    {{ range .Purchases }}
      {{ if .Date }}
      <tr>
        <td colspan="4" style="font-style:italic;color:#444">Paid on {{ .Date }} by {{ .Method }}:</td>
      </tr>
      {{ end }}
      <tr>
        <td>{{ .Item }}</td>
	{{ if $.MultipleBidders }}
	  <td style="padding-left:1em">{{ .Bidder }}</td>
	{{ end }}
	<td style="text-align:right">{{ .Amount }}</td>
	{{ if $.HasValues }}
	  <td style="text-align:right">{{ .Value }}</td>
	{{ end }}
      </tr>
    {{ end }}
    {{ if gt (len .Purchases ) 1 }}
      <tr>
        {{ if .MultipleBidders }}
	  <td></td>
	{{ end }}
        <td style="font-weight:bold;text-align:right">TOTAL</td>
	<td style="font-weight:bold;text-align:right;border-top:thin solid black">${{ .TotalAmount }}</td>
	{{ if and .HasValues .ShowTotalValue }}
	  <td style="font-weight:bold;text-align:right;border-top:thin solid black">${{ .TotalValue }}</td>
	{{ end }}
      </tr>
    {{ end }}
  </tbody>
</table>
`
var pledgeTableTemplate = `
<table>
  <thead>
    <tr>
      <th style="text-align:left">Item</th>
      {{ if .MultipleBidders }}
	<th style="text-align:left;padding-left:1em">Bidder</th>
      {{ end }}
      <th style="text-align:right;padding-left:1em">Amount Pledged</th>
      {{ if .HasValues }}
        <th style="text-align:right;padding-left:1em">Estimated Value</th>
      {{ end }}
    </tr>
  </thead>
  <tbody>
    {{ range .Pledges }}
      <tr>
        <td>{{ .Item }}</td>
	{{ if $.MultipleBidders }}
	  <td style="padding-left:1em">{{ .Bidder }}</td>
	{{ end }}
	<td style="text-align:right">{{ .Amount }}</td>
	{{ if $.HasValues }}
	  <td style="text-align:right">{{ .Value }}</td>
	{{ end }}
      </tr>
    {{ end }}
  </tbody>
</table>
`
var payerTemplate = template.Must(template.New("payer").Parse(`
<div class="receipt">
{{ if .Purchases }}
<p>Schola Cantorum confirms the following purchases and donations from {{ .Payer }}{{ if .Date }}, paid on {{ .Date }} by {{ .Method }}{{ end }}:</p>
{{ template "paidTable" . }}
{{ end }}
{{ if .Pledges }}
<p>Schola Cantorum acknowledges the following pledges from {{ .Payer }}, which we look forward to receiving:</p>
{{ template "pledgeTable" . }}
{{ end }}
<p>
  Schola Cantorum is a 501(c)(3) tax-exempt organization.
  Our federal tax ID number is 94‑2597822.
  {{ if not .HasValues }}
    No goods or services were provided in return for your donation.
  {{ end }}
</p>
<p>Thank you for your support of Schola Cantorum!</p>
</div>
`))

func init() {
	template.Must(payerTemplate.New("paidTable").Parse(paidTableTemplate))
	template.Must(payerTemplate.New("pledgeTable").Parse(pledgeTableTemplate))
}

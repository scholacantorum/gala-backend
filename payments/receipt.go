package payments

import (
	"bytes"
	"fmt"
	"html/template"
	"net/mail"
	"strings"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
	"github.com/scholacantorum/gala-backend/sendmail"
)

func sendChargeReceipt(r *request.Request, onum int, payer *model.Guest, purchases []*model.Purchase) {
	type purchase struct {
		Item   string
		Bidder string
		Amount int
		Value  int
	}
	var emailData struct {
		Payer           string
		EventTitle      string
		EventDate       string
		Card            string
		MultipleBidders bool
		TotalValue      int
		TotalAmount     int
		Deductible      int
		Purchases       []purchase
	}
	var (
		message sendmail.Message
		addr    mail.Address
		hb      bytes.Buffer
	)

	// Fill in the template data.
	emailData.Payer = payer.Name
	emailData.EventTitle = config.Get("galaTitle")
	emailData.EventDate = config.Get("galaDate")
	emailData.Card = payer.StripeDescription
	emailData.Purchases = make([]purchase, len(purchases))
	for i, p := range purchases {
		var item = model.FetchItem(r.Tx, p.ItemID)
		emailData.Purchases[i] = purchase{
			Item:   item.Name,
			Bidder: model.FetchGuest(r.Tx, p.GuestID).Name,
			Amount: p.Amount / 100,
			Value:  item.Value / 100,
		}
		if item.Value > p.Amount {
			emailData.Purchases[i].Value = p.Amount / 100
		}
		if emailData.Purchases[i].Bidder != emailData.Purchases[0].Bidder {
			emailData.MultipleBidders = true
		}
		emailData.TotalAmount += emailData.Purchases[i].Amount
		emailData.TotalValue += emailData.Purchases[i].Value
	}
	emailData.Deductible = emailData.TotalAmount - emailData.TotalValue
	emailTemplate.Execute(&hb, &emailData)

	// Start the email.
	message.From = "Schola Cantorum <admin@scholacantorum.org>"
	addr.Name = payer.Name
	addr.Address = payer.Email
	message.SendTo = strings.Split(config.Get("emailTo"), ",")
	message.SendTo = append(message.SendTo, payer.Email)
	message.To = []string{addr.String()}
	message.Subject = fmt.Sprintf("Schola Cantorum Order #%d", onum)
	message.ReplyTo = "Schola Cantorum <info@scholacantorum.org>"
	message.Images = [][]byte{sendmail.ScholaLogoPNG}
	message.HTML = hb.String()
	message.Send()
}

var tableTemplate = `
<table>
  <thead>
    <tr>
      <th style="text-align:left">Item</th>
      {{ if .MultipleBidders }}
	<th style="text-align:left;padding-left:1em">Bidder</th>
      {{ end }}
      <th style="text-align:right;padding-left:1em">Amount Paid</th>
      {{ if .TotalValue }}
        <th style="text-align:right;padding-left:1em">Estimated Value</th>
      {{ end }}
    </tr>
  </thead>
  <tbody>
    {{ range .Purchases }}
      <tr>
        <td>{{ .Item }}</td>
	{{ if $.MultipleBidders }}
	  <td style="padding-left:1em">{{ .Bidder }}</td>
	{{ end }}
	<td style="text-align:right">{{ .Amount }}</td>
	{{ if $.TotalValue }}
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
	{{ if .TotalValue }}
	  <td style="font-weight:bold;text-align:right;border-top:thin solid black">${{ .TotalValue }}</td>
	{{ end }}
      </tr>
    {{ end }}
  </tbody>
</table>
<p>
  Schola Cantorum is a 501(c)(3) tax-exempt organization.
  Our federal tax ID number is 94‑2597822.
  {{ if not .TotalValue }}
    No goods or services were provided in return for your donation.
  {{ end }}
</p>
`
var emailTemplate = template.Must(template.New("email").Parse(`
<!DOCTYPE html><html><head><body style="margin:0"><div style="width:600px;margin:0 auto"><div style="margin-bottom:24px"><img src="CID:IMG0" alt="[Schola Cantorum]" style="border-width:0"></div>
<p>Dear {{ .Payer }},</p>
<p>We confirm the following purchases and donations made at {{ .EventTitle }} on {{ .EventDate }}, charged to {{ .Card }}:</p>
{{ template "table" . }}
<p>Thank you for your support of Schola Cantorum!</p>
<p>
  Sincerely yours,<br>
  Schola Cantorum
</p>
<p>
  Web: <a href="https://scholacantorum.org">scholacantorum.org</a><br>
  Email: <a href="mailto:info@scholacantorum.org">info@scholacantorum.org</a><br>
  Phone: (650) 254-1700
</p></div></body></html>
`))

func init() {
	template.Must(emailTemplate.New("table").Parse(tableTemplate))
}

package payments

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
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
		MultipleBidders bool
		TotalValue      int
		TotalAmount     int
		Deductible      int
		Purchases       []purchase
	}
	var (
		emailTo []string
		cmd     *exec.Cmd
		pipe    io.WriteCloser
		err     error
	)

	// Fill in the template data.
	emailData.Payer = payer.Name
	emailData.EventTitle = config.GalaTitle
	emailData.EventDate = config.GalaDate
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

	// Start the email.
	emailTo = append([]string{}, config.EmailTo...)
	emailTo = append(emailTo, payer.Email)
	cmd = exec.Command("/home/scsv/bin/send-email", emailTo...)
	if pipe, err = cmd.StdinPipe(); err != nil {
		log.Printf("receipt: can't pipe to send-email: %s", err)
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		log.Printf("receipt: can't start send-email: %s", err)
		return
	}
	fmt.Fprintf(pipe, `From: Schola Cantorum Web Site <admin@scholacantorum.org>
To: %s <%s>
Reply-To: info@scholacantorum.org
Subject: Schola Cantorum Order #%d

`,
		payer.Name, payer.Email, onum)

	// Render the email template.
	emailTemplate.Execute(pipe, &emailData)
	pipe.Close()
	if err = cmd.Wait(); err != nil {
		log.Printf("receipt: send-email failed: %s", err)
	}
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
        <th style="text-align:right;padding-left:1em">Value Received</th>
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
{{ if eq .TotalAmount .TotalValue }}
  <p>
    The total value received is ${{ .TotalValue }}.
    Your purchases are not tax deductible.
  </p>
{{ else if .TotalValue }}
  <p>
    The total value received is ${{ .TotalValue }}.
    This amount is not tax deductible.
    The balance of your payment, ${{ .Deductible }}, is a tax-deductible donation.
  </p>
{{ else }}
  <p>
    No goods or services were provided in return for this payment.
    Your payment of ${{ .TotalAmount }} is a tax deductible donation.
  </p>
{{ end }}
`
var emailTemplate = template.Must(template.New("email").Parse(`
<p>Dear {{ .Payer }},</p>
<p>We confirm the following purchases and donations made at {{ .EventTitle }} on {{ .EventDate }}:</p>
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
</p>
`))

func init() {
	template.Must(emailTemplate.New("table").Parse(tableTemplate))
}

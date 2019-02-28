package payments

import (
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

func sendChargeReceipt(r *request.Request, onum int, payer *model.Guest, purchases []*model.Purchase) {
	var (
		emailTo     []string
		cmd         *exec.Cmd
		pipe        io.WriteCloser
		items       []*model.Item
		totalAmount int
		totalValue  int
		err         error
	)
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
		log.Printf("register: can't start send-email: %s", err)
		return
	}
	items = make([]*model.Item, len(purchases))
	for i, purchase := range purchases {
		items[i] = model.FetchItem(r.Tx, purchase.ItemID)
		totalAmount += purchase.Amount
		if items[i].Value < purchase.Amount {
			totalValue += items[i].Value
		} else {
			totalValue += purchase.Amount
		}
	}
	fmt.Fprintf(pipe, `From: Schola Cantorum Web Site <admin@scholacantorum.org>
To: %s <%s>
Reply-To: info@scholacantorum.org
Subject: Schola Cantorum Order #%d

<p>Dear %s,</p><p>We confirm the following purchases and donations made at %s on %s:</p><table><thead><tr><th style="text-align:left">Item</th><th style="text-align:right">Amount Paid</th>`,
		payer.Name, payer.Email, onum, html.EscapeString(payer.Name), config.GalaTitle, config.GalaDate)
	if totalValue > 0 {
		fmt.Fprint(pipe, `<th style="text-align:right">Value Received</th></tr>`)
	}
	fmt.Fprint(pipe, `</thead><tbody>`)
	for i, purchase := range purchases {
		if totalValue > 0 {
			if items[i].Value < purchase.Amount {
				fmt.Fprintf(pipe, `<tr><td>%s</td><td style="text-align:right">$%d</td><td style="text-align:right">$%d</td></tr>`,
					html.EscapeString(items[i].Name), purchase.Amount/100, items[i].Value/100)
			} else {
				fmt.Fprintf(pipe, `<tr><td>%s</td><td style="text-align:right">$%d</td><td style="text-align:right">$%d</td></tr>`,
					html.EscapeString(items[i].Name), purchase.Amount/100, purchase.Amount/100)
			}
		} else {
			fmt.Fprintf(pipe, `<tr><td>%s</td><td style="text-align:right">$%d</td></tr>`,
				html.EscapeString(items[i].Name), purchase.Amount/100)
		}
	}
	if len(purchases) > 1 {
		if totalValue > 0 {
			fmt.Fprintf(pipe, `<tr><td style="font-weight:bold;text-align:right">TOTAL</td><td style="font-weight:bold;text-align:right">$%d</td><td style="font-weight:bold;text-align:right">$%d</td></tr>`,
				totalAmount/100, totalValue/100)
		} else {
			fmt.Fprintf(pipe, `<tr><td style="font-weight:bold;text-align:right">TOTAL</td><td style="font-weight:bold;text-align:right">$%d</td></tr>`,
				totalAmount/100)
		}
	}
	fmt.Fprint(pipe, `</tbody></table>`)
	switch {
	case totalValue == totalAmount:
		fmt.Fprintf(pipe, `<p>The total value received is $%d.  Your purchases are not tax deductible.</p>`,
			totalValue/100)
	case totalValue > 0:
		fmt.Fprintf(pipe, `<p>The total value of received is $%d.  This amount is not tax deductible.  The balance of your payment, $%d, is a tax deductible donation.</p>`,
			totalValue/100, (totalAmount-totalValue)/100)
	default:
		fmt.Fprintf(pipe, `<p>No goods or services were provided in return for this payment.  Your payment of $%d is a tax deductible donation.</p>`,
			totalAmount/100)
	}
	fmt.Fprint(pipe, `<p>Thank you for your support of Schola Cantorum!</p>`)
	fmt.Fprint(pipe, `<p>Sincerely yours,<br>Schola Cantorum</p><p>Web: <a href="https://scholacantorum.org">scholacantorum.org</a><br>Email: <a href="mailto:info@scholacantorum.org">info@scholacantorum.org</a><br>Phone: (650) 254-1700</p>`)
	pipe.Close()
}

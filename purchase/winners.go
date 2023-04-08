package purchase

import (
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/jung-kurt/gofpdf"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

type itemt struct {
	itemName   string
	bidderNum  int
	bidderName string
	prepaid    bool
}

func serveAuctionWinners(w *request.ResponseWriter, r *request.Request) {
	var (
		items []*itemt
		pdf   *gofpdf.Fpdf
	)
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="auction-winners.pdf"`)
	// Gather the data.
	model.FetchPurchases(r.Tx, func(p *model.Purchase) {
		var item = model.FetchItem(r.Tx, p.ItemID)
		if item.ID == 1 || item.Value == 0 {
			// Registration, Donation, Fund-a-Need, etc.
			return
		}
		var winner = model.FetchGuest(r.Tx, p.GuestID)
		var payer = model.FetchGuest(r.Tx, p.PayerID)
		items = append(items, &itemt{
			itemName:   item.Name,
			bidderNum:  winner.Bidder,
			bidderName: winner.Name,
			prepaid:    payer.UseCard,
		})
	}, "")
	sort.Slice(items, func(i, j int) bool {
		return items[i].itemName < items[j].itemName
	})
	// Create a PDF document.
	pdf = gofpdf.New("P", "pt", "Letter", "")
	pdf.SetMargins(36, 36, 36) // 36pt = ½ inch
	pdf.SetAutoPageBreak(true, 36)
	pdf.AddPage()
	renderHeading(pdf)
	for i, item := range items {
		renderItem(pdf, i, item)
	}
	if err := pdf.Error(); err != nil {
		log.Printf("PDF ERROR: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	pdf.Output(w)
}

func renderHeading(pdf *gofpdf.Fpdf) {
	pdf.SetFont("helvetica", "B", 24)
	pdf.MoveTo(36, 24)
	pdf.CellFormat(540, 24, "Auction Winners", "", 1, "CM", false, 0, "")
	pdf.SetFillColor(64, 64, 64)
	pdf.SetTextColor(255, 255, 255)
	pdf.Rect(36, 61, 540, 18, "F")
	pdf.SetFont("helvetica", "B", 14)
	pdf.MoveTo(36, 62)
	pdf.CellFormat(234, 14, "Auction Item", "", 1, "LM", false, 0, "")
	pdf.MoveTo(324, 62)
	pdf.CellFormat(218, 14, "Winning Bidder", "", 1, "LM", false, 0, "")
	pdf.SetFont("helvetica", "", 14)
	pdf.SetFillColor(224, 224, 224)
	pdf.SetTextColor(0, 0, 0)
}

func renderItem(pdf *gofpdf.Fpdf, idx int, item *itemt) {
	y := 80.0 + 18.0*float64(idx)
	if idx%2 == 0 {
		pdf.Rect(36, y-1, 540, 18, "F")
	}
	pdf.MoveTo(36, y)
	pdf.CellFormat(234, 14, item.itemName, "", 1, "LM", false, 0, "")
	pdf.MoveTo(324, y)
	pdf.CellFormat(27, 14, fmt.Sprintf("%x", item.bidderNum), "", 1, "RM", false, 0, "")
	pdf.MoveTo(360, y)
	pdf.CellFormat(180, 14, item.bidderName, "", 1, "LM", false, 0, "")
	if !item.prepaid {
		pdf.MoveTo(540, y)
		pdf.CellFormat(36, 14, "!!", "", 1, "LM", false, 0, "")
	}
}

package guest

import (
	"log"
	"net/http"
	"sort"

	"github.com/jung-kurt/gofpdf"

	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

// serveProgramLabels generates a PDF of labels for the programs.  The PDF is
// designed to be printed onto a sheet of 1" x 2.625" adhesive labels, with 10
// rows of 3 labels on each sheet.  The sheets have 0.5" margins at top and
// bottom, and no vertical gutter between labels; they have 0.15625" (5/32")
// margins on left and right and 0.15625" horizontal gutters between the labels.
// Each label gets the guest's name (in bold face), their table number, and
// their bidder number.
func serveProgramLabels(w *request.ResponseWriter, r *request.Request) {
	var (
		guests []*model.Guest
		pdf    *gofpdf.Fpdf
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
	w.Header().Set("Content-Disposition", `attachment; filename="program-labels.pdf"`)

	// First, get the sorted list of guests.
	model.FetchGuests(r.Tx, func(g *model.Guest) {
		if g.Bidder != 0 {
			var copy = *g
			guests = append(guests, &copy)
		}
	}, "")
	sort.Slice(guests, func(i, j int) bool { return guests[i].Sortname < guests[j].Sortname })

	// Create a PDF document.
	pdf = gofpdf.New("P", "pt", "Letter", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	for i := 0; i < len(guests); i += 30 {
		pdf.AddPage()
		renderHashMarks(pdf)
		renderPageOfLabels(pdf, r, guests[i:])
	}
	if err := pdf.Error(); err != nil {
		log.Printf("PDF ERROR: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	pdf.Output(w)
}

func renderHashMarks(pdf *gofpdf.Fpdf) {
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(11.25, 0, 11.25, 36)
	pdf.Line(200.25, 0, 200.25, 36)
	pdf.Line(211.5, 0, 211.5, 36)
	pdf.Line(400.5, 0, 400.5, 36)
	pdf.Line(411.75, 0, 411.75, 36)
	pdf.Line(600.75, 0, 600.75, 36)
	pdf.Line(0, 36, 11.25, 36)
	pdf.Line(0, 108, 11.25, 108)
	pdf.Line(0, 180, 11.25, 180)
	pdf.Line(0, 252, 11.25, 252)
	pdf.Line(0, 324, 11.25, 324)
	pdf.Line(0, 396, 11.25, 396)
	pdf.Line(0, 468, 11.25, 468)
	pdf.Line(0, 540, 11.25, 540)
	pdf.Line(0, 612, 11.25, 612)
	pdf.Line(0, 684, 11.25, 684)
	pdf.Line(0, 756, 11.25, 756)
}

func renderPageOfLabels(pdf *gofpdf.Fpdf, r *request.Request, guests []*model.Guest) {
	for col := 0; col < 3; col++ {
		for row := 0; row < 10; row++ {
			idx := col*10 + row
			if idx >= len(guests) {
				return
			}
			guest := guests[idx]
			party := model.FetchParty(r.Tx, guest.PartyID)
			table := model.FetchTable(r.Tx, party.TableID)
			renderLabel(pdf, guest, table, col, row)
		}
	}
}

func renderLabel(pdf *gofpdf.Fpdf, guest *model.Guest, table *model.Table, col, row int) {
	left := 200.25*float64(col) + 11.25
	top := 72*float64(row) + 46
	pdf.SetFont("helvetica", "B", 14)
	pdf.MoveTo(left, top)
	pdf.CellFormat(166.5, 28, guest.Name, "", 0, "TL", false, 0, "")
	pdf.SetFont("helvetica", "", 12)
	pdf.MoveTo(left, top+28)
	pdf.Cellf(166.5, 12, "Table %d", table.Number)
	pdf.MoveTo(left, top+40)
	pdf.Cellf(166.5, 12, "Bidder %X", guest.Bidder)
}

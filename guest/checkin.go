package guest

import (
	"log"
	"net/http"
	"sort"

	"github.com/jung-kurt/gofpdf"

	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

func serveCheckinForms(w *request.ResponseWriter, r *request.Request) {
	var (
		guests []*model.Guest
		left   []*model.Guest
		right  []*model.Guest
		pdf    *gofpdf.Fpdf
		logo   *gofpdf.ImageInfoType
		banner *gofpdf.ImageInfoType
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
	w.Header().Set("Content-Disposition", `attachment; filename="checkin-forms.pdf"`)

	// First, get the sorted list of guests, and split it in half.  We'll
	// put the first half on the left sides of 8.5x11" pages and the second
	// half on the right sides of those pages.  That way, the resulting
	// stack can be sliced with a paper cutter, the right half can be put
	// under the left half, and we wind up with a sorted stack of 5.5x8.5"
	// pages.
	model.FetchGuests(r.Tx, func(g *model.Guest) {
		var copy = *g
		guests = append(guests, &copy)
	}, "")
	sort.Slice(guests, func(i, j int) bool { return guests[i].Sortname < guests[j].Sortname })
	left = guests[:(len(guests)+1)/2]
	right = guests[len(left):]
	if len(right) < len(left) {
		right = append(right, nil)
	}

	// Create a PDF document.
	pdf = gofpdf.New("L", "pt", "Letter", "")
	pdf.SetMargins(0, 0, 0)
	for i := range left {
		pdf.AddPage()
		renderGuest(pdf, logo, banner, left[i], 0)
		if right[i] != nil {
			renderGuest(pdf, logo, banner, right[i], 396) // 5.5 inches
		}
	}
	if err := pdf.Error(); err != nil {
		log.Printf("PDF ERROR: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	pdf.Output(w)
}

func renderGuest(pdf *gofpdf.Fpdf, logo, banner *gofpdf.ImageInfoType, guest *model.Guest, offset float64) {
	pdf.ImageOptions("logo1.png", 36+offset, 36, 144, 0, false, gofpdf.ImageOptions{}, 0, "")
	pdf.ImageOptions("logo2.png", 216+offset, 36, 144, 0, false, gofpdf.ImageOptions{}, 0, "")
	pdf.SetFont("helvetica", "B", 24)
	pdf.MoveTo(36+offset, 121)
	pdf.CellFormat(324, 30, "Guest Registration", "", 0, "TC", false, 0, "")
	pdf.SetFont("helvetica", "", 16)
	pdf.MoveTo(36+offset, 151)
	pdf.CellFormat(324, 20, "Please provide or correct this information.", "", 1, "TC", false, 0, "")
	pdf.MoveTo(36+offset, 193)
	pdf.Cell(324, 20, "Name")
	pdf.MoveTo(36+offset, 245)
	pdf.Cell(324, 20, "Email")
	pdf.MoveTo(36+offset, 297)
	pdf.Cell(324, 20, "Address")
	pdf.MoveTo(36+offset, 401)
	pdf.Cell(324, 20, "Phone")
	pdf.MoveTo(144+offset, 193)
	pdf.Line(108+offset, 209, 360+offset, 209)
	pdf.Line(108+offset, 261, 360+offset, 261)
	pdf.Line(108+offset, 313, 360+offset, 313)
	pdf.Line(108+offset, 365, 360+offset, 365)
	pdf.Line(108+offset, 417, 360+offset, 417)
	pdf.SetFontSize(14)
	pdf.MoveTo(117+offset, 193)
	pdf.Cell(234, 16, guest.Name)
	pdf.MoveTo(117+offset, 245)
	pdf.Cell(234, 16, guest.Email)
	pdf.MoveTo(117+offset, 297)
	pdf.Cell(234, 16, guest.Address)
	pdf.MoveTo(117+offset, 349)
	if guest.City != "" || guest.State != "" || guest.Zip != "" {
		pdf.Cellf(234, 16, "%s, %s  %s", guest.City, guest.State, guest.Zip)
	}
	pdf.MoveTo(117+offset, 401)
	pdf.Cell(234, 16, guest.Phone)
	pdf.SetFontSize(16)
	pdf.MoveTo(36+offset, 453)
	pdf.Cell(324, 20, "Would you like to be on our mailing lists?")
	pdf.Line(108+offset, 473, 124+offset, 473)
	pdf.Line(124+offset, 473, 124+offset, 489)
	pdf.Line(124+offset, 489, 108+offset, 489)
	pdf.Line(108+offset, 489, 108+offset, 473)
	pdf.MoveTo(132+offset, 473)
	pdf.Cell(324, 20, "Email")
	pdf.Line(216+offset, 473, 232+offset, 473)
	pdf.Line(232+offset, 473, 232+offset, 489)
	pdf.Line(232+offset, 489, 216+offset, 489)
	pdf.Line(216+offset, 489, 216+offset, 473)
	pdf.MoveTo(240+offset, 473)
	pdf.Cell(324, 20, "Postal")
	pdf.MoveTo(36+offset, 513)
	pdf.CellFormat(324, 20, "Thank you for supporting Schola Cantorum!", "", 0, "TC", false, 0, "")
}

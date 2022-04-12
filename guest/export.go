package guest

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

func serveGuestList(w *request.ResponseWriter, r *request.Request) {
	var (
		cw *csv.Writer
	)
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="gala-guests.csv"`)
	cw = csv.NewWriter(w)
	cw.UseCRLF = true
	cw.Write([]string{"Bidder", "Guest", "Email", "Address", "City", "State", "Zip", "Phone", "Entree", "Requests"})
	model.FetchGuests(r.Tx, func(g *model.Guest) {
		fields := []string{"", g.Sortname, g.Email, g.Address, g.City, g.State, g.Zip, g.Phone, g.Entree,
			strings.ReplaceAll(g.Requests, "\n", " ")}
		if g.Bidder != 0 {
			fields[0] = strconv.FormatInt(int64(g.Bidder), 16)
		}
		cw.Write(fields)
	}, "1 ORDER BY sortname")
	cw.Flush()
}

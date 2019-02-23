package journal

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/request"
)

const (
	pongWait   = 60 * time.Second
	pingPeriod = pongWait * 9 / 10
	writeWait  = 10 * time.Second
)

type client struct {
	conn *websocket.Conn
	send chan message
}
type message struct {
	Seq  int             `json:"seq"`
	Data json.RawMessage `json:"data"`
}

var (
	clients    = map[*client]struct{}{}
	broadcast  = make(chan message)
	register   = make(chan *client)
	unregister = make(chan *client)
	upgrader   = websocket.Upgrader{
		CheckOrigin: config.CheckWebSocketOrigin,
		Error: func(w http.ResponseWriter, _ *http.Request, status int, _ error) {
			w.WriteHeader(status)
		},
	}
)

// Sender is a goroutine that routes updates to all connected clients.
func Sender() {
	for {
		select {
		case client := <-register:
			clients[client] = struct{}{}
		case client := <-unregister:
			if _, ok := clients[client]; ok {
				delete(clients, client)
				close(client.send)
			}
		case message := <-broadcast:
			for client := range clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(clients, client)
				}
			}
		}
	}
}

// ServeWS handles requests for /ws, the websocket for journal updates.
func ServeWS(w *request.ResponseWriter, r *request.Request) {
	var (
		cl  client
		err error
	)

	if cl.conn, err = upgrader.Upgrade(w.ResponseWriter, r.Request, nil); err != nil {
		log.Printf("websocket upgrader: %s", err)
		return // upgrader sent an error
	}
	cl.send = make(chan message, 256)
	register <- &cl
	go cl.writer()
	go cl.reader()
}

func (c *client) writer() {
	var (
		ticker *time.Ticker
		w      io.WriteCloser
		err    error
	)
	ticker = time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if w, err = c.conn.NextWriter(websocket.TextMessage); err != nil {
				return
			}
			json.NewEncoder(w).Encode(message)
			if err = w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err = c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *client) reader() {
	defer func() {
		unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket: %v", err)
			}
			return
		}
	}
}

// Log adds an entry to the journal and sends it to all clients.
func Log(r *request.Request, je *model.JournalEntry) {
	var (
		by       []byte
		username sql.NullString
		res      sql.Result
		cid      int64
		err      error
	)
	je.Populate(r.Tx)
	if by, err = json.Marshal(je); err != nil {
		panic(err)
	}
	if r.Username != "" {
		username = sql.NullString{Valid: true, String: r.Username}
	}
	if res, err = r.Tx.Exec(`INSERT INTO journal (user, timestamp, change) VALUES (?,?,?)`,
		username, time.Now().Format(time.RFC3339), by); err != nil {
		panic(err)
	}
	cid, _ = res.LastInsertId()
	broadcast <- message{int(cid), by}
}

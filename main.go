// gala is the web server for the Schola Cantorum gala manager site.  It expects
// its working directory to be the root directory of the site.
//
// On startup, the server attempts to take a lock on run.lock.  If it fails, it
// will exit immediately and silently.  This allows the server to be started
// every 5 minutes by a cron job, as a poor man's HA strategy.
//
// The server listens on two ports: 9000 for HTTP/WS traffic and 9001 for
// HTTPS/WSS traffic.  In production use, clients send everything using TLS.
// But while the websocket requests come to port 9001, encrypted, the HTTP
// requests come to Dreamhost's server on port 443 and are proxied to our port
// 9000, losing their encryption along the way.  In practice, this code makes
// no distinction between the ports other than ensuring that some secure path
// was used.

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/scholacantorum/gala-backend/authn"
	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/db"
	"github.com/scholacantorum/gala-backend/guest"
	"github.com/scholacantorum/gala-backend/item"
	"github.com/scholacantorum/gala-backend/journal"
	"github.com/scholacantorum/gala-backend/party"
	"github.com/scholacantorum/gala-backend/payments"
	"github.com/scholacantorum/gala-backend/purchase"
	"github.com/scholacantorum/gala-backend/request"
	"github.com/scholacantorum/gala-backend/table"
)

var dbh *sqlx.DB
var requestMutex sync.Mutex

func main() {
	var (
		logFH     *os.File
		lockFH    *os.File
		listener  net.Listener
		listener2 net.Listener
		wg        sync.WaitGroup
		err       error
		server    = http.Server{Addr: config.Get("httpsListen"), Handler: http.HandlerFunc(handler)}
		server2   = http.Server{Addr: config.Get("wssListen"), Handler: http.HandlerFunc(handler)}
		sig       = make(chan os.Signal, 1)
	)
	if logFH, err = os.OpenFile("server.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
	log.SetOutput(logFH)
	if lockFH, err = os.Create("run.lock"); err != nil {
		log.Fatalf("ERROR: open run.lock: %s", err)
	}
	if err = syscall.Flock(int(lockFH.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if err == syscall.EWOULDBLOCK {
			os.Exit(0) // another copy is already running
		}
		log.Fatalf("ERROR: lock run.lock: %s", err)
	}
	if listener, err = net.Listen("tcp", server.Addr); err != nil {
		log.Fatalf("ERROR: listen on %s: %s", server.Addr, err)
	}
	if listener2, err = net.Listen("tcp", server2.Addr); err != nil {
		log.Fatalf("ERROR: listen on %s: %s", server2.Addr, err)
	}
	if dbh, err = db.Open("gala.db"); err != nil {
		log.Fatalf("ERROR: open gala.db: %s", err)
	}
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	wg.Add(1)
	go func() {
		if s := <-sig; s == syscall.SIGTERM {
			log.Fatalf("SERVER KILLED")
		} // otherwise it's SIGINT, graceful shutdown
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)
		log.Printf("SHUTTING DOWN")
		if err = server.Shutdown(context.Background()); err != nil {
			log.Printf("ERROR: shutdown: %s", err)
		}
		if err = server2.Shutdown(context.Background()); err != nil {
			log.Printf("ERROR: shutdown: %s", err)
		}
		wg.Done()
	}()
	go journal.Sender()
	log.Printf("SERVER START")
	go func() {
		err := server2.ServeTLS(tcpKeepAliveListener{listener2.(*net.TCPListener)}, "cert.pem", "key.pem")
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("ERROR: server failed: %s", err)
		}
	}()
	if err = server.Serve(tcpKeepAliveListener{listener.(*net.TCPListener)}); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ERROR: server failed: %s", err)
	}
	wg.Wait()
	log.Printf("SERVER SHUTDOWN")
}

// ServeHTTP wraps the request handling with a log entry that records the
// IP address, username, method, URI, status code, response length, and elapsed
// time of the request.
func handler(w http.ResponseWriter, r *http.Request) {
	origin := config.Get("webSocketOrigin")
	if r.URL.Path == "/register" {
		origin = config.Get("registerOrigin")
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "auth")
	w.Header().Set("Access-Control-Expose-Headers", "auth")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !requireSecure(w, r) {
		return
	}
	ourw := request.NewResponseWriter(w, r)
	ourr := request.NewRequest(r)
	panicCatcher(ourw, ourr)
	ourw.Close()
}

// requireSecure ensures the is on https (if it isn't in development use).
func requireSecure(w http.ResponseWriter, r *http.Request) bool {
	// If it came in through port 9001, it's already secure, we're good.
	if r.TLS != nil {
		return true
	}

	// It may have come in through port 9000, but via the Apache proxy, and
	// the connection to the proxy may have been secure.  That's good enough
	// too.
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}

	// We may be running this on localhost for development, in which case
	// non-SSL is OK.  That's the case if the requesting IP is localhost
	// and it's not coming through the proxy.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(ip).IsLoopback() && (r.Header.Get("X-Forwarded-For") == "" || r.Header.Get("X-Forwarded-For") == "127.0.0.1") {
		return true
	}

	// In all other cases, it's an error.
	log.Printf("reject insecure connection")
	w.WriteHeader(http.StatusForbidden)
	return false
}

// panicCatcher wraps the request in a panic catcher.
func panicCatcher(w *request.ResponseWriter, r *request.Request) {
	defer func() {
		if panicked := recover(); panicked != nil {
			log.Printf("ERROR: %v", panicked)
			log.Print(string(debug.Stack()))
			w.WriteHeader(http.StatusInternalServerError)
			if r.Tx != nil {
				r.Tx.Rollback()
			}
		}
	}()
	transactionWrapper(w, r)
}

// transactionWrapper wraps the request in a database transaction.  It also
// uses a lock to ensure we're handling only one request at a time.  Neither
// restriction applies to websocket connections.
func transactionWrapper(w *request.ResponseWriter, r *request.Request) {
	var err error

	requestMutex.Lock()
	defer requestMutex.Unlock()
	if r.Tx, err = dbh.Beginx(); err != nil {
		panic(err)
	}
	authChecker(w, r)
	r.Tx.Rollback()
}

// authChecker checks the authentication of the caller.
func authChecker(w *request.ResponseWriter, r *request.Request) {
	r.URL.Path = path.Clean(r.URL.Path)
	if r.URL.Path != "/login" && r.URL.Path != "/register" {
		if !authn.ValidSession(r) {
			log.Printf("reject unauthorized")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	router(w, r)
}

// router starts the routing based on URI.
func router(w *request.ResponseWriter, r *request.Request) {
	var head string

	head, r.URL.Path = request.ShiftPath(r.URL.Path)
	if head == "backend" {
		head, r.URL.Path = request.ShiftPath(r.URL.Path)
	}
	switch head {
	case "all":
		journal.ServeAll(w, r)
	case "guest":
		guest.ServeGuest(w, r)
	case "guests":
		guest.ServeGuests(w, r)
	case "item":
		item.ServeItem(w, r)
	case "items":
		item.ServeItems(w, r)
	case "login":
		authn.ServeLogin(w, r)
	case "party":
		party.ServeParty(w, r)
	case "payments":
		payments.ServePayments(w, r)
	case "purchase":
		purchase.ServePurchase(w, r)
	case "purchases":
		purchase.ServePurchases(w, r)
	case "register":
		guest.ServeRegister(w, r)
	case "table":
		table.ServeTable(w, r)
	case "ws":
		r.Tx.Rollback()
		requestMutex.Unlock()
		journal.ServeWS(w, r)
		requestMutex.Lock()
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

type tcpKeepAliveListener struct{ *net.TCPListener }

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

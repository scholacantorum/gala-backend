package request

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// ResponseWriter is a wrapper around http.ResponseWriter that addresses this
// web site's specific needs.
type ResponseWriter struct {
	http.ResponseWriter
	callerGzipOK bool
	writer       io.Writer
}

// Pools for common/expensive allocations.
var gzipPool sync.Pool
var writerPool = sync.Pool{New: func() interface{} { return new(ResponseWriter) }}

// NewResponseWriter creates a new ResponseWriter for the response.
func NewResponseWriter(w http.ResponseWriter, r *http.Request) *ResponseWriter {
	nw := writerPool.Get().(*ResponseWriter)
	nw.ResponseWriter = w
	nw.callerGzipOK = strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && r.URL.Path != "/ws"
	nw.writer = nil
	return nw
}

// Write wraps the http.ResponseWriter.Write to perform gzip encoding.
func (w *ResponseWriter) Write(buf []byte) (int, error) {
	if w.writer == nil {
		w.startWriting()
	}
	return w.writer.Write(buf)
}

// WriteHeader wraps the http.RseponseWriter.WriteHeader to track status code
// and content length and perform gzip encoding.
func (w *ResponseWriter) WriteHeader(statusCode int) {
	w.startWriting()
	w.ResponseWriter.WriteHeader(statusCode)
}

// startWriting starts the gzip encoding if desired.
func (w *ResponseWriter) startWriting() {
	if !w.callerGzipOK {
		w.writer = w.ResponseWriter
	} else {
		w.Header().Set("Content-Encoding", "gzip")
		g := gzipPool.Get()
		if g == nil {
			w.writer = gzip.NewWriter(w.ResponseWriter)
		} else {
			g.(*gzip.Writer).Reset(w.ResponseWriter)
			w.writer = g.(*gzip.Writer)
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
}

// Close finalizes the response writer.
func (w *ResponseWriter) Close() {
	if w.writer != nil && w.writer != w.ResponseWriter {
		w.writer.(*gzip.Writer).Close()
		gzipPool.Put(w.writer)
	}
	w.writer = nil
	writerPool.Put(w)
}

// CommitNoContent is a shortcut that commits the request transaction and, if
// successful, sends a 204 No Content response.
func (w *ResponseWriter) CommitNoContent(r *Request) {
	if err := r.Tx.Commit(); err != nil {
		panic(err)
	}
	w.WriteHeader(http.StatusNoContent)
}

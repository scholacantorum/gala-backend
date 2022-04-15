package sendmail

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"

	"github.com/scholacantorum/gala-backend/config"
)

//go:embed "mail-logo.png"

// ScholaLogoPNG is the Schola Cantorum logo header for the top of an email.
var ScholaLogoPNG []byte

// Message is an outgoing message.
type Message struct {
	// SendTo is the list of addresses to which to send the message.  It
	// does not have to include all of the To, Cc, or Bcc addresses, and it
	// can contain others.  Note that these strings should be bare
	// addresses.
	SendTo []string
	// From is the content of the From: header.
	From string
	// To is the content of the To: header.  Multiple strings will be joined
	// with ", ".
	To []string
	// Cc is the content of the Cc: header.  Multiple strings will be joined
	// with ", ".
	Cc []string
	// Bcc is the content of the Bcc: header.  Multiple strings, will be
	// joined with ", ".
	Bcc []string
	// Subject is the content of the Subject: header.
	Subject string
	// ReplyTo is the content of the Reply-To: header.
	ReplyTo string
	// Text is the plain text form of the message body.
	Text string
	// HTML is the HTML form of the message body.
	HTML string
	// Images is an optional list of images to embed in the message body.
	// They can be referenced in the HTML with <img src="cid:IMG#">, where
	// # is the zero-based index of the image in the Images slice.  They
	// must be in PNG format.
	Images [][]byte
	// To be complete, we should allow attachments as well.  But we don't
	// need them for anything right now, so that's a someday thing.
}

// Send sends an email message.
func (m *Message) Send() (err error) {
	by := m.render()
	err = smtp.SendMail(
		config.Get("smtpServer"),
		smtp.PlainAuth("",
			config.Get("smtpUsername"),
			config.Get("smtpPassword"),
			config.Get("smtpHost"),
		),
		config.Get("smtpFromAddr"),
		m.SendTo,
		by,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR sending email: %s\n", err)
	}
	return err
}

func (m *Message) render() []byte {
	var (
		buf  bytes.Buffer
		mw   *multipart.Writer
		mw2  *multipart.Writer
		hdr  textproto.MIMEHeader
		part io.Writer
		qw   *quotedprintable.Writer
		bw   io.WriteCloser
	)
	if m.From != "" {
		fmt.Fprintf(&buf, "From: %s\r\n", m.From)
	}
	if m.ReplyTo != "" {
		fmt.Fprintf(&buf, "Reply-To: %s\r\n", m.ReplyTo)
	}
	if len(m.To) != 0 {
		fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(m.To, ", "))
	}
	if len(m.Cc) != 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(m.Cc, ", "))
	}
	if len(m.Bcc) != 0 {
		fmt.Fprintf(&buf, "Bcc: %s\r\n", strings.Join(m.Bcc, ", "))
	}
	if m.Subject != "" {
		fmt.Fprintf(&buf, "Subject: %s\r\n", m.Subject)
	}
	switch {
	case m.Text != "" && m.HTML == "":
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	case m.Text != "" && m.HTML != "":
		mw = multipart.NewWriter(&buf)
		fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", mw.Boundary())
	case m.Text == "" && m.HTML != "" && len(m.Images) == 0:
		fmt.Fprintf(&buf, "Content-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	case m.Text == "" && m.HTML != "" && len(m.Images) != 0:
		mw = multipart.NewWriter(&buf)
		fmt.Fprintf(&buf, "Content-Type: multipart/related; boundary=%s\r\n\r\n", mw.Boundary())
	}
	if m.Text != "" {
		if mw != nil {
			hdr = make(textproto.MIMEHeader)
			hdr.Set("Content-Type", "text/plain; charset=UTF-8")
			part, _ = mw.CreatePart(hdr)
			io.WriteString(part, m.Text)
		} else {
			io.WriteString(&buf, m.Text)
		}
	}
	if m.HTML != "" && len(m.Images) == 0 {
		if mw != nil {
			hdr := make(textproto.MIMEHeader)
			hdr.Set("Content-Type", "text/html; charset=UTF-8")
			hdr.Set("Content-Transfer-Encoding", "quoted-printable")
			part, _ := mw.CreatePart(hdr)
			qw = quotedprintable.NewWriter(part)
		} else {
			qw = quotedprintable.NewWriter(&buf)
		}
		io.WriteString(qw, m.HTML)
		qw.Close()
	}
	if m.HTML != "" && len(m.Images) != 0 {
		if m.Text != "" {
			hdr = make(textproto.MIMEHeader)
			hdr.Set("Content-Type", "multipart/related; boundary=X"+mw.Boundary())
			part, _ = mw.CreatePart(hdr)
			mw2 = multipart.NewWriter(part)
			mw2.SetBoundary("X" + mw.Boundary())
		} else {
			mw2 = mw
		}
		hdr = make(textproto.MIMEHeader)
		hdr.Set("Content-Type", "text/html; charset=UTF-8")
		hdr.Set("Content-Transfer-Encoding", "quoted-printable")
		part, _ = mw2.CreatePart(hdr)
		qw = quotedprintable.NewWriter(part)
		io.WriteString(qw, m.HTML)
		qw.Close()
		for i, image := range m.Images {
			hdr = make(textproto.MIMEHeader)
			hdr.Set("Content-Type", "image/png")
			hdr.Set("Content-ID", fmt.Sprintf("<IMG%d>", i))
			hdr.Set("Content-Transfer-Encoding", "base64")
			part, _ = mw2.CreatePart(hdr)
			bw = base64.NewEncoder(base64.StdEncoding, part)
			bw.Write(image)
			bw.Close()
		}
		if mw2 != mw {
			mw2.Close()
		}
	}
	if mw != nil {
		mw.Close()
	}
	return buf.Bytes()
}

// Package mailer delivers transactional messages through a configured SMTP relay.
package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type SMTP struct {
	addr, host, from, user, password string
	allowPlain                       bool
}

func FromEnv(get func(string) string) (*SMTP, error) {
	addr := get("SMTP_ADDR")
	if addr == "" {
		return nil, nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.New("SMTP_ADDR requires host:port")
	}
	from := get("SMTP_FROM")
	parsed, err := mail.ParseAddress(from)
	if err != nil || parsed.Address != from || strings.ContainsAny(from, "\r\n") {
		return nil, errors.New("invalid SMTP_FROM")
	}
	plain := false
	if v := get("SMTP_ALLOW_PLAINTEXT"); v != "" {
		plain, err = strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("invalid SMTP_ALLOW_PLAINTEXT")
		}
	}
	return &SMTP{addr, host, from, get("SMTP_USERNAME"), get("SMTP_PASSWORD"), plain}, nil
}
func (s *SMTP) Send(ctx context.Context, to, subject, body string) error {
	if s == nil {
		return errors.New("SMTP is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c, err := new(net.Dialer).DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	deadline, _ := ctx.Deadline()
	if err = c.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	relay, err := smtp.NewClient(c, s.host)
	if err != nil {
		return err
	}
	defer func() { _ = relay.Close() }()
	if ok, _ := relay.Extension("STARTTLS"); ok {
		if err = relay.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	} else if !s.allowPlain || s.user != "" {
		return errors.New("SMTP relay requires STARTTLS")
	}
	if s.user != "" {
		if err = relay.Auth(smtp.PlainAuth("", s.user, s.password, s.host)); err != nil {
			return err
		}
	}
	if err = relay.Mail(s.from); err != nil {
		return err
	}
	if err = relay.Rcpt(to); err != nil {
		return err
	}
	w, err := relay.Data()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", s.from, to, mime.QEncoding.Encode("utf-8", subject), time.Now().Format(time.RFC1123Z), body)
	if err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return relay.Quit()
}

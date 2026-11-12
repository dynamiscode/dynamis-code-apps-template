package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

type Sender interface {
	Send(context.Context, string, string, string) error
}

type SMTP struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
}

func NewSMTP(cfg config.Mail) (*SMTP, error) {
	return newSMTP(cfg, "")
}

// NewWorkspaceSMTP builds a sender whose From header carries a display name.
func NewWorkspaceSMTP(cfg config.Mail, senderName string) (*SMTP, error) {
	return newSMTP(cfg, senderName)
}

func newSMTP(cfg config.Mail, senderName string) (*SMTP, error) {
	if cfg.Host == "" {
		return nil, nil
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, errors.New("SMTP_FROM must be a valid address")
	}
	return &SMTP{
		host: cfg.Host, port: cfg.Port, username: cfg.Username,
		password: cfg.Password, from: cfg.From, fromName: senderName,
	}, nil
}

func (s *SMTP) Send(ctx context.Context, recipient, subject, body string) error {
	if s == nil {
		return errors.New("SMTP is not configured")
	}
	if strings.ContainsAny(recipient+subject, "\r\n") {
		return errors.New("mail header contains invalid characters")
	}
	client, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Mail(s.from); err != nil {
		return errors.New("SMTP sender rejected")
	}
	if err := client.Rcpt(recipient); err != nil {
		return errors.New("SMTP recipient rejected")
	}
	writer, err := client.Data()
	if err != nil {
		return errors.New("SMTP message rejected")
	}
	message := formatMessage(s.fromHeader(), subject, body)
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return errors.New("SMTP message write failed")
	}
	if err := writer.Close(); err != nil {
		return errors.New("SMTP message delivery failed")
	}
	return client.Quit()
}

// TestLogin proves the server accepts STARTTLS and the configured login. It sends nothing.
func (s *SMTP) TestLogin(ctx context.Context) error {
	if s == nil {
		return errors.New("SMTP is not configured")
	}
	client, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	_ = client.Quit()
	return nil
}

// session dials, upgrades to TLS, and authenticates. The caller closes the client.
func (s *SMTP) session(ctx context.Context) (*smtp.Client, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	address := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := net.Dialer{Timeout: time.Until(deadline)}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, errors.New("SMTP connection failed")
	}
	if err := connection.SetDeadline(deadline); err != nil {
		connection.Close()
		return nil, errors.New("SMTP deadline failed")
	}
	client, err := smtp.NewClient(connection, s.host)
	if err != nil {
		connection.Close()
		return nil, errors.New("SMTP handshake failed")
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		client.Close()
		return nil, errors.New("SMTP server does not support STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
		client.Close()
		return nil, errors.New("SMTP STARTTLS failed")
	}
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			client.Close()
			return nil, errors.New("SMTP authentication failed")
		}
	}
	return client, nil
}

// fromHeader is the From header; the envelope sender stays the bare address.
func (s *SMTP) fromHeader() string {
	if s.fromName == "" {
		return s.from
	}
	return (&mail.Address{Name: s.fromName, Address: s.from}).String()
}

func formatMessage(from, subject, body string) string {
	return "From: " + from + "\r\nTo: undisclosed-recipients:;\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body + "\r\n"
}

package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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
}

func NewSMTP(cfg config.Mail) (*SMTP, error) {
	if cfg.Host == "" {
		return nil, nil
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, errors.New("SMTP_FROM must be a valid address")
	}
	return &SMTP{
		host: cfg.Host, port: cfg.Port, username: cfg.Username,
		password: cfg.Password, from: cfg.From,
	}, nil
}

func (s *SMTP) Send(ctx context.Context, recipient, subject, body string) error {
	if s == nil {
		return errors.New("SMTP is not configured")
	}
	if strings.ContainsAny(recipient+subject, "\r\n") {
		return errors.New("mail header contains invalid characters")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	address := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := net.Dialer{Timeout: time.Until(deadline)}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return errors.New("SMTP connection failed")
	}
	defer connection.Close()
	if err := connection.SetDeadline(deadline); err != nil {
		return errors.New("SMTP deadline failed")
	}
	client, err := smtp.NewClient(connection, s.host)
	if err != nil {
		return errors.New("SMTP handshake failed")
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not support STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
		return errors.New("SMTP STARTTLS failed")
	}
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return errors.New("SMTP authentication failed")
		}
	}
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
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", s.from, recipient, subject, body)
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return errors.New("SMTP message write failed")
	}
	if err := writer.Close(); err != nil {
		return errors.New("SMTP message delivery failed")
	}
	return client.Quit()
}

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

const cloudflareEndpoint = "https://api.cloudflare.com/client/v4"

// Cloudflare sends mail through Cloudflare Email Sending's REST API.
type Cloudflare struct {
	accountID string
	token     string
	from      string
	endpoint  string
	client    *http.Client
}

func NewCloudflare(cfg config.CloudflareEmail) (*Cloudflare, error) {
	if cfg.AccountID == "" {
		return nil, nil
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, errors.New("CLOUDFLARE_EMAIL_FROM must be a valid address")
	}
	return &Cloudflare{
		accountID: cfg.AccountID, token: cfg.APIToken, from: cfg.From,
		endpoint: cloudflareEndpoint, client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// NewSender returns the configured provider. Config loading rejects both at once.
func NewSender(smtpCfg config.Mail, cloudflareCfg config.CloudflareEmail) (Sender, error) {
	smtpSender, err := NewSMTP(smtpCfg)
	if err != nil {
		return nil, err
	}
	if smtpSender != nil {
		return smtpSender, nil
	}
	cloudflareSender, err := NewCloudflare(cloudflareCfg)
	if err != nil {
		return nil, err
	}
	if cloudflareSender != nil {
		return cloudflareSender, nil
	}
	return nil, nil
}

func (c *Cloudflare) Send(ctx context.Context, recipient, subject, body string) error {
	if c == nil {
		return errors.New("Cloudflare Email Sending is not configured")
	}
	if strings.ContainsAny(recipient+subject, "\r\n") {
		return errors.New("mail header contains invalid characters")
	}
	payload, err := json.Marshal(map[string]string{
		"to": recipient, "from": c.from, "subject": subject, "text": body,
	})
	if err != nil {
		return errors.New("Cloudflare message encoding failed")
	}
	target := c.endpoint + "/accounts/" + url.PathEscape(c.accountID) + "/email/sending/send"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return errors.New("Cloudflare request failed")
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return errors.New("Cloudflare Email Sending request failed")
	}
	defer response.Body.Close()
	// The body is bounded and never echoed into errors: it may reflect request data.
	var envelope struct {
		Success bool `json:"success"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope)
	if response.StatusCode != http.StatusOK || decodeErr != nil || !envelope.Success {
		return errors.New("Cloudflare Email Sending rejected the message")
	}
	return nil
}

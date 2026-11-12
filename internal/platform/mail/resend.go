package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// ResendBaseURL is the production Resend API origin.
const ResendBaseURL = "https://api.resend.com"

// Resend sends mail through Resend's HTTPS API.
type Resend struct {
	baseURL string
	apiKey  string
	from    string
	name    string
}

// resendClient bounds every Resend call. Shared, so both paths reuse one connection pool.
var resendClient = &http.Client{Timeout: 10 * time.Second}

// ResendDomain is one sending domain on the Resend account.
type ResendDomain struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// NewResend builds a sender. senderName becomes the display name in From.
func NewResend(baseURL, apiKey, from, senderName string) (*Resend, error) {
	if _, err := mail.ParseAddress(from); err != nil {
		return nil, errors.New("sender address must be a valid address")
	}
	if apiKey == "" {
		return nil, errors.New("Resend API key is required")
	}
	return &Resend{
		baseURL: baseURL, apiKey: apiKey, from: from, name: senderName,
	}, nil
}

func (r *Resend) fromHeader() string {
	if r.name == "" {
		return r.from
	}
	return (&mail.Address{Name: r.name, Address: r.from}).String()
}

// Send posts one message. Response bodies are never echoed into errors.
func (r *Resend) Send(ctx context.Context, recipient, subject, body string) error {
	if strings.ContainsAny(recipient+subject, "\r\n") {
		return errors.New("mail header contains invalid characters")
	}
	payload, err := json.Marshal(map[string]any{
		"from": r.fromHeader(), "to": []string{recipient}, "subject": subject, "text": body,
	})
	if err != nil {
		return errors.New("Resend message encoding failed")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return errors.New("Resend request failed")
	}
	request.Header.Set("Authorization", "Bearer "+r.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := resendClient.Do(request)
	if err != nil {
		return errors.New("Resend request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return errors.New("Resend rejected the message")
	}
	return nil
}

// ResendDomains lists the account's sending domains. It sends nothing and is the read-only key check.
func ResendDomains(ctx context.Context, baseURL, apiKey string) ([]ResendDomain, error) {
	// ponytail: first 100 domains only, no pagination; a sender domain past that reads as not_found. Follow has_more when accounts grow past it.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/domains?limit=100", nil)
	if err != nil {
		return nil, errors.New("Resend request failed")
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := resendClient.Do(request)
	if err != nil {
		return nil, errors.New("Resend request failed")
	}
	defer response.Body.Close()
	// Only auth failures mean the key is bad; rate limits and outages are reported as such.
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, errors.New("Resend rejected the API key")
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("Resend check failed")
	}
	var page struct {
		Data []ResendDomain `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&page); err != nil {
		return nil, errors.New("Resend response was unreadable")
	}
	return page.Data, nil
}

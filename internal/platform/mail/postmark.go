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

// PostmarkBaseURL is the production Postmark API origin.
const PostmarkBaseURL = "https://api.postmarkapp.com"

// Postmark sends mail through Postmark's HTTPS API with a server token.
type Postmark struct {
	baseURL string
	token   string
	from    string
	name    string
}

// postmarkClient bounds every Postmark call.
var postmarkClient = &http.Client{Timeout: 10 * time.Second}

// PostmarkDomain is one sending domain on the Postmark account.
type PostmarkDomain struct {
	Name                     string `json:"Name"`
	DKIMVerified             bool   `json:"DKIMVerified"`
	ReturnPathDomainVerified bool   `json:"ReturnPathDomainVerified"`
}

// NewPostmark builds a sender. senderName becomes the display name in From.
func NewPostmark(baseURL, serverToken, from, senderName string) (*Postmark, error) {
	if _, err := mail.ParseAddress(from); err != nil {
		return nil, errors.New("sender address must be a valid address")
	}
	if serverToken == "" {
		return nil, errors.New("Postmark server token is required")
	}
	return &Postmark{baseURL: baseURL, token: serverToken, from: from, name: senderName}, nil
}

func (p *Postmark) fromHeader() string {
	if p.name == "" {
		return p.from
	}
	return (&mail.Address{Name: p.name, Address: p.from}).String()
}

// Send posts one message. Response bodies are never echoed into errors.
func (p *Postmark) Send(ctx context.Context, recipient, subject, body string) error {
	if strings.ContainsAny(recipient+subject, "\r\n") {
		return errors.New("mail header contains invalid characters")
	}
	payload, err := json.Marshal(map[string]any{
		"From": p.fromHeader(), "To": recipient, "Subject": subject, "TextBody": body,
	})
	if err != nil {
		return errors.New("Postmark message encoding failed")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/email", bytes.NewReader(payload))
	if err != nil {
		return errors.New("Postmark request failed")
	}
	request.Header.Set("X-Postmark-Server-Token", p.token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := postmarkClient.Do(request)
	if err != nil {
		return errors.New("Postmark request failed")
	}
	defer response.Body.Close()
	var result struct {
		ErrorCode int `json:"ErrorCode"`
	}
	// Postmark can report a rejected message with a 2xx status, so the envelope is checked too.
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result)
	if response.StatusCode < 200 || response.StatusCode > 299 || decodeErr != nil || result.ErrorCode != 0 {
		return errors.New("Postmark rejected the message")
	}
	return nil
}

// PostmarkServerCheck reads the server the token belongs to. It sends nothing.
func PostmarkServerCheck(ctx context.Context, baseURL, serverToken string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/server", nil)
	if err != nil {
		return errors.New("Postmark request failed")
	}
	request.Header.Set("X-Postmark-Server-Token", serverToken)
	request.Header.Set("Accept", "application/json")
	response, err := postmarkClient.Do(request)
	if err != nil {
		return errors.New("Postmark request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return errors.New("Postmark rejected the server token")
	}
	if response.StatusCode != http.StatusOK {
		return errors.New("Postmark check failed")
	}
	return nil
}

// PostmarkDomains lists the account's sending domains. It sends nothing.
func PostmarkDomains(ctx context.Context, baseURL, accountToken string) ([]PostmarkDomain, error) {
	// ponytail: first 500 domains only, no paging past offset 0; a sender domain beyond that reads as not_found. Follow TotalCount when accounts grow past it.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/domains?count=500&offset=0", nil)
	if err != nil {
		return nil, errors.New("Postmark request failed")
	}
	request.Header.Set("X-Postmark-Account-Token", accountToken)
	request.Header.Set("Accept", "application/json")
	response, err := postmarkClient.Do(request)
	if err != nil {
		return nil, errors.New("Postmark request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, errors.New("Postmark rejected the account token")
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("Postmark check failed")
	}
	var page struct {
		Domains []PostmarkDomain `json:"Domains"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&page); err != nil {
		return nil, errors.New("Postmark response was unreadable")
	}
	return page.Domains, nil
}

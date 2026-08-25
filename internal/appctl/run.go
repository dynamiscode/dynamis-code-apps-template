package appctl

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

type options struct {
	baseURL string
	token   string
	timeout time.Duration
}

type optionalString struct {
	set   bool
	value string
}

func (value *optionalString) String() string { return value.value }
func (value *optionalString) Set(raw string) error {
	value.set, value.value = true, raw
	return nil
}

func Run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "items" {
		return usage(stderr, "usage: appctl items <list|get|create|update|delete> [flags]")
	}
	command := args[1]
	if command != "list" && command != "get" && command != "create" &&
		command != "update" && command != "delete" {
		return usage(stderr, "unknown items command")
	}
	flags := flag.NewFlagSet("appctl items "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaultTimeout := 30 * time.Second
	if raw := getenv("APPCTL_TIMEOUT"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return usage(stderr, "APPCTL_TIMEOUT is invalid")
		}
		defaultTimeout = value
	}
	common := options{
		baseURL: envOr(getenv, "APPCTL_BASE_URL", "http://127.0.0.1:8080"),
		token:   getenv("APPCTL_TOKEN"), timeout: defaultTimeout,
	}
	flags.StringVar(&common.baseURL, "base-url", common.baseURL, "remote server URL")
	flags.StringVar(&common.token, "token", common.token, "API bearer token")
	flags.DurationVar(&common.timeout, "timeout", common.timeout, "request timeout")
	workspaceID := flags.String("workspace", "", "workspace ID")
	itemID := flags.String("item", "", "item ID")
	title := flags.String("title", "", "item title")
	status := flags.String("status", "", "item status")
	cursor := flags.String("cursor", "", "page cursor")
	sort := flags.String("sort", "-created_at", "created_at or -created_at")
	limit := flags.Int("limit", 50, "page size")
	version := flags.Int64("version", 0, "item version")
	idempotencyKey := flags.String("idempotency-key", "", "caller-generated idempotency key")
	var updateTitle, updateStatus optionalString
	flags.Var(&updateTitle, "set-title", "new title")
	flags.Var(&updateStatus, "set-status", "active or complete")
	if err := flags.Parse(args[2:]); err != nil || len(flags.Args()) != 0 {
		return usage(stderr, "flags are invalid")
	}
	allowedFlags := map[string]bool{
		"base-url": true, "token": true, "timeout": true, "workspace": true,
	}
	for _, name := range map[string][]string{
		"list": {"status", "cursor", "sort", "limit"},
		"get":  {"item"}, "create": {"title", "idempotency-key"},
		"update": {"item", "version", "set-title", "set-status"},
		"delete": {"item", "version"},
	}[command] {
		allowedFlags[name] = true
	}
	invalidFlag := false
	flags.Visit(func(value *flag.Flag) { invalidFlag = invalidFlag || !allowedFlags[value.Name] })
	if invalidFlag {
		return usage(stderr, "flag is not valid for this command")
	}
	base, err := validateOptions(common)
	if err != nil {
		return usage(stderr, err.Error())
	}
	if !validID(*workspaceID) {
		return usage(stderr, "--workspace must be a 32-character lowercase hexadecimal ID")
	}
	path := "/api/v1/workspaces/" + *workspaceID + "/items"
	method, body := http.MethodGet, any(nil)
	headers := make(http.Header)
	switch command {
	case "list":
		if *limit < 1 || *limit > 100 || (*status != "" && *status != "active" && *status != "complete") ||
			(*sort != "created_at" && *sort != "-created_at") {
			return usage(stderr, "list flags are invalid")
		}
		query := make(url.Values)
		query.Set("limit", strconv.Itoa(*limit))
		query.Set("sort", *sort)
		if *status != "" {
			query.Set("status", *status)
		}
		if *cursor != "" {
			query.Set("cursor", *cursor)
		}
		path += "?" + query.Encode()
	case "get":
		if !validID(*itemID) {
			return usage(stderr, "--item must be a 32-character lowercase hexadecimal ID")
		}
		path += "/" + *itemID
	case "create":
		if strings.TrimSpace(*title) == "" || *idempotencyKey == "" {
			return usage(stderr, "create requires --title and --idempotency-key")
		}
		method, body = http.MethodPost, map[string]string{"title": *title}
		headers.Set("Idempotency-Key", *idempotencyKey)
	case "update":
		if !validID(*itemID) || *version < 1 || (!updateTitle.set && !updateStatus.set) {
			return usage(stderr, "update requires --item, --version, and --set-title or --set-status")
		}
		if updateStatus.set && updateStatus.value != "active" && updateStatus.value != "complete" {
			return usage(stderr, "--set-status must be active or complete")
		}
		values := make(map[string]string)
		if updateTitle.set {
			values["title"] = updateTitle.value
		}
		if updateStatus.set {
			values["status"] = updateStatus.value
		}
		method, body, path = http.MethodPatch, values, path+"/"+*itemID
		headers.Set("If-Match", fmt.Sprintf(`"v%d"`, *version))
	case "delete":
		if !validID(*itemID) || *version < 1 {
			return usage(stderr, "delete requires --item and --version")
		}
		method, path = http.MethodDelete, path+"/"+*itemID
		headers.Set("If-Match", fmt.Sprintf(`"v%d"`, *version))
	}
	return execute(common, base.ResolveReference(&url.URL{Path: pathWithoutQuery(path), RawQuery: rawQuery(path)}), method, body, headers, stdout, stderr)
}

func validateOptions(options options) (*url.URL, error) {
	if options.token == "" {
		return nil, errors.New("APPCTL_TOKEN or --token is required")
	}
	if options.timeout < time.Second || options.timeout > 5*time.Minute {
		return nil, errors.New("timeout must be from 1s to 5m")
	}
	base, err := url.Parse(options.baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" ||
		base.User != nil || (base.Path != "" && base.Path != "/") ||
		base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("base URL must be an HTTP origin without credentials, path, query, or fragment")
	}
	base.Path = "/"
	return base, nil
}

func execute(options options, endpoint *url.URL, method string, body any, headers http.Header, stdout, stderr io.Writer) int {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return failure(stderr, 1, "request_failed")
		}
		payload = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), payload)
	if err != nil {
		return failure(stderr, 1, "request_failed")
	}
	request.Header.Set("Authorization", "Bearer "+options.token)
	for key, values := range headers {
		request.Header[key] = values
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{
		Timeout:       options.timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return failure(stderr, 1, "request_failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return failure(stderr, 1, "response_invalid")
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if response.StatusCode == http.StatusNoContent {
			_, _ = io.WriteString(stdout, "{\"deleted\":true}\n")
		} else {
			if !json.Valid(data) {
				return failure(stderr, 1, "response_invalid")
			}
			_, _ = stdout.Write(data)
			if len(data) == 0 || data[len(data)-1] != '\n' {
				_, _ = io.WriteString(stdout, "\n")
			}
		}
		return 0
	}
	if !json.Valid(data) {
		return failure(stderr, exitForStatus(response.StatusCode), "remote_error")
	}
	_, _ = stderr.Write(data)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		_, _ = io.WriteString(stderr, "\n")
	}
	return exitForStatus(response.StatusCode)
}

func exitForStatus(status int) int {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return 3
	case status == http.StatusTooManyRequests:
		return 5
	case status >= 400 && status < 500:
		return 4
	default:
		return 1
	}
}

func usage(stderr io.Writer, detail string) int {
	encoded, _ := json.Marshal(map[string]string{"error": "usage", "detail": detail})
	_, _ = fmt.Fprintln(stderr, string(encoded))
	return 2
}

func failure(stderr io.Writer, status int, code string) int {
	encoded, _ := json.Marshal(map[string]string{"error": code})
	_, _ = fmt.Fprintln(stderr, string(encoded))
	return status
}

func validID(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func pathWithoutQuery(value string) string {
	path, _, _ := strings.Cut(value, "?")
	return path
}

func rawQuery(value string) string {
	_, query, _ := strings.Cut(value, "?")
	return query
}

func envOr(getenv func(string) string, key, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}

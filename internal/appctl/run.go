package appctl

import (
	"bytes"
	"context"
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

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const maxResponseBytes = 1 << 20

const usageLine = "usage: appctl <clients|fields|transactions> <command> [flags]"

// collections maps each resource to its collection path under a workspace.
var collections = map[string]string{
	"clients": "/clients",
	"fields":  "/client-fields",
}

// commandFlags names the flags each "resource command" accepts, beyond the
// connection flags that every command takes.
var commandFlags = map[string][]string{
	"clients list":      {"archived", "filter", "sort", "page", "limit"},
	"clients show":      {"client"},
	"clients create":    {"display-name", "email", "phone", "notes"},
	"clients edit":      {"client", "version", "set-display-name", "set-email", "set-phone", "set-notes"},
	"clients archive":   {"client"},
	"clients unarchive": {"client"},
	"fields define":     {"label", "type"},
	"fields archive":    {"field"},
}

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

// multiValue is a flag that may repeat; the values keep their order.
type multiValue []string

func (values *multiValue) String() string { return strings.Join(*values, ",") }
func (values *multiValue) Set(raw string) error {
	*values = append(*values, raw)
	return nil
}

func Run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		return usage(stderr, usageLine)
	}
	if args[0] == "transactions" {
		return runTransactions(args, getenv, stdout, stderr)
	}
	resource, command := args[0], args[1]
	if _, ok := collections[resource]; !ok {
		return usage(stderr, usageLine)
	}
	key := resource + " " + command
	allowed, known := commandFlags[key]
	if !known {
		return usage(stderr, "unknown "+resource+" command")
	}
	flags := flag.NewFlagSet("appctl "+key, flag.ContinueOnError)
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
	workspaceID := flags.String("workspace", "", "workspace short ID")
	page := flags.String("page", "", "1-based page number")
	sort := flags.String("sort", "", "sort order")
	limit := flags.Int("limit", 50, "page size")
	version := flags.Int64("version", 0, "record version")
	clientID := flags.String("client", "", "client short ID")
	fieldID := flags.String("field", "", "client field short ID")
	displayName := flags.String("display-name", "", "client display name")
	email := flags.String("email", "", "client email")
	phone := flags.String("phone", "", "client phone")
	notes := flags.String("notes", "", "client notes")
	archived := flags.String("archived", "", "active, archived, or all (default active)")
	var filters multiValue
	flags.Var(&filters, "filter", "field:operator:value condition; repeat to combine with AND")
	var setDisplayName, setEmail, setPhone, setNotes optionalString
	flags.Var(&setDisplayName, "set-display-name", "new display name")
	flags.Var(&setEmail, "set-email", "new email; empty clears it")
	flags.Var(&setPhone, "set-phone", "new phone; empty clears it")
	flags.Var(&setNotes, "set-notes", "new notes; empty clears them")
	label := flags.String("label", "", "field label")
	fieldType := flags.String("type", "", "text, number, date, yes_no, or single_choice")
	if err := flags.Parse(args[2:]); err != nil || len(flags.Args()) != 0 {
		return usage(stderr, "flags are invalid")
	}
	allowedFlags := map[string]bool{
		"base-url": true, "token": true, "timeout": true, "workspace": true,
	}
	for _, name := range allowed {
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
	if !validShortID(*workspaceID) {
		return usage(stderr, "--workspace must be a workspace short ID")
	}
	path := "/api/v1/workspaces/" + *workspaceID + collections[resource]
	method, body := http.MethodGet, any(nil)
	headers := make(http.Header)
	switch key {
	case "clients list":
		if *limit < 1 || *limit > 100 || (*archived != "" && *archived != "active" && *archived != "archived" && *archived != "all") {
			return usage(stderr, "list flags are invalid")
		}
		query := make(url.Values)
		query.Set("limit", strconv.Itoa(*limit))
		if *sort != "" {
			query.Set("sort", *sort)
		}
		if *archived != "" {
			query.Set("archived", *archived)
		}
		if *page != "" {
			query.Set("page", *page)
		}
		for _, filter := range filters {
			query.Add("filter", filter)
		}
		path += "?" + query.Encode()
	case "clients show":
		if !validShortID(*clientID) {
			return usage(stderr, "--client must be a client short ID")
		}
		path += "/" + *clientID
	case "clients create":
		if strings.TrimSpace(*displayName) == "" {
			return usage(stderr, "create requires --display-name")
		}
		payload := map[string]string{"displayName": *displayName}
		if *email != "" {
			payload["email"] = *email
		}
		if *phone != "" {
			payload["phone"] = *phone
		}
		if *notes != "" {
			payload["notes"] = *notes
		}
		method, body = http.MethodPost, payload
	case "clients edit":
		if !validShortID(*clientID) || *version < 1 ||
			(!setDisplayName.set && !setEmail.set && !setPhone.set && !setNotes.set) {
			return usage(stderr, "edit requires --client, --version, and at least one --set- flag")
		}
		values := make(map[string]string)
		if setDisplayName.set {
			values["displayName"] = setDisplayName.value
		}
		if setEmail.set {
			values["email"] = setEmail.value
		}
		if setPhone.set {
			values["phone"] = setPhone.value
		}
		if setNotes.set {
			values["notes"] = setNotes.value
		}
		method, body, path = http.MethodPatch, values, path+"/"+*clientID
		headers.Set("If-Match", fmt.Sprintf(`"v%d"`, *version))
	case "clients archive", "clients unarchive":
		if !validShortID(*clientID) {
			return usage(stderr, "--client must be a client short ID")
		}
		method, path = http.MethodPost, path+"/"+*clientID+"/"+command
	case "fields define":
		if strings.TrimSpace(*label) == "" || *fieldType == "" {
			return usage(stderr, "define requires --label and --type")
		}
		switch *fieldType {
		case "text", "number", "date", "yes_no", "single_choice":
		default:
			return usage(stderr, "--type must be text, number, date, yes_no, or single_choice")
		}
		method, body = http.MethodPost, map[string]string{"label": *label, "type": *fieldType}
	case "fields archive":
		if !validShortID(*fieldID) {
			return usage(stderr, "--field must be a client field short ID")
		}
		method, path = http.MethodPost, path+"/"+*fieldID+"/archive"
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

// validShortID checks only the shape of a workspace or item short ID, because
// the value becomes a URL path segment. The server decodes it strictly.
func validShortID(value string) bool {
	if len(value) < 4 || len(value) > 32 {
		return false
	}
	for index := range len(value) {
		if !strings.ContainsRune(id.ShortIDAlphabet, rune(value[index])) {
			return false
		}
	}
	return true
}

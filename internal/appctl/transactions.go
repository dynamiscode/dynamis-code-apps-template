package appctl

import (
	"flag"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// runTransactions sends transactions commands to the versioned REST API.
func runTransactions(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[1] != "create" && args[1] != "list" && args[1] != "totals" && args[1] != "methods" && args[1] != "method-create" && args[1] != "method-update" && args[1] != "void" && args[1] != "categories" && args[1] != "category-create" && args[1] != "category-rename" && args[1] != "category-archive") {
		return usage(stderr, "unknown transactions command")
	}
	command := args[1]
	flags := flag.NewFlagSet("appctl transactions "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	timeout := 30 * time.Second
	if raw := getenv("APPCTL_TIMEOUT"); raw != "" {
		var err error
		timeout, err = time.ParseDuration(raw)
		if err != nil {
			return usage(stderr, "APPCTL_TIMEOUT is invalid")
		}
	}
	common := options{baseURL: envOr(getenv, "APPCTL_BASE_URL", "http://127.0.0.1:8080"), token: getenv("APPCTL_TOKEN"), timeout: timeout}
	flags.StringVar(&common.baseURL, "base-url", common.baseURL, "remote server URL")
	flags.StringVar(&common.token, "token", common.token, "API bearer token")
	flags.DurationVar(&common.timeout, "timeout", common.timeout, "request timeout")
	workspace := flags.String("workspace", "", "workspace short ID")
	page := flags.Int("page", 1, "1-based page number")
	period := flags.String("period", "", "this_month, last_month, or custom")
	start := flags.String("start", "", "custom period start date (YYYY-MM-DD)")
	end := flags.String("end", "", "custom period end date (YYYY-MM-DD)")
	description := flags.String("description", "", "entry description")
	direction := flags.String("direction", "", "in or out")
	amount := flags.Int64("amount-minor", 0, "amount in minor units")
	currency := flags.String("currency", "", "currency code")
	method := flags.String("payment-method", "", "payment method")
	occurred := flags.String("occurred-on", "", "date in YYYY-MM-DD format")
	client := flags.String("client", "", "client short ID")
	category := flags.String("category", "", "expense category short ID")
	from := flags.String("from", "", "inclusive start date in YYYY-MM-DD format")
	to := flags.String("to", "", "inclusive end date in YYYY-MM-DD format")
	name := flags.String("name", "", "category or payment method name")
	note := flags.String("note", "", "entry note")
	methodID := flags.String("id", "", "payment method ID")
	enabled := flags.Bool("enabled", true, "payment method enabled")
	entry := flags.String("entry", "", "transactions entry short ID")
	reason := flags.String("reason", "", "reason for voiding")
	replaces := flags.String("replaces-entry", "", "voided transactions entry short ID")
	refund := flags.String("refund-of-entry", "", "money-in transactions entry short ID")
	if err := flags.Parse(args[2:]); err != nil || len(flags.Args()) != 0 {
		return usage(stderr, "flags are invalid")
	}
	invalid := false
	flags.Visit(func(value *flag.Flag) {
		if command != "create" {
			switch value.Name {
			case "description", "amount-minor", "currency", "occurred-on", "note", "replaces-entry", "refund-of-entry":
				invalid = true
			}
		}
		if command != "create" && command != "list" && (value.Name == "direction" || value.Name == "payment-method" || value.Name == "client") {
			invalid = true
		}
		if command != "list" && (value.Name == "from" || value.Name == "to") {
			invalid = true
		}
		if value.Name == "category" && command != "create" && command != "list" && command != "category-rename" && command != "category-archive" {
			invalid = true
		}
		if command != "void" && (value.Name == "entry" || value.Name == "reason") {
			invalid = true
		}
		if command != "list" && value.Name == "page" {
			invalid = true
		}
		if command != "totals" && (value.Name == "period" || value.Name == "start" || value.Name == "end") {
			invalid = true
		}
		if value.Name == "id" && command != "method-update" {
			invalid = true
		}
		if value.Name == "name" && command != "method-create" && command != "method-update" && command != "category-create" && command != "category-rename" {
			invalid = true
		}
		if value.Name == "enabled" && command != "method-update" {
			invalid = true
		}
	})
	if invalid {
		return usage(stderr, "flag is not valid for this command")
	}
	base, err := validateOptions(common)
	if err != nil {
		return usage(stderr, err.Error())
	}
	if !validShortID(*workspace) {
		return usage(stderr, "--workspace must be a workspace short ID")
	}
	path := "/api/v1/workspaces/" + *workspace + "/transactions/entries"
	verb, body := http.MethodGet, any(nil)
	query := url.Values{}
	if command == "list" {
		if *page < 1 || *page > 1000000 {
			return usage(stderr, "--page must be from 1 to 1000000")
		}
		query.Set("page", strconv.Itoa(*page))
		if (*client != "" && !validShortID(*client)) || (*category != "" && !validShortID(*category)) {
			return usage(stderr, "--client and --category must be short IDs")
		}
		for key, value := range map[string]string{"clientId": *client, "categoryId": *category, "paymentMethod": *method, "direction": *direction, "from": *from, "to": *to} {
			if value != "" {
				query.Set(key, value)
			}
		}
	}
	if command == "totals" {
		path = "/api/v1/workspaces/" + *workspace + "/transactions/totals"
		if *period != "" {
			query.Set("period", *period)
		}
		if *start != "" {
			query.Set("start", *start)
		}
		if *end != "" {
			query.Set("end", *end)
		}
	}
	if command == "totals" {
		path = "/api/v1/workspaces/" + *workspace + "/transactions/totals"
		if *period != "" {
			query.Set("period", *period)
		}
		if *start != "" {
			query.Set("start", *start)
		}
		if *end != "" {
			query.Set("end", *end)
		}
	}
	if command == "methods" || command == "method-create" || command == "method-update" {
		path = "/api/v1/workspaces/" + *workspace + "/transactions/methods"
		if command == "method-create" {
			if *name == "" {
				return usage(stderr, "--name is required")
			}
			verb, body = http.MethodPost, map[string]any{"name": *name}
		} else if command == "method-update" {
			hasEnabled := false
			flags.Visit(func(value *flag.Flag) {
				hasEnabled = hasEnabled || value.Name == "enabled"
			})
			if *methodID == "" || *name == "" || !hasEnabled {
				return usage(stderr, "--id, --name, and --enabled are required")
			}
			verb, body = http.MethodPut, map[string]any{"id": *methodID, "name": *name, "enabled": *enabled}
		}
	} else if command == "create" {
		if *client != "" && !validShortID(*client) {
			return usage(stderr, "--client must be a client short ID")
		}
		if (*replaces != "" && !validShortID(*replaces)) || (*refund != "" && !validShortID(*refund)) {
			return usage(stderr, "correction links must be transactions entry short IDs")
		}
		payload := map[string]any{"description": *description, "direction": *direction, "amountMinor": *amount, "currency": *currency, "paymentMethod": *method}
		if *occurred != "" {
			payload["occurredOn"] = *occurred
		}
		if *client != "" {
			payload["clientId"] = *client
		}
		if *category != "" {
			if !validShortID(*category) {
				return usage(stderr, "--category must be a category short ID")
			}
			payload["categoryId"] = *category
		}
		if *note != "" {
			payload["note"] = *note
		}
		if *replaces != "" {
			payload["replacesEntryId"] = *replaces
		}
		if *refund != "" {
			payload["refundOfEntryId"] = *refund
		}
		verb, body = http.MethodPost, payload
	} else if command == "categories" || command == "category-create" || command == "category-rename" || command == "category-archive" {
		path = "/api/v1/workspaces/" + *workspace + "/transactions/categories"
		if command == "category-create" {
			if *name == "" {
				return usage(stderr, "--name is required")
			}
			verb, body = http.MethodPost, map[string]any{"name": *name}
		} else if command == "category-rename" || command == "category-archive" {
			if !validShortID(*category) {
				return usage(stderr, "--category must be a category short ID")
			}
			path += "/" + *category
			if command == "category-rename" {
				if *name == "" {
					return usage(stderr, "--name is required")
				}
				verb, body = http.MethodPatch, map[string]any{"name": *name}
			} else {
				verb = http.MethodDelete
			}
		}
	} else if command == "void" {
		if !validShortID(*entry) {
			return usage(stderr, "--entry is required")
		}
		path += "/" + *entry + "/void"
		verb, body = http.MethodPost, map[string]any{"reason": *reason}
	}
	return execute(common, base.ResolveReference(&url.URL{Path: path, RawQuery: query.Encode()}), verb, body, nil, stdout, stderr)
}

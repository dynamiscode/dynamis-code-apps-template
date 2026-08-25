package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type DatabaseDriver string

const (
	SQLite   DatabaseDriver = "sqlite"
	Postgres DatabaseDriver = "postgres"
)

type Database struct {
	Driver       DatabaseDriver
	SQLitePath   string
	URL          string
	MaxOpenConns int
	MaxIdleConns int
}

type Config struct {
	Database  Database
	Bootstrap Bootstrap
	OIDC      OIDC
	Mail      Mail
	PublicURL string
	HTTP      HTTP
	MCP       MCP
	Data      Data
	Telemetry Telemetry
}

type Bootstrap struct {
	AdminEmail     string
	AdminPassword  string
	AdminWorkspace string
	SetupToken     string
}

type OIDC struct {
	Enabled      bool
	ProviderID   string
	ProviderName string
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type Mail struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type HTTP struct {
	Address            string
	Secure             bool
	ReadHeaderTimeout  time.Duration
	RequestTimeout     time.Duration
	ShutdownTimeout    time.Duration
	ReadinessTimeout   time.Duration
	MaxHeaderBytes     int
	MaxBodyBytes       int64
	DefaultPageSize    int
	MaxPageSize        int
	RequestsPerMinute  int
	AuthRequestsPerMin int
	SSEPollInterval    time.Duration
	SSEHeartbeat       time.Duration
	SSEMaxLifetime     time.Duration
	SSEMaxConnections  int
	SSEMaxPerUser      int
	MaxConcurrent      int
}
var providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

type LookupEnv func(string) (string, bool)

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup LookupEnv) (Config, error) {
	driver := DatabaseDriver(valueOrDefault(lookup, "DATABASE_DRIVER", string(SQLite)))
	if driver != SQLite && driver != Postgres {
		return Config{}, fmt.Errorf(
			"DATABASE_DRIVER must be %q or %q",
			SQLite,
			Postgres,
		)
	}

	maxOpen, err := positiveInt(lookup, "DATABASE_MAX_OPEN_CONNS", 4)
	if err != nil {
		return Config{}, err
	}
	maxIdle, err := positiveInt(lookup, "DATABASE_MAX_IDLE_CONNS", 2)
	if err != nil {
		return Config{}, err
	}
	if maxIdle > maxOpen {
		return Config{}, fmt.Errorf(
			"DATABASE_MAX_IDLE_CONNS must not exceed DATABASE_MAX_OPEN_CONNS",
		)
	}

	database := Database{
		Driver:       driver,
		MaxOpenConns: maxOpen,
		MaxIdleConns: maxIdle,
	}

	if driver == SQLite {
		database.SQLitePath = valueOrDefault(lookup, "SQLITE_PATH", "data/app.db")
		if strings.TrimSpace(database.SQLitePath) == "" {
			return Config{}, fmt.Errorf("SQLITE_PATH must not be empty")
		}
	} else {
		database.URL = valueOrDefault(lookup, "DATABASE_URL", "")
		if strings.TrimSpace(database.URL) == "" {
			return Config{}, fmt.Errorf(
				"DATABASE_URL is required when DATABASE_DRIVER is %q",
				Postgres,
			)
		}
	}

	bootstrap, err := loadBootstrap(lookup)
	if err != nil {
		return Config{}, err
	}
	oidc, err := loadOIDC(lookup)
	if err != nil {
		return Config{}, err
	}
	publicURL, err := loadPublicURL(lookup)
	if err != nil {
		return Config{}, err
	}
	mailConfig, err := loadMail(lookup, publicURL)
	if err != nil {
		return Config{}, err
	}
	httpConfig, err := loadHTTP(lookup)
	if err != nil {
		return Config{}, err
	}
	mcpConfig, err := loadMCP(lookup)
	if err != nil {
		return Config{}, err
	}
	dataConfig, err := loadData(lookup)
	if err != nil {
		return Config{}, err
	}
	telemetryConfig, err := loadTelemetry(lookup)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Database: database, Bootstrap: bootstrap, OIDC: oidc, Mail: mailConfig,
		PublicURL: publicURL, HTTP: httpConfig, MCP: mcpConfig,
		Data: dataConfig, Telemetry: telemetryConfig,
	}, nil
}

func loadPublicURL(lookup LookupEnv) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(valueOrDefault(lookup, "APP_PUBLIC_URL", "")), "/")
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()))) {
		return "", fmt.Errorf("APP_PUBLIC_URL must use HTTPS or loopback HTTP")
	}
	return value, nil
}

func loadMail(lookup LookupEnv, publicURL string) (Mail, error) {
	host, hostSet := lookup("SMTP_HOST")
	portRaw, portSet := lookup("SMTP_PORT")
	username, usernameSet := lookup("SMTP_USERNAME")
	password, passwordSet := lookup("SMTP_PASSWORD")
	from, fromSet := lookup("SMTP_FROM")
	configured := hostSet || portSet || usernameSet || passwordSet || fromSet
	if !configured {
		return Mail{}, nil
	}
	host = strings.TrimSpace(host)
	if host == "" || publicURL == "" || (usernameSet != passwordSet) {
		return Mail{}, fmt.Errorf("SMTP configuration requires SMTP_HOST, APP_PUBLIC_URL, and matching username/password")
	}
	port := 587
	if portSet {
		value, err := strconv.Atoi(strings.TrimSpace(portRaw))
		if err != nil || value < 1 || value > 65535 {
			return Mail{}, fmt.Errorf("SMTP_PORT must be an integer from 1 to 65535")
		}
		port = value
	}
	from = strings.TrimSpace(from)
	if from == "" || strings.ContainsAny(from, "\r\n") {
		return Mail{}, fmt.Errorf("SMTP_FROM must be a valid address")
	}
	if usernameSet && strings.TrimSpace(username) == "" {
		return Mail{}, fmt.Errorf("SMTP_USERNAME must not be empty when SMTP is configured")
	}
	return Mail{Host: host, Port: port, Username: username, Password: password, From: from}, nil
}

func loadBootstrap(lookup LookupEnv) (Bootstrap, error) {
	email, _ := lookup("BOOTSTRAP_ADMIN_EMAIL")
	workspace, _ := lookup("BOOTSTRAP_ADMIN_WORKSPACE")
	password, _ := lookup("BOOTSTRAP_ADMIN_PASSWORD")
	setupToken := valueOrDefault(lookup, "BOOTSTRAP_SETUP_TOKEN", "")
	if strings.TrimSpace(setupToken) == "" {
		setupToken = ""
	}
	return Bootstrap{
		AdminEmail: strings.TrimSpace(email), AdminPassword: password,
		AdminWorkspace: strings.TrimSpace(workspace), SetupToken: setupToken,
	}, nil
}

func loadHTTP(lookup LookupEnv) (HTTP, error) {
	secure, err := boolValue(lookup, "HTTP_SECURE", false)
	if err != nil {
		return HTTP{}, err
	}
	readHeaderTimeout, err := durationValue(
		lookup, "HTTP_READ_HEADER_TIMEOUT", 10*time.Second, time.Second, time.Minute,
	)
	if err != nil {
		return HTTP{}, err
	}
	requestTimeout, err := durationValue(
		lookup, "HTTP_REQUEST_TIMEOUT", 30*time.Second, time.Second, 5*time.Minute,
	)
	if err != nil {
		return HTTP{}, err
	}
	shutdownTimeout, err := durationValue(
		lookup, "HTTP_SHUTDOWN_TIMEOUT", 10*time.Second, time.Second, 2*time.Minute,
	)
	if err != nil {
		return HTTP{}, err
	}
	readinessTimeout, err := durationValue(
		lookup, "HTTP_READINESS_TIMEOUT", 2*time.Second, 100*time.Millisecond, 30*time.Second,
	)
	if err != nil {
		return HTTP{}, err
	}
	maxHeaderBytes, err := rangedInt(
		lookup, "HTTP_MAX_HEADER_BYTES", 32*1024, 8*1024, 1024*1024,
	)
	if err != nil {
		return HTTP{}, err
	}
	maxBodyBytes, err := rangedInt(
		lookup, "HTTP_MAX_BODY_BYTES", 1024*1024, 1024, 16*1024*1024,
	)
	if err != nil {
		return HTTP{}, err
	}
	maxPageSize, err := rangedInt(lookup, "HTTP_MAX_PAGE_SIZE", 100, 1, 100)
	if err != nil {
		return HTTP{}, err
	}
	defaultPageSize, err := rangedInt(
		lookup, "HTTP_DEFAULT_PAGE_SIZE", 50, 1, maxPageSize,
	)
	if err != nil {
		return HTTP{}, err
	}
	requestsPerMinute, err := rangedInt(
		lookup, "HTTP_REQUESTS_PER_MINUTE", 120, 1, 10000,
	)
	if err != nil {
		return HTTP{}, err
	}
	authRequestsPerMin, err := rangedInt(
		lookup, "HTTP_AUTH_REQUESTS_PER_MINUTE", 10, 1, requestsPerMinute,
	)
	if err != nil {
		return HTTP{}, err
	}
	address := strings.TrimSpace(valueOrDefault(lookup, "HTTP_ADDRESS", ":8080"))
	if address == "" {
		return HTTP{}, fmt.Errorf("HTTP_ADDRESS must not be empty")
	}
	return HTTP{
		Address: address, Secure: secure,
		ReadHeaderTimeout: readHeaderTimeout, RequestTimeout: requestTimeout,
		ShutdownTimeout: shutdownTimeout, ReadinessTimeout: readinessTimeout,
		MaxHeaderBytes: maxHeaderBytes, MaxBodyBytes: int64(maxBodyBytes),
		DefaultPageSize: defaultPageSize, MaxPageSize: maxPageSize,
		RequestsPerMinute:  requestsPerMinute,
		AuthRequestsPerMin: authRequestsPerMin,
	}, nil
}

func loadOIDC(lookup LookupEnv) (OIDC, error) {
	enabled, err := boolValue(lookup, "OIDC_ENABLED", false)
	if err != nil {
		return OIDC{}, err
	}
	if !enabled {
		return OIDC{}, nil
	}
	oidc := OIDC{
		Enabled:      true,
		ProviderID:   strings.TrimSpace(valueOrDefault(lookup, "OIDC_PROVIDER_ID", "")),
		ProviderName: strings.TrimSpace(valueOrDefault(lookup, "OIDC_PROVIDER_NAME", "")),
		IssuerURL:    strings.TrimSpace(valueOrDefault(lookup, "OIDC_ISSUER_URL", "")),
		ClientID:     strings.TrimSpace(valueOrDefault(lookup, "OIDC_CLIENT_ID", "")),
		ClientSecret: valueOrDefault(lookup, "OIDC_CLIENT_SECRET", ""),
		RedirectURL:  strings.TrimSpace(valueOrDefault(lookup, "OIDC_REDIRECT_URL", "")),
	}
	if !providerIDPattern.MatchString(oidc.ProviderID) {
		return OIDC{}, fmt.Errorf("OIDC_PROVIDER_ID must be a lowercase stable identifier")
	}
	if oidc.ProviderName == "" || len(oidc.ProviderName) > 80 {
		return OIDC{}, fmt.Errorf("OIDC_PROVIDER_NAME must be 1 to 80 characters")
	}
	if oidc.ClientID == "" {
		return OIDC{}, fmt.Errorf("OIDC_CLIENT_ID is required when OIDC is enabled")
	}
	if oidc.ClientSecret == "" {
		return OIDC{}, fmt.Errorf("OIDC_CLIENT_SECRET is required when OIDC is enabled")
	}
	issuer, err := url.Parse(oidc.IssuerURL)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" ||
		issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" {
		return OIDC{}, fmt.Errorf("OIDC_ISSUER_URL must be an HTTPS issuer URL")
	}
	redirect, err := url.Parse(oidc.RedirectURL)
	if err != nil || redirect.Host == "" || redirect.User != nil ||
		redirect.RawQuery != "" || redirect.Fragment != "" ||
		(redirect.Scheme != "https" &&
			!(redirect.Scheme == "http" && isLoopbackHost(redirect.Hostname()))) {
		return OIDC{}, fmt.Errorf("OIDC_REDIRECT_URL must use HTTPS or loopback HTTP")
	}
	return oidc, nil
}

func boolValue(lookup LookupEnv, key string, fallback bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func valueOrDefault(lookup LookupEnv, key, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}
	return value
}

func positiveInt(lookup LookupEnv, key string, fallback int) (int, error) {
	return rangedInt(lookup, key, fallback, 1, 64)
}

func rangedInt(
	lookup LookupEnv,
	key string,
	fallback int,
	minimum int,
	maximum int,
) (int, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf(
			"%s must be an integer from %d to %d", key, minimum, maximum,
		)
	}
	return value, nil
}

func durationValue(
	lookup LookupEnv,
	key string,
	fallback time.Duration,
	minimum time.Duration,
	maximum time.Duration,
) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf(
			"%s must be a duration from %s to %s", key, minimum, maximum,
		)
	}
	return value, nil
}

package httpapi

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maximumRateLimitSources = 4096

type rateWindow struct {
	started time.Time
	count   int
}

type rateLimiter struct {
	mu       sync.Mutex
	windows  map[string]rateWindow
	ordinary int
	auth     int
	now      func() time.Time
	window   time.Duration
}

func newRateLimiter(ordinary int, auth int) *rateLimiter {
	return &rateLimiter{
		windows: make(map[string]rateWindow), ordinary: ordinary, auth: auth,
		now: time.Now, window: time.Minute,
	}
}

func (limiter *rateLimiter) allow(key string, authentication bool) (bool, int) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	limit := limiter.ordinary
	prefix := "ordinary:"
	if authentication {
		limit = limiter.auth
		prefix = "auth:"
	}
	key = prefix + key
	window, ok := limiter.windows[key]
	if !ok || now.Sub(window.started) >= limiter.window {
		if !ok && len(limiter.windows) >= maximumRateLimitSources {
			limiter.removeExpired(now)
			if len(limiter.windows) >= maximumRateLimitSources {
				return false, 1
			}
		}
		limiter.windows[key] = rateWindow{started: now, count: 1}
		return true, 0
	}
	if window.count >= limit {
		retry := int(math.Ceil(window.started.Add(limiter.window).Sub(now).Seconds()))
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	window.count++
	limiter.windows[key] = window
	return true, 0
}

func (limiter *rateLimiter) removeExpired(now time.Time) {
	for key, window := range limiter.windows {
		if now.Sub(window.started) >= limiter.window {
			delete(limiter.windows, key)
		}
	}
}

func rateLimitMiddleware(next http.Handler, limiter *rateLimiter) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authentication := strings.HasPrefix(request.URL.Path, "/api/v1/auth/") ||
			request.URL.Path == "/login" || request.URL.Path == "/setup" ||
			strings.HasPrefix(request.URL.Path, "/mfa") ||
			strings.HasPrefix(request.URL.Path, "/security/")
		allowed, retry := limiter.allow(sourceHost(request.RemoteAddr), authentication)
		if !allowed {
			writer.Header().Set("Retry-After", strconv.Itoa(retry))
			writeProblem(
				writer, request, http.StatusTooManyRequests, "rate-limited",
				"Too many requests. Retry after the indicated delay.",
			)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func sourceHost(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	if len(remoteAddress) > 128 {
		return remoteAddress[:128]
	}
	return remoteAddress
}

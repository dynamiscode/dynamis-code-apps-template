package httpapi

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
	"go.opentelemetry.io/otel/trace"
)

const maxBufferedResponseBytes = 4 * 1024 * 1024

var errResponseTooLarge = errors.New("response exceeds buffer limit")

func middleware(
	next http.Handler,
	cfg config.HTTP,
	limiter *rateLimiter,
	logger *slog.Logger,
) http.Handler {
	next = timeoutMiddleware(next, cfg.RequestTimeout, logger)
	next = concurrencyMiddleware(next, cfg.MaxConcurrent)
	next = rateLimitMiddleware(next, limiter)
	next = bodyLimitMiddleware(next, cfg.MaxBodyBytes)
	next = securityHeadersMiddleware(next, cfg.Secure)
	next = loggingMiddleware(next, logger)
	return requestIDMiddleware(next)
}

func concurrencyMiddleware(next http.Handler, maximum int) http.Handler {
	active := make(chan struct{}, maximum)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/workspaces/") &&
			strings.HasSuffix(request.URL.Path, "/items/events") {
			next.ServeHTTP(writer, request)
			return
		}
		select {
		case active <- struct{}{}:
			defer func() { <-active }()
			next.ServeHTTP(writer, request)
		default:
			telemetry.RecordLimitRejection(request.Context(), "http_request")
			writer.Header().Set("Retry-After", "1")
			writeProblem(writer, request, http.StatusTooManyRequests,
				"concurrency-limit", "The concurrent request limit was reached.")
		}
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("X-Request-ID")
		if !validRequestID(requestID) {
			var err error
			requestID, err = id.New()
			if err != nil {
				writeProblem(
					writer, request, http.StatusInternalServerError,
					"request-id-failed", "The request could not be started.",
				)
				return
			}
		}
		writer.Header().Set("X-Request-ID", requestID)
		request.Header.Set("X-Request-ID", requestID)
		next.ServeHTTP(writer, request.WithContext(
			withRequestID(request.Context(), requestID),
		))
	})
}

func validRequestID(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func securityHeadersMiddleware(next http.Handler, secure bool) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		headers := writer.Header()
		headers.Set(
			"Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'",
		)
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("Referrer-Policy", "no-referrer")
		headers.Set("Permissions-Policy", "tools=(self)")
		headers.Set("Origin-Agent-Cluster", "?1")
		if secure {
			headers.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(writer, request)
	})
}

func bodyLimitMiddleware(next http.Handler, maximum int64) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Body != nil {
			request.Body = http.MaxBytesReader(writer, request.Body, maximum)
		}
		next.ServeHTTP(writer, request)
	})
}

type bufferedResponse struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	writeErr error
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header)}
}

func (response *bufferedResponse) Header() http.Header {
	return response.header
}

func (response *bufferedResponse) WriteHeader(status int) {
	if response.status == 0 {
		response.status = status
	}
}

func (response *bufferedResponse) Write(value []byte) (int, error) {
	if response.status == 0 {
		response.status = http.StatusOK
	}
	if response.body.Len()+len(value) > maxBufferedResponseBytes {
		response.writeErr = errResponseTooLarge
		return 0, errResponseTooLarge
	}
	return response.body.Write(value)
}

type handlerResult struct {
	response *bufferedResponse
	panicked bool
}

func timeoutMiddleware(
	next http.Handler,
	timeout time.Duration,
	logger *slog.Logger,
) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/workspaces/") &&
			strings.HasSuffix(request.URL.Path, "/items/events") {
			next.ServeHTTP(writer, request)
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), timeout)
		defer cancel()
		result := make(chan handlerResult, 1)
		response := newBufferedResponse()
		go func() {
			outcome := handlerResult{response: response}
			defer func() {
				if recover() != nil {
					outcome.panicked = true
				}
				result <- outcome
			}()
			next.ServeHTTP(response, request.WithContext(ctx))
		}()
		select {
		case outcome := <-result:
			if outcome.panicked || outcome.response.writeErr != nil {
				logger.Error("request handler failed", "request_id", requestIDFrom(ctx))
				internalProblem(writer, request.WithContext(ctx))
				return
			}
			copyHeaders(writer.Header(), outcome.response.header)
			status := outcome.response.status
			if status == 0 {
				status = http.StatusOK
			}
			writer.WriteHeader(status)
			_, _ = writer.Write(outcome.response.body.Bytes())
		case <-ctx.Done():
			writeProblem(
				writer, request.WithContext(ctx), http.StatusGatewayTimeout,
				"request-timeout", "The request exceeded its time limit.",
			)
		}
	})
}

func copyHeaders(destination http.Header, source http.Header) {
	for key, values := range source {
		destination.Del(key)
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (writer *statusWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (writer *statusWriter) WriteHeader(status int) {
	if writer.status == 0 {
		writer.status = status
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *statusWriter) Write(value []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	return writer.ResponseWriter.Write(value)
}

func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		response := &statusWriter{ResponseWriter: writer}
		next.ServeHTTP(response, request)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		attributes := []any{
			"request_id", requestIDFrom(request.Context()),
			"method", request.Method,
			"path", logPath(request.URL.Path),
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
		}
		if traceparent := request.Header.Get("traceparent"); validTraceparent(traceparent) {
			attributes = append(attributes, "traceparent", traceparent)
		}
		if span := trace.SpanContextFromContext(request.Context()); span.IsValid() {
			attributes = append(attributes, "trace_id", span.TraceID().String())
		}
		logger.Info("http request", attributes...)
	})
}

func logPath(path string) string {
	if strings.HasPrefix(path, "/share/") {
		return "/share/[redacted]"
	}
	return path
}

func validTraceparent(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) != 4 || len(parts[0]) != 2 || len(parts[1]) != 32 ||
		len(parts[2]) != 16 || len(parts[3]) != 2 {
		return false
	}
	for _, part := range parts {
		if strings.ToLower(part) != part {
			return false
		}
		if _, err := hex.DecodeString(part); err != nil {
			return false
		}
	}
	return parts[0] == "00" && parts[1] != strings.Repeat("0", 32) &&
		parts[2] != strings.Repeat("0", 16)
}

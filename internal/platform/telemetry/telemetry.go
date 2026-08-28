package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "example.com/dynamis-code/apps-template"

var (
	meter                      = otel.Meter(instrumentationName)
	httpRequests, _            = meter.Int64Counter("http.server.request.count")
	httpDuration, _            = meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"))
	httpClientRequests, _      = meter.Int64Counter("http.client.request.count")
	httpClientDuration, _      = meter.Float64Histogram("http.client.request.duration", metric.WithUnit("s"))
	authFailures, _            = meter.Int64Counter("auth.failure.count")
	databaseChecks, _          = meter.Int64Counter("database.health.check.count")
	databaseDuration, _        = meter.Float64Histogram("database.health.check.duration", metric.WithUnit("s"))
	activeStreams, _           = meter.Int64UpDownCounter("realtime.stream.active")
	streamRejections, _        = meter.Int64Counter("realtime.stream.rejected.count")
	resourceLimitRejections, _ = meter.Int64Counter("resource.limit.rejected.count")
	webhookDeliveries, _       = meter.Int64Counter("webhook.delivery.count")
)

type roundTripper struct {
	base http.RoundTripper
}

type Provider struct {
	traces  *sdktrace.TracerProvider
	metrics *sdkmetric.MeterProvider
}

func New(ctx context.Context, cfg config.Telemetry) (*Provider, error) {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
	)
	traceOptions := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	metricOptions := []sdkmetric.Option{sdkmetric.WithResource(res)}
	if cfg.Endpoint != "" {
		traceExporter, err := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(cfg.Endpoint+"/v1/traces"),
			otlptracehttp.WithTimeout(cfg.ExportTimeout),
		)
		if err != nil {
			return nil, fmt.Errorf("initialize trace exporter: %w", err)
		}
		metricExporter, err := otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithEndpointURL(cfg.Endpoint+"/v1/metrics"),
			otlpmetrichttp.WithTimeout(cfg.ExportTimeout),
		)
		if err != nil {
			_ = traceExporter.Shutdown(ctx)
			return nil, fmt.Errorf("initialize metric exporter: %w", err)
		}
		traceOptions = append(traceOptions, sdktrace.WithBatcher(traceExporter))
		metricOptions = append(metricOptions, sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(metricExporter,
				sdkmetric.WithInterval(cfg.ExportInterval),
				sdkmetric.WithTimeout(cfg.ExportTimeout),
			),
		))
	}
	provider := &Provider{
		traces:  sdktrace.NewTracerProvider(traceOptions...),
		metrics: sdkmetric.NewMeterProvider(metricOptions...),
	}
	otel.SetTracerProvider(provider.traces)
	otel.SetMeterProvider(provider.metrics)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return provider, nil
}

func (provider *Provider) Shutdown(ctx context.Context) error {
	return errors.Join(provider.metrics.Shutdown(ctx), provider.traces.Shutdown(ctx))
}

func HTTPHandler(next http.Handler) http.Handler {
	tracer := otel.Tracer(instrumentationName)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(
			request.Context(), propagation.HeaderCarrier(request.Header),
		)
		ctx, span := tracer.Start(ctx, "HTTP "+request.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", request.Method)),
		)
		started := time.Now()
		response := &statusWriter{ResponseWriter: writer}
		next.ServeHTTP(response, request.WithContext(ctx))
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		attributes := metric.WithAttributes(
			attribute.String("http.request.method", request.Method),
			attribute.Int("http.response.status_code", status),
		)
		httpRequests.Add(ctx, 1, attributes)
		httpDuration.Record(ctx, time.Since(started).Seconds(), attributes)
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status == http.StatusUnauthorized {
			authFailures.Add(ctx, 1)
		}
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		span.End()
	})
}

func HTTPClientTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &roundTripper{base: base}
}

func (transport *roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx, span := otel.Tracer(instrumentationName).Start(
		request.Context(), "HTTP "+request.Method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("http.request.method", request.Method)),
	)
	started := time.Now()
	response, err := transport.base.RoundTrip(request.Clone(ctx))
	status := 0
	if response != nil {
		status = response.StatusCode
		span.SetAttributes(attribute.Int("http.response.status_code", status))
	}
	attributes := metric.WithAttributes(
		attribute.String("http.request.method", request.Method),
		attribute.Int("http.response.status_code", status),
	)
	httpClientRequests.Add(ctx, 1, attributes)
	httpClientDuration.Record(ctx, time.Since(started).Seconds(), attributes)
	if err != nil || status >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, "outbound request failed")
	}
	span.End()
	return response, err
}

func RecordDatabaseHealth(ctx context.Context, healthy bool, duration time.Duration) {
	value := "healthy"
	if !healthy {
		value = "unhealthy"
	}
	attributes := metric.WithAttributes(attribute.String("database.health", value))
	databaseChecks.Add(ctx, 1, attributes)
	databaseDuration.Record(ctx, duration.Seconds(), attributes)
}

func RecordStream(ctx context.Context, delta int64, rejected bool) {
	if delta != 0 {
		activeStreams.Add(ctx, delta)
	}
	if rejected {
		streamRejections.Add(ctx, 1)
	}
}

func RecordLimitRejection(ctx context.Context, resource string) {
	resourceLimitRejections.Add(ctx, 1,
		metric.WithAttributes(attribute.String("resource.type", resource)))
}

func RecordWebhookDelivery(ctx context.Context, status string) {
	webhookDeliveries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("webhook.delivery.status", status),
	))
}

type statusWriter struct {
	http.ResponseWriter
	status int
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

func (writer *statusWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

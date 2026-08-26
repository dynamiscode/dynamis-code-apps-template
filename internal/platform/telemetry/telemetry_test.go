package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestHTTPMetricsTraceCorrelationAndRedaction(t *testing.T) {
	previousTracer := otel.GetTracerProvider()
	previousMeter := otel.GetMeterProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousTracer)
		otel.SetMeterProvider(previousMeter)
		otel.SetTextMapPropagator(previousPropagator)
	})
	spans := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		tracerProvider.Shutdown(context.Background())
		meterProvider.Shutdown(context.Background())
	})

	secret := "token-must-not-appear"
	handler := HTTPHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		RecordDatabaseHealth(request.Context(), false, 2*time.Millisecond)
		RecordStream(request.Context(), 0, true)
		RecordLimitRejection(request.Context(), "test_resource")
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	request := httptest.NewRequest(http.MethodGet, "/items?secret="+secret, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	client := &http.Client{Transport: HTTPClientTransport(nil)}
	clientRequest, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, upstream.URL+"/private/"+secret, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	clientResponse, err := client.Do(clientRequest)
	if err != nil {
		t.Fatal(err)
	}
	clientResponse.Body.Close()

	ended := spans.Ended()
	if len(ended) != 2 || ended[0].SpanContext().TraceID().String() != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("ended spans = %+v", ended)
	}
	spanEvidence := fmt.Sprintf("%+v", ended)
	if strings.Contains(spanEvidence, secret) || strings.Contains(spanEvidence, request.URL.Path) {
		t.Fatalf("span leaked request data: %s", spanEvidence)
	}

	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatal(err)
	}
	evidence := fmt.Sprintf("%+v", metrics)
	for _, name := range []string{
		"http.server.request.count", "http.server.request.duration",
		"http.client.request.count", "http.client.request.duration",
		"auth.failure.count", "database.health.check.count",
		"database.health.check.duration", "realtime.stream.rejected.count",
		"resource.limit.rejected.count",
	} {
		if !strings.Contains(evidence, name) {
			t.Errorf("metric %q missing from %s", name, evidence)
		}
	}
	if strings.Contains(evidence, secret) {
		t.Fatalf("metrics leaked request data: %s", evidence)
	}
}

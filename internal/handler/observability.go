package handler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/expfmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	system "github.com/meridian-labs/meridian/internal/generated/api/system"
)

// Observability constants. Names and shapes are aligned with GoFr so dashboards
// and alerts written against GoFr's conventions keep working unchanged.
const (
	// correlationIDHeader is the response header carrying the W3C trace ID of
	// the request, aligned with GoFr's X-Correlation-Id (canonical spelling).
	correlationIDHeader = "X-Correlation-Id"

	// xffHeader is the proxy header consulted to resolve the immediate client
	// IP, following GoFr's getIPAddress (GCLB-style first-hop extraction).
	xffHeader = "X-Forwarded-For"

	// tracerName is the OpenTelemetry instrumentation scope name for Meridian.
	tracerName = "meridian"

	// zeroTraceID and zeroSpanID are the W3C invalid-ID spellings emitted when
	// no span context is in scope, so the trace_id / span_id fields keep a
	// stable placeholder instead of an empty string (aligned with GoFr).
	zeroTraceID = "00000000000000000000000000000000"
	zeroSpanID  = "0000000000000000"

	// httpResponseMetricName mirrors GoFr's app_http_response latency histogram.
	httpResponseMetricName = "app_http_response"

	// Prometheus label keys of the latency histogram.
	metricLabelPath   = "path"
	metricLabelMethod = "method"
	metricLabelStatus = "status"

	// OpenTelemetry HTTP semantic-convention attribute keys (stable, ≥ v1.21).
	// Kept inline rather than importing semconv to avoid an extra dependency.
	spanAttributeMethod     = "http.request.method"
	spanAttributeRoute      = "http.route"
	spanAttributeStatusCode = "http.response.status_code"

	// Trace exporter environment variables, aligned with GoFr's TRACE_EXPORTER
	// and TRACER_URL so an OTLP collector endpoint drops in without a shim.
	envTraceExporter = "TRACE_EXPORTER"
	envTracerURL     = "TRACER_URL"

	// otlpExporter selects the OTLP/gRPC trace exporter.
	otlpExporter = "otlp"

	// defaultOTLPEndpoint is the OTLP/gRPC endpoint used when TRACER_URL is
	// unset, following the standard collector gRPC port.
	defaultOTLPEndpoint = "localhost:4317"
)

// httpLatencyBuckets are the Prometheus histogram boundaries in seconds,
// spanning 1ms through 10s — the typical range for a control-plane request.
var httpLatencyBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// httpResponseHistogram records HTTP latency (seconds) by route template,
// method and status code, aligned with GoFr's app_http_response metric.
var httpResponseHistogram = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    httpResponseMetricName,
		Help:    "HTTP response latency in seconds, labelled by path, method and status.",
		Buckets: httpLatencyBuckets,
	},
	[]string{metricLabelPath, metricLabelMethod, metricLabelStatus},
)

var errHijackNotSupported = errors.New("response writer does not support hijacking")

// StatusResponseWriter captures the HTTP status and body size so middlewares
// can read them after the handler returns. It mirrors GoFr's wrapper and adds
// Flush/Hijack forwarding so streaming (SSE) and upgrades keep working.
type StatusResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *StatusResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Status returns the status as it appears on the wire: net/http emits an
// implicit 200 when a handler writes a body without calling WriteHeader, so a
// zero internal value is normalized to 200 rather than reported as 0.
func (w *StatusResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *StatusResponseWriter) BytesWritten() int { return w.bytes }

func (w *StatusResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush forwards to the underlying writer so streaming responses (SSE) that
// assert http.Flusher keep working through this wrapper.
func (w *StatusResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack forwards to the underlying writer so WebSocket upgrades keep working.
func (w *StatusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errHijackNotSupported
}

// ConfigureTracer installs Meridian's default OpenTelemetry setup, aligned
// with GoFr: W3C TraceContext+Baggage propagation, and an OTLP/gRPC trace
// exporter when TRACE_EXPORTER=otlp with TRACER_URL set (default
// localhost:4317). Spans then carry a valid, unique trace/span ID for
// correlation AND are exported to the collector. With no exporter configured,
// the provider is sampled with NeverSample so IDs stay present but no spans are
// exported and no sampling cost is paid. Call once at startup, before serving.
func ConfigureTracer() error {
	if len(otel.GetTextMapPropagator().Fields()) == 0 {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		))
	}

	exporterName := strings.ToLower(strings.TrimSpace(os.Getenv(envTraceExporter)))
	if exporterName == "" {
		otel.SetTracerProvider(sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
		))
		return nil
	}
	if exporterName != otlpExporter {
		return fmt.Errorf("unsupported %s %q: only %q is implemented", envTraceExporter, exporterName, otlpExporter)
	}

	endpoint := strings.TrimSpace(os.Getenv(envTracerURL))
	if endpoint == "" {
		endpoint = defaultOTLPEndpoint
	}
	exporter, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(endpoint),
	)
	if err != nil {
		return fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "meridian"))),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	return nil
}

// requestRoute returns the matched chi route template (e.g.
// "/api/v1/t/{tenantSlug}/repositories") so logs, metrics and span names stay
// bounded by route count instead of one entry per concrete path. Falls back to
// the raw path for unmatched requests (404 / static assets).
func requestRoute(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" && pattern != "/*" {
			return pattern
		}
	}
	return r.URL.Path
}

// spanIDs resolves the trace and span IDs of the in-scope span, substituting
// the all-zero placeholders when no span is present (noop or unset provider).
func spanIDs(ctx context.Context) (traceID, spanID string) {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return zeroTraceID, zeroSpanID
	}
	return sc.TraceID().String(), sc.SpanID().String()
}

// traceRequests starts a span per request and stamps the response with the
// trace ID, aligned with GoFr's Tracer middleware. It is the outermost of the
// observability middlewares: it always wraps the ResponseWriter so downstream
// logging/metrics middlewares can reuse the same StatusResponseWriter.
func traceRequests(next http.Handler) http.Handler {
	tracer := otel.Tracer(tracerName)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		method := r.Method
		// Route matching happens after middleware in chi, so the template is
		// resolved after the handler returns and the span name/attributes are
		// finalized in the defer below.
		ctx, span := tracer.Start(ctx, method+" "+r.URL.Path)
		defer span.End()

		if traceID, _ := spanIDs(ctx); traceID != zeroTraceID {
			w.Header().Set(correlationIDHeader, traceID)
		}

		srw := &StatusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(srw, r.WithContext(ctx))

		route := requestRoute(r)
		span.SetName(method + " " + route)
		span.SetAttributes(
			attribute.String(spanAttributeMethod, method),
			attribute.String(spanAttributeRoute, route),
			attribute.Int(spanAttributeStatusCode, srw.Status()),
		)
	})
}

// logRequests emits one structured log line per request with the method, route
// template, status, latency, body sizes, trace/span/request IDs and client
// identity — the fields GoFr's Logging middleware records, extended with the
// request body size. 5xx responses are logged at Error level, everything else
// at Info.
func logRequests(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			srw, ok := w.(*StatusResponseWriter)
			if !ok {
				srw = &StatusResponseWriter{ResponseWriter: w}
			}

			next.ServeHTTP(srw, r)

			traceID, spanID := spanIDs(r.Context())
			level := slog.LevelInfo
			if srw.Status() >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			logger.LogAttrs(r.Context(), level, "http_request",
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", requestRoute(r)),
				slog.Int("status", srw.Status()),
				slog.Float64("latency_ms", float64(time.Since(start))/float64(time.Millisecond)),
				slog.Int64("request_bytes", r.ContentLength),
				slog.Int("response_bytes", srw.BytesWritten()),
				slog.String("remote_addr", clientIP(r)),
				slog.String("user_agent", r.UserAgent()),
			)
		})
	}
}

// measureRequests records request latency into the Prometheus histogram,
// reusing the StatusResponseWriter installed by an outer middleware.
func measureRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srw, ok := w.(*StatusResponseWriter)
		if !ok {
			srw = &StatusResponseWriter{ResponseWriter: w}
		}

		start := time.Now()
		next.ServeHTTP(srw, r)

		httpResponseHistogram.WithLabelValues(
			requestRoute(r), r.Method, strconv.Itoa(srw.Status()),
		).Observe(time.Since(start).Seconds())
	})
}

// clientIP resolves the immediate client IP from X-Forwarded-For, falling back
// to the connection's remote address, matching GoFr's getIPAddress.
func clientIP(r *http.Request) string {
	forwarded := r.Header.Get(xffHeader)
	if i := strings.IndexByte(forwarded, ','); i >= 0 {
		forwarded = forwarded[:i]
	}
	if forwarded == "" {
		return r.RemoteAddr
	}
	return strings.TrimSpace(forwarded)
}

// Metrics renders the default Prometheus registry in text exposition format,
// serving the contract's GET /metrics endpoint.
func (s *Server) Metrics(context.Context, system.MetricsRequestObject) (system.MetricsResponseObject, error) {
	body, err := renderMetrics()
	if err != nil {
		return nil, err
	}
	return system.Metrics200TextResponse(body), nil
}

// renderMetrics gathers the default registry and encodes it as Prometheus text.
func renderMetrics() ([]byte, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, family := range families {
		if err := encoder.Encode(family); err != nil {
			return nil, err
		}
	}
	if closer, ok := encoder.(expfmt.Closer); ok {
		_ = closer.Close()
	}
	return buf.Bytes(), nil
}

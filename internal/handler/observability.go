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

// 可观测性常量。名称与形状对齐 GoFr，使针对 GoFr 约定编写的仪表盘与告警无需改动即可继续工作。
const (
	// correlationIDHeader 是承载请求 W3C trace ID 的响应头，对齐 GoFr 的 X-Correlation-Id（规范拼写）。
	correlationIDHeader = "X-Correlation-Id"

	// xffHeader 是用于解析直接客户端 IP 的代理头，遵循 GoFr 的 getIPAddress（GCLB 风格首跳提取）。
	xffHeader = "X-Forwarded-For"

	// tracerName 是 Meridian 的 OpenTelemetry 插桩作用域名称。
	tracerName = "meridian"

	// zeroTraceID 与 zeroSpanID 是 W3C 无效 ID 的拼写，在无 span 上下文时发出，
	// 使 trace_id / span_id 字段保持稳定占位而非空串（对齐 GoFr）。
	zeroTraceID = "00000000000000000000000000000000"
	zeroSpanID  = "0000000000000000"

	// httpResponseMetricName 镜像 GoFr 的 app_http_response 延迟直方图。
	httpResponseMetricName = "app_http_response"

	// 延迟直方图的 Prometheus 标签键。
	metricLabelPath   = "path"
	metricLabelMethod = "method"
	metricLabelStatus = "status"

	// OpenTelemetry HTTP 语义约定属性键（稳定版，≥ v1.21）。
	// 保持内联而非引入 semconv，避免额外依赖。
	spanAttributeMethod     = "http.request.method"
	spanAttributeRoute      = "http.route"
	spanAttributeStatusCode = "http.response.status_code"

	// 追踪导出器环境变量，对齐 GoFr 的 TRACE_EXPORTER 与 TRACER_URL，
	// 使 OTLP 采集器端点无需适配层即可接入。
	envTraceExporter = "TRACE_EXPORTER"
	envTracerURL     = "TRACER_URL"

	// otlpExporter 选择 OTLP/gRPC 追踪导出器。
	otlpExporter = "otlp"

	// defaultOTLPEndpoint 是 TRACER_URL 未设置时使用的 OTLP/gRPC 端点，
	// 遵循标准采集器 gRPC 端口。
	defaultOTLPEndpoint = "localhost:4317"
)

// httpLatencyBuckets 是 Prometheus 直方图以秒为单位的边界，
// 覆盖 1ms 到 10s —— 控制平面请求的典型范围。
var httpLatencyBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// httpResponseHistogram 按路由模板、方法与状态码记录 HTTP 延迟（秒），
// 对齐 GoFr 的 app_http_response 指标。
var httpResponseHistogram = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    httpResponseMetricName,
		Help:    "HTTP response latency in seconds, labelled by path, method and status.",
		Buckets: httpLatencyBuckets,
	},
	[]string{metricLabelPath, metricLabelMethod, metricLabelStatus},
)

// errHijackNotSupported 表示底层 ResponseWriter 不支持连接劫持。
var errHijackNotSupported = errors.New("response writer does not support hijacking")

// StatusResponseWriter 捕获 HTTP 状态码与响应体大小，使中间件可在处理器返回后读取。
// 它镜像 GoFr 的包装器，并追加 Flush/Hijack 转发，使流式（SSE）与协议升级继续可用。
type StatusResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

// WriteHeader 记录首个写入的状态码并透传给底层写入器。
func (w *StatusResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

// Write 记录写入字节数，并在处理器未显式调用 WriteHeader 时隐式记录 200。
func (w *StatusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Status 返回实际出现在线路上的状态码：net/http 在处理器写正文但未调用
// WriteHeader 时会隐式发出 200，因此零内部值被归一化为 200 而非上报为 0。
func (w *StatusResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// BytesWritten 返回已写入的响应体字节数。
func (w *StatusResponseWriter) BytesWritten() int { return w.bytes }

// Unwrap 暴露底层 ResponseWriter。
func (w *StatusResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush 转发到底层写入器，使断言 http.Flusher 的流式响应（SSE）能穿透本包装器继续工作。
func (w *StatusResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack 转发到底层写入器，使 WebSocket 升级继续工作。
func (w *StatusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errHijackNotSupported
}

// ConfigureTracer 安装 Meridian 的默认 OpenTelemetry 配置，对齐 GoFr：
// W3C TraceContext+Baggage 传播；当 TRACE_EXPORTER=otlp 且 TRACER_URL 设置
// （默认 localhost:4317）时启用 OTLP/gRPC 追踪导出器。此后 span 既携带有效且唯一的
// trace/span ID 用于关联，也被导出到采集器。未配置导出器时，provider 以 NeverSample
// 采样，使 ID 仍在但无 span 导出且不付采样成本。启动时调用一次，须在开始服务之前。
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
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", tracerName))),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	return nil
}

// requestRoute 返回匹配到的 chi 路由模板（例如
// "/api/v1/t/{tenantSlug}/repositories"），使日志、指标与 span 名受路由数量约束
// 而非每条具体路径一项。对未匹配请求（404 / 静态资源）回退到原始路径。
func requestRoute(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" && pattern != "/*" {
			return pattern
		}
	}
	return r.URL.Path
}

// spanIDs 解析当前作用域 span 的 trace 与 span ID；当无 span 存在
// （noop 或未设置的 provider）时替换为全零占位。
func spanIDs(ctx context.Context) (traceID, spanID string) {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return zeroTraceID, zeroSpanID
	}
	return sc.TraceID().String(), sc.SpanID().String()
}

// traceRequests 为每个请求开启一个 span 并以 trace ID 加盖响应头，对齐 GoFr 的
// Tracer 中间件。它是可观测性中间件的最外层：始终包装 ResponseWriter，
// 使下游日志/指标中间件可复用同一个 StatusResponseWriter。
func traceRequests(next http.Handler) http.Handler {
	tracer := otel.Tracer(tracerName)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		method := r.Method
		// chi 中路由匹配发生在中间件之后，因此模板在处理器返回后解析，
		// span 名与属性在下方的 defer 中最终确定。
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

// logRequests 为每个请求输出一行结构化日志，包含方法、路由模板、状态码、延迟、
// 请求/响应体大小、trace/span/请求 ID 与客户端身份 —— GoFr Logging 中间件记录的字段，
// 并追加请求体大小。5xx 响应以 Error 级别记录，其余以 Info 记录。
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

// measureRequests 将请求延迟记录到 Prometheus 直方图，
// 复用外层中间件安装的 StatusResponseWriter。
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

// clientIP 从 X-Forwarded-For 解析直接客户端 IP，回退到连接远端地址，
// 匹配 GoFr 的 getIPAddress。
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

// Metrics 以文本展示格式渲染默认 Prometheus 注册表，
// 服务契约的 GET /metrics 端点。
func (s *Server) Metrics(context.Context, system.MetricsRequestObject) (system.MetricsResponseObject, error) {
	body, err := renderMetrics()
	if err != nil {
		return nil, err
	}
	return system.Metrics200TextResponse(body), nil
}

// renderMetrics 收集默认注册表并将其编码为 Prometheus 文本。
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

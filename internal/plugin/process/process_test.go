package process

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"testing"

	"github.com/meridian-labs/meridian/internal/plugin"
)

// helperEnv 标记测试二进制应以插件进程身份运行。
const helperEnv = "MERIDIAN_PLUGIN_HELPER"

// TestHelperProcess 是子进程测试的双面入口：当以插件身份运行时（由测试二进制带
// 特定环境变量执行），它实现一个 echo/fail 契约夹具并通过 Serve 消费 stdio；在普通
// 测试运行中直接跳过。
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process marker not set")
	}
	if err := Serve(context.Background(), func(_ context.Context, call plugin.Call) (plugin.Result, error) {
		switch call.Method {
		case "echo":
			return plugin.Result{ContentType: call.ContentType, Payload: call.Payload}, nil
		case "fail":
			return plugin.Result{}, errors.New("plugin exploded")
		default:
			return plugin.Result{ContentType: "text/plain", Payload: []byte("ok")}, nil
		}
	}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

// newHelperFactory 构造一个指向测试二进制自身的进程工厂，运行 echo/fail 契约夹具。
func newHelperFactory(t *testing.T, capabilities ...plugin.Capability) *Factory {
	t.Helper()
	factory, err := NewFactory(Config{
		Manifest: plugin.Manifest{
			ID: "meridian.test.echo", Version: "1", ProtocolVersion: plugin.CurrentProtocolVersion,
			Capabilities: capabilities,
		},
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestHelperProcess", "--"},
		Env:        append(os.Environ(), helperEnv+"=1"),
	})
	if err != nil {
		t.Fatalf("new factory: %v", err)
	}
	return factory
}

func TestFactoryOpenInvokeAndClose(t *testing.T) {
	t.Parallel()
	factory := newHelperFactory(t, plugin.Capability{ID: "kind/echo", Version: "1"})
	host := plugin.NewHost()
	if err := host.Install(t.Context(), factory); err != nil {
		t.Fatalf("install: %v", err)
	}
	defer host.Close(t.Context())

	payload := []byte{0x00, 0x01, 0xfe, 0xff, 0x80} // 非 UTF-8，验证 base64 往返。
	result, err := host.Invoke(t.Context(), plugin.Call{
		Capability: "kind/echo", Method: "echo", ContentType: "application/octet-stream", Payload: payload,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if string(result.Payload) != string(payload) || result.ContentType != "application/octet-stream" {
		t.Fatalf("unexpected echo result: %#v", result)
	}
}

func TestInvokeSurfacesPluginError(t *testing.T) {
	t.Parallel()
	factory := newHelperFactory(t, plugin.Capability{ID: "kind/echo", Version: "1"})
	host := plugin.NewHost()
	if err := host.Install(t.Context(), factory); err != nil {
		t.Fatalf("install: %v", err)
	}
	defer host.Close(t.Context())

	if _, err := host.Invoke(t.Context(), plugin.Call{Capability: "kind/echo", Method: "fail"}); err == nil {
		t.Fatal("expected plugin error to surface")
	}
}

func TestInvokeCorrelatesConcurrentRequests(t *testing.T) {
	t.Parallel()
	factory := newHelperFactory(t, plugin.Capability{ID: "kind/echo", Version: "1"})
	host := plugin.NewHost()
	if err := host.Install(t.Context(), factory); err != nil {
		t.Fatalf("install: %v", err)
	}
	defer host.Close(t.Context())

	const workers = 16
	errorsCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(worker int) {
			payload := []byte{byte(worker)}
			result, err := host.Invoke(t.Context(), plugin.Call{Capability: "kind/echo", Method: "echo", Payload: payload})
			if err != nil {
				errorsCh <- err
				return
			}
			if string(result.Payload) != string(payload) {
				errorsCh <- errors.New("mismatched correlation")
				return
			}
			errorsCh <- nil
		}(i)
	}
	for range workers {
		if err := <-errorsCh; err != nil {
			t.Fatalf("concurrent invoke: %v", err)
		}
	}
}

func TestServeRunsPluginLoop(t *testing.T) {
	// Serve 读 os.Stdin，这里直接验证其 Handler 逻辑而非进程边界：用替换后的 stdin 会引入
	// 平台差异，故改为对 wireCall 编解码的往返验证，确保插件侧接收到的载荷与主机侧一致。
	call := plugin.Call{Capability: "kind/x", Method: "m", ContentType: "application/json", Payload: []byte("raw bytes")}
	wire := wireCallFrom(call)
	roundTrip := wire.toPlugin()
	if roundTrip.Capability != call.Capability || roundTrip.Method != call.Method || string(roundTrip.Payload) != string(call.Payload) {
		t.Fatalf("wire round trip mismatch: %#v", roundTrip)
	}
	encoded := base64.StdEncoding.EncodeToString(call.Payload)
	if wire.Payload != encoded {
		t.Fatalf("expected base64 payload %q, got %q", encoded, wire.Payload)
	}
}

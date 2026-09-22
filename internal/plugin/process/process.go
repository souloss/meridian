// Package process implements the plugin host contract across a child-process
// boundary over newline-delimited JSON on stdin/stdout.
//
// It is the first concrete transport for the transport-neutral plugin runtime:
// a Factory pairs an operator-owned Manifest with an executable, and the child
// process is a self-contained program (any language) that reads request frames
// on stdin and writes result/log frames on stdout. This closes the loop between
// the byte-envelope Endpoint contract and a genuinely external plugin.
//
// # Wire protocol
//
// Each side exchanges exactly one JSON object per line (NDJSON). Payload bytes
// are base64 (standard encoding) because a JSON string cannot carry arbitrary
// bytes. The "v" field is the protocol version; it must match before any other
// field is trusted.
//
//	Host → plugin   {"v":1,"type":"invoke","id":1,"call":{"capability":"...","method":"...","contentType":"...","payload":"<b64>","metadata":{...}}}
//	Host → plugin   {"v":1,"type":"close"}
//	Plugin → host   {"v":1,"type":"result","id":1,"result":{"contentType":"...","payload":"<b64>","metadata":{...}},"error":""}
//	Plugin → host   {"v":1,"type":"log","line":"..."}
//
// Requests are correlated by "id"; a plugin may handle them concurrently. The
// host serializes frame writes and demultiplexes responses to the waiting
// Invoke call. On "close" (or stdin EOF) the plugin exits.
package process

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	"github.com/meridian-labs/meridian/internal/plugin"
)

// Wire protocol constants. The protocol version is embedded in every frame so
// a version skew is detectable before any payload is interpreted.
const (
	// protocolVersion 是 stdio 线协议的当前版本，写入每个帧的 v 字段。
	protocolVersion = 1
	// frameTypeInvoke/Result/Close/Log 是四个帧类型字符串。
	frameTypeInvoke = "invoke"
	frameTypeResult = "result"
	frameTypeClose  = "close"
	frameTypeLog    = "log"
	// pendingResponseBuffer 是每个在途请求的响应通道容量；缓冲 1 使读循环投递永不阻塞。
	pendingResponseBuffer = 1
	// scannerInitialBuffer 是帧扫描器的初始缓冲；超出后自动扩容至 maxFrameBytes。
	scannerInitialBuffer = 64 * 1024
	// maxFrameBytes 是单个 stdio 帧的最大字节数（base64 后的载荷），防止畸形插件撑爆内存。
	maxFrameBytes = 64 << 20
	// defaultCloseTimeout 是关闭时等待插件进程自行退出的宽限时长，超时后强制杀进程。
	defaultCloseTimeout = 5 * time.Second
)

// errEndpointClosed 报告在一个已经关闭的子进程端点上发起的调用。
var errEndpointClosed = errors.New("plugin process endpoint closed")

// Config configures a child-process plugin Factory.
type Config struct {
	// Manifest is the operator-owned identity and capability description. It is
	// supplied out-of-band (a descriptor file or database row) because the
	// plugin.Factory contract requires Manifest() to return without starting
	// the process.
	Manifest plugin.Manifest
	// Executable is the plugin program to spawn.
	Executable string
	// Args are optional arguments appended to Executable.
	Args []string
	// Env is the full environment for the plugin process. If nil, the parent
	// environment is inherited.
	Env []string
	// CloseTimeout bounds how long Close waits for a graceful exit. Zero uses
	// defaultCloseTimeout.
	CloseTimeout time.Duration
	// Log receives the plugin's stderr and any "log" frames, one line at a time.
	// It may be nil to discard.
	Log func(string)
}

// Factory implements plugin.Factory by spawning the configured executable on
// Open. The manifest is fixed at construction time.
type Factory struct {
	manifest     plugin.Manifest
	executable   string
	args         []string
	env          []string
	closeTimeout time.Duration
	log          func(string)
}

// NewFactory validates and builds a child-process plugin Factory.
func NewFactory(config Config) (*Factory, error) {
	if config.Executable == "" {
		return nil, errors.New("plugin process executable is empty")
	}
	if config.Manifest.ID == "" {
		return nil, errors.New("plugin process manifest has no ID")
	}
	closeTimeout := config.CloseTimeout
	if closeTimeout <= 0 {
		closeTimeout = defaultCloseTimeout
	}
	return &Factory{
		manifest:     cloneManifest(config.Manifest),
		executable:   config.Executable,
		args:         slices.Clone(config.Args),
		env:          slices.Clone(config.Env),
		closeTimeout: closeTimeout,
		log:          config.Log,
	}, nil
}

// Manifest returns the fixed manifest for this plugin.
func (factory *Factory) Manifest() plugin.Manifest { return cloneManifest(factory.manifest) }

// Open spawns the plugin process and returns a request/response endpoint over
// its stdio. The process lifetime is owned by the returned Endpoint, not by ctx.
func (factory *Factory) Open(ctx context.Context) (plugin.Endpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := exec.Command(factory.executable, factory.args...)
	if factory.env != nil {
		command.Env = factory.env
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	command.Stderr = &stderrSplitter{log: factory.log}
	if err := command.Start(); err != nil {
		return nil, err
	}
	endpoint := &Endpoint{
		cmd:          command,
		stdin:        stdin,
		log:          factory.log,
		pending:      make(map[uint64]chan response),
		readerDone:   make(chan struct{}),
		closeTimeout: factory.closeTimeout,
	}
	go endpoint.readLoop(stdout)
	return endpoint, nil
}

// Endpoint is the transport-neutral plugin.Endpoint backed by a child process.
// Invoke may be called concurrently; requests are correlated by an id and
// responses are demultiplexed back to the waiting caller.
type Endpoint struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	log   func(string)

	mu           sync.Mutex
	pending      map[uint64]chan response
	nextID       uint64
	closed       bool
	readerDone   chan struct{}
	closeTimeout time.Duration
}

// response carries either a successful plugin.Result or an error string
// produced by the plugin.
type response struct {
	result  plugin.Result
	errText string
}

// Invoke marshals a request frame, writes it to the plugin's stdin, and blocks
// until the correlated response arrives, ctx is cancelled, or the process dies.
func (endpoint *Endpoint) Invoke(ctx context.Context, call plugin.Call) (plugin.Result, error) {
	endpoint.mu.Lock()
	if endpoint.closed {
		endpoint.mu.Unlock()
		return plugin.Result{}, errEndpointClosed
	}
	id := endpoint.nextID
	endpoint.nextID++
	channel := make(chan response, pendingResponseBuffer)
	endpoint.pending[id] = channel
	frame, err := json.Marshal(requestFrame{V: protocolVersion, Type: frameTypeInvoke, ID: id, Call: wireCallFrom(call)})
	if err != nil {
		delete(endpoint.pending, id)
		endpoint.mu.Unlock()
		return plugin.Result{}, err
	}
	if _, err := endpoint.stdin.Write(append(frame, '\n')); err != nil {
		delete(endpoint.pending, id)
		endpoint.mu.Unlock()
		return plugin.Result{}, err
	}
	endpoint.mu.Unlock()

	select {
	case received := <-channel:
		if received.errText != "" {
			return received.result, errors.New(received.errText)
		}
		return received.result, nil
	case <-ctx.Done():
		endpoint.dropPending(id)
		return plugin.Result{}, ctx.Err()
	}
}

// dropPending removes a request that is no longer waiting. The response may
// still arrive later and is discarded; the buffered channel prevents blocking.
func (endpoint *Endpoint) dropPending(id uint64) {
	endpoint.mu.Lock()
	delete(endpoint.pending, id)
	endpoint.mu.Unlock()
}

// Close sends a close frame, waits for the process to exit within the close
// timeout, and kills it as a last resort. It is idempotent.
func (endpoint *Endpoint) Close(ctx context.Context) error {
	endpoint.mu.Lock()
	if endpoint.closed {
		endpoint.mu.Unlock()
		return nil
	}
	endpoint.closed = true
	closeFrame, _ := json.Marshal(closeFrame{V: protocolVersion, Type: frameTypeClose})
	_, _ = endpoint.stdin.Write(append(closeFrame, '\n'))
	_ = endpoint.stdin.Close()
	endpoint.mu.Unlock()

	timeout := endpoint.closeTimeout
	select {
	case <-endpoint.readerDone:
		return nil
	case <-ctx.Done():
		endpoint.kill()
		return ctx.Err()
	case <-time.After(timeout):
		endpoint.kill()
		return errors.New("plugin process did not exit within close timeout")
	}
}

// kill force-terminates the plugin process. Reaping is owned by readLoop's
// cmd.Wait, which returns as soon as the process dies.
func (endpoint *Endpoint) kill() {
	if endpoint.cmd != nil && endpoint.cmd.Process != nil {
		_ = endpoint.cmd.Process.Kill()
	}
}

// readLoop consumes result/log frames from the plugin stdout, demultiplexes
// them by id, and reaps the process on EOF.
func (endpoint *Endpoint) readLoop(stdout io.Reader) {
	defer close(endpoint.readerDone)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, scannerInitialBuffer), maxFrameBytes)
	for scanner.Scan() {
		var frame responseFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			if endpoint.log != nil {
				endpoint.log("invalid plugin frame: " + err.Error())
			}
			continue
		}
		endpoint.deliver(frame)
	}
	// stdout 关闭意味着进程正在退出：回收进程，并让所有在途请求失败。
	waitErr := endpoint.cmd.Wait()
	if waitErr == nil {
		waitErr = errors.New("plugin process exited")
	}
	endpoint.failPending(waitErr)
}

// deliver routes a decoded frame to the waiting Invoke, or forwards a log line.
func (endpoint *Endpoint) deliver(frame responseFrame) {
	if frame.Type == frameTypeLog {
		if endpoint.log != nil {
			endpoint.log(frame.Line)
		}
		return
	}
	if frame.Type != frameTypeResult {
		return
	}
	endpoint.mu.Lock()
	channel, ok := endpoint.pending[frame.ID]
	if ok {
		delete(endpoint.pending, frame.ID)
	}
	endpoint.mu.Unlock()
	if ok {
		channel <- response{result: frame.Result.toPlugin(), errText: frame.Error}
	}
}

// failPending fails every in-flight request. Callers hold no lock here.
func (endpoint *Endpoint) failPending(err error) {
	endpoint.mu.Lock()
	for id, channel := range endpoint.pending {
		delete(endpoint.pending, id)
		channel <- response{errText: err.Error()}
	}
	endpoint.mu.Unlock()
}

var _ plugin.Factory = (*Factory)(nil)
var _ plugin.Endpoint = (*Endpoint)(nil)

// requestFrame is a host→plugin frame.
type requestFrame struct {
	V    int      `json:"v"`
	Type string   `json:"type"`
	ID   uint64   `json:"id"`
	Call wireCall `json:"call,omitempty"`
}

// responseFrame is a plugin→host frame.
type responseFrame struct {
	V      int        `json:"v"`
	Type   string     `json:"type"`
	ID     uint64     `json:"id"`
	Result wireResult `json:"result,omitempty"`
	Error  string     `json:"error,omitempty"`
	Line   string     `json:"line,omitempty"`
}

// closeFrame is a host→plugin close frame.
type closeFrame struct {
	V    int    `json:"v"`
	Type string `json:"type"`
}

// wireCall is the wire representation of plugin.Call. Payload is base64.
type wireCall struct {
	Capability  string            `json:"capability"`
	Method      string            `json:"method"`
	ContentType string            `json:"contentType,omitempty"`
	Payload     string            `json:"payload,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func wireCallFrom(call plugin.Call) wireCall {
	return wireCall{
		Capability:  call.Capability,
		Method:      call.Method,
		ContentType: call.ContentType,
		Payload:     base64.StdEncoding.EncodeToString(call.Payload),
		Metadata:    call.Metadata,
	}
}

func (call wireCall) toPlugin() plugin.Call {
	payload, _ := base64.StdEncoding.DecodeString(call.Payload)
	return plugin.Call{
		Capability:  call.Capability,
		Method:      call.Method,
		ContentType: call.ContentType,
		Payload:     payload,
		Metadata:    call.Metadata,
	}
}

// wireResult is the wire representation of plugin.Result. Payload is base64.
type wireResult struct {
	ContentType string            `json:"contentType,omitempty"`
	Payload     string            `json:"payload,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func wireResultFrom(result plugin.Result) wireResult {
	return wireResult{
		ContentType: result.ContentType,
		Payload:     base64.StdEncoding.EncodeToString(result.Payload),
		Metadata:    result.Metadata,
	}
}

func (result wireResult) toPlugin() plugin.Result {
	payload, _ := base64.StdEncoding.DecodeString(result.Payload)
	return plugin.Result{ContentType: result.ContentType, Payload: payload, Metadata: result.Metadata}
}

// cloneManifest returns a defensive copy of a manifest.
func cloneManifest(manifest plugin.Manifest) plugin.Manifest {
	manifest.Capabilities = slices.Clone(manifest.Capabilities)
	manifest.Dependencies = slices.Clone(manifest.Dependencies)
	return manifest
}

// Handler is the server-side capability implementation a plugin exposes.
type Handler func(context.Context, plugin.Call) (plugin.Result, error)

// Serve runs the plugin side of the stdio protocol on the current process's
// stdin/stdout until a close frame or EOF is received. It is the minimal loop a
// plugin author reuses to implement an external plugin.
func Serve(ctx context.Context, handler Handler) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, scannerInitialBuffer), maxFrameBytes)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	for scanner.Scan() {
		var frame requestFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return err
		}
		switch frame.Type {
		case frameTypeInvoke:
			result, callErr := handler(ctx, frame.Call.toPlugin())
			response := responseFrame{V: protocolVersion, Type: frameTypeResult, ID: frame.ID}
			if callErr != nil {
				response.Error = callErr.Error()
			} else {
				response.Result = wireResultFrom(result)
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				return err
			}
			if _, err := writer.Write(append(encoded, '\n')); err != nil {
				return err
			}
			if err := writer.Flush(); err != nil {
				return err
			}
		case frameTypeClose:
			return nil
		}
	}
	return scanner.Err()
}

// stderrSplitter forwards the plugin's stderr to a log function line-by-line.
// When log is nil it discards the bytes.
type stderrSplitter struct {
	log func(string)
	buf []byte
}

func (splitter *stderrSplitter) Write(data []byte) (int, error) {
	if splitter.log == nil {
		return len(data), nil
	}
	splitter.buf = append(splitter.buf, data...)
	for {
		index := bytes.IndexByte(splitter.buf, '\n')
		if index < 0 {
			break
		}
		splitter.log(string(splitter.buf[:index]))
		splitter.buf = splitter.buf[index+1:]
	}
	return len(data), nil
}

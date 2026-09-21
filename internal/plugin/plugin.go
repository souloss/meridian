// Package plugin contains the transport-neutral plugin host contract.
//
// A plugin is described by a versioned manifest and exposes one or more
// capabilities through an endpoint. The endpoint only accepts request and
// response envelopes, so an in-process implementation and a future process
// or RPC implementation share the same lifecycle and invocation semantics.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

const CurrentProtocolVersion = "meridian.plugin.v1"

var (
	ErrInvalidManifest        = errors.New("invalid plugin manifest")
	ErrPluginAlreadyInstalled = errors.New("plugin already installed")
	ErrCapabilityAlreadyBound = errors.New("capability already bound")
	ErrDependencyUnavailable  = errors.New("plugin dependency unavailable")
	ErrCapabilityUnavailable  = errors.New("capability unavailable")
	ErrPluginNotInstalled     = errors.New("plugin not installed")
	ErrHostClosed             = errors.New("plugin host closed")
	ErrPluginClosing          = errors.New("plugin is closing")
	ErrInvalidCall            = errors.New("invalid plugin call")
	ErrRegistrationNotOwner   = errors.New("plugin registration is not the owner")
)

// State describes a plugin's lifecycle state in the host.
type State string

const (
	StateActive  State = "active"
	StateClosing State = "closing"
	StateClosed  State = "closed"
)

// EventType identifies a host lifecycle event.
type EventType string

const (
	EventInstalled EventType = "plugin.installed"
	EventClosing   EventType = "plugin.closing"
	EventRemoved   EventType = "plugin.removed"
	EventFailed    EventType = "plugin.failed"
)

// Event is a transport-neutral lifecycle notification. Error is diagnostic
// text only; plugin code must not put credentials or payloads in it.
type Event struct {
	Type     EventType `json:"type"`
	PluginID string    `json:"pluginId"`
	State    State     `json:"state"`
	Manifest Manifest  `json:"manifest"`
	Error    string    `json:"error,omitempty"`
}

// Snapshot is a stable host diagnostic view of an installed plugin.
type Snapshot struct {
	Manifest    Manifest `json:"manifest"`
	State       State    `json:"state"`
	ActiveCalls int      `json:"activeCalls"`
	LastError   string   `json:"lastError,omitempty"`
}

// Capability declares a named service surface provided by a plugin.
type Capability struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// Dependency identifies a plugin required before this plugin can be opened.
// An empty constraint accepts any installed version; otherwise the installed
// version must match exactly. A richer constraint grammar can be added without
// changing the wire envelope.
type Dependency struct {
	PluginID string `json:"pluginId"`
	Version  string `json:"version,omitempty"`
}

// Manifest is the portable identity and dependency description of a plugin.
type Manifest struct {
	ID              string       `json:"id"`
	Version         string       `json:"version"`
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    []Capability `json:"capabilities"`
	Dependencies    []Dependency `json:"dependencies,omitempty"`
}

// Call is the transport-neutral invocation envelope. Payload is owned by the
// caller and should be treated as immutable by implementations.
type Call struct {
	Capability  string            `json:"capability"`
	Method      string            `json:"method"`
	ContentType string            `json:"contentType,omitempty"`
	Payload     []byte            `json:"payload,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Result is the transport-neutral invocation response.
type Result struct {
	ContentType string            `json:"contentType,omitempty"`
	Payload     []byte            `json:"payload,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Endpoint is the lifecycle and invocation boundary implemented by every
// runtime adapter. An RPC runtime can implement it with a client proxy.
type Endpoint interface {
	Invoke(context.Context, Call) (Result, error)
	Close(context.Context) error
}

// Factory creates an endpoint for one plugin manifest. It intentionally does
// not receive a Go service container: host capabilities are represented by
// explicit calls and can therefore be implemented across a process boundary.
type Factory interface {
	Manifest() Manifest
	Open(context.Context) (Endpoint, error)
}

// Registration is an effect/disposer handle for one plugin installation.
// Closing an old handle cannot remove a newer installation with the same ID.
type Registration interface {
	PluginID() string
	Close(context.Context) error
}

// Subscription receives lifecycle events until it is closed or the host is
// closed. Events are delivered best-effort to a bounded buffer.
type Subscription interface {
	Events() <-chan Event
	Close() error
}

// Runtime is the host surface required by the composition root. Host is the
// default implementation; a process-supervisor or RPC-aware host can provide
// the same surface without changing domain capability adapters.
type Runtime interface {
	Install(context.Context, Factory) error
	Invoke(context.Context, Call) (Result, error)
	Endpoint(string) (Endpoint, error)
	Remove(context.Context, string) error
	Close(context.Context) error
}

type Host struct {
	mu             sync.Mutex
	opMu           sync.Mutex
	plugins        map[string]*installed
	capabilities   map[string]string
	order          []string
	closed         bool
	nextGeneration uint64
	nextSubscriber uint64
	subscribers    map[uint64]*subscription
}

type installed struct {
	manifest      Manifest
	endpoint      Endpoint
	generation    uint64
	state         State
	active        int
	drained       chan struct{}
	drainSignaled bool
	lastError     error
}

func NewHost() *Host {
	return &Host{
		plugins:      make(map[string]*installed),
		capabilities: make(map[string]string),
		subscribers:  make(map[uint64]*subscription),
	}
}

// Install opens and registers a plugin. Dependencies and capability conflicts
// are checked before the endpoint becomes visible to callers.
func (host *Host) Install(ctx context.Context, factory Factory) error {
	_, err := host.InstallWithRegistration(ctx, factory)
	return err
}

// InstallWithRegistration opens and registers a plugin, returning a disposer
// that owns exactly this installation.
func (host *Host) InstallWithRegistration(ctx context.Context, factory Factory) (Registration, error) {
	host.opMu.Lock()
	defer host.opMu.Unlock()
	return host.install(ctx, factory)
}

func (host *Host) install(ctx context.Context, factory Factory) (Registration, error) {
	if factory == nil {
		return nil, ErrInvalidManifest
	}
	if ctx == nil {
		ctx = context.Background()
	}
	manifest := factory.Manifest()
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}

	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		return nil, ErrHostClosed
	}
	if _, ok := host.plugins[manifest.ID]; ok {
		host.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrPluginAlreadyInstalled, manifest.ID)
	}
	if err := host.validateDependenciesLocked(manifest); err != nil {
		host.mu.Unlock()
		return nil, err
	}
	if err := host.validateCapabilitiesLocked(manifest); err != nil {
		host.mu.Unlock()
		return nil, err
	}
	host.mu.Unlock()

	endpoint, err := factory.Open(ctx)
	if err != nil {
		host.emitFailure(manifest, err)
		return nil, fmt.Errorf("open plugin %s: %w", manifest.ID, err)
	}
	if endpoint == nil {
		err = ErrInvalidManifest
		host.emitFailure(manifest, err)
		return nil, fmt.Errorf("open plugin %s: %w", manifest.ID, err)
	}

	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		_ = endpoint.Close(context.Background())
		return nil, ErrHostClosed
	}
	if _, ok := host.plugins[manifest.ID]; ok {
		host.mu.Unlock()
		_ = endpoint.Close(context.Background())
		return nil, fmt.Errorf("%w: %s", ErrPluginAlreadyInstalled, manifest.ID)
	}
	if err := host.validateDependenciesLocked(manifest); err != nil {
		host.mu.Unlock()
		_ = endpoint.Close(context.Background())
		return nil, err
	}
	if err := host.validateCapabilitiesLocked(manifest); err != nil {
		host.mu.Unlock()
		_ = endpoint.Close(context.Background())
		return nil, err
	}

	manifest.Capabilities = slices.Clone(manifest.Capabilities)
	manifest.Dependencies = slices.Clone(manifest.Dependencies)
	host.nextGeneration++
	entry := &installed{
		manifest:   manifest,
		endpoint:   endpoint,
		generation: host.nextGeneration,
		state:      StateActive,
		drained:    make(chan struct{}),
	}
	host.signalDrainedLocked(entry)
	host.plugins[manifest.ID] = entry
	host.order = append(host.order, manifest.ID)
	for _, capability := range manifest.Capabilities {
		host.capabilities[capability.ID] = manifest.ID
	}
	host.emitLocked(Event{Type: EventInstalled, PluginID: manifest.ID, State: StateActive, Manifest: manifest})
	host.mu.Unlock()
	return &registration{host: host, pluginID: manifest.ID, generation: entry.generation}, nil
}

func (host *Host) validateDependenciesLocked(manifest Manifest) error {
	for _, dependency := range manifest.Dependencies {
		installedDependency, ok := host.plugins[dependency.PluginID]
		if !ok || installedDependency.state != StateActive || (dependency.Version != "" && installedDependency.manifest.Version != dependency.Version) {
			return fmt.Errorf("%w: %s", ErrDependencyUnavailable, dependency.PluginID)
		}
	}
	return nil
}

func (host *Host) validateCapabilitiesLocked(manifest Manifest) error {
	for _, capability := range manifest.Capabilities {
		if owner, ok := host.capabilities[capability.ID]; ok {
			return fmt.Errorf("%w: %s (owned by %s)", ErrCapabilityAlreadyBound, capability.ID, owner)
		}
	}
	return nil
}

// Invoke routes a request to the plugin that declared the capability.
func (host *Host) Invoke(ctx context.Context, call Call) (Result, error) {
	if strings.TrimSpace(call.Capability) == "" || strings.TrimSpace(call.Method) == "" {
		return Result{}, ErrInvalidCall
	}
	host.mu.Lock()
	entry, err := host.lookupLocked(call.Capability, 0)
	if err != nil {
		host.mu.Unlock()
		return Result{}, err
	}
	entry.active++
	endpoint := entry.endpoint
	host.mu.Unlock()
	defer host.release(entry)
	return endpoint.Invoke(ctx, call)
}

func (host *Host) invokeBound(ctx context.Context, pluginID string, generation uint64, call Call) (Result, error) {
	if strings.TrimSpace(call.Capability) == "" || strings.TrimSpace(call.Method) == "" {
		return Result{}, ErrInvalidCall
	}
	host.mu.Lock()
	entry, err := host.lookupLocked(call.Capability, generation)
	if err == nil && host.capabilities[call.Capability] != pluginID {
		err = fmt.Errorf("%w: %s", ErrCapabilityUnavailable, call.Capability)
	}
	if err != nil {
		host.mu.Unlock()
		return Result{}, err
	}
	entry.active++
	endpoint := entry.endpoint
	host.mu.Unlock()
	defer host.release(entry)
	return endpoint.Invoke(ctx, call)
}

func (host *Host) lookupLocked(capability string, generation uint64) (*installed, error) {
	if host.closed {
		return nil, ErrHostClosed
	}
	pluginID, ok := host.capabilities[capability]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityUnavailable, capability)
	}
	entry, ok := host.plugins[pluginID]
	if !ok || entry.state != StateActive || (generation != 0 && entry.generation != generation) {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityUnavailable, capability)
	}
	return entry, nil
}

func (host *Host) release(entry *installed) {
	host.mu.Lock()
	entry.active--
	if entry.active == 0 && entry.state == StateClosing {
		host.signalDrainedLocked(entry)
	}
	host.mu.Unlock()
}

// Endpoint returns a host-owned routed endpoint for a declared capability.
// The returned endpoint remains safe to retain: removal or host shutdown makes
// later calls fail instead of exposing a closed in-process object.
func (host *Host) Endpoint(capability string) (Endpoint, error) {
	if strings.TrimSpace(capability) == "" {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityUnavailable, capability)
	}
	host.mu.Lock()
	entry, err := host.lookupLocked(capability, 0)
	if err != nil {
		host.mu.Unlock()
		return nil, err
	}
	routed := routedEndpoint{host: host, pluginID: host.capabilities[capability], generation: entry.generation, capability: capability}
	host.mu.Unlock()
	return routed, nil
}

// Remove closes and unregisters a plugin. If the context expires while calls
// are active, a normal removal is rolled back and can be retried later.
func (host *Host) Remove(ctx context.Context, pluginID string) error {
	host.opMu.Lock()
	defer host.opMu.Unlock()
	return host.remove(ctx, pluginID, 0)
}

// Close shuts down plugins in reverse installation order. All plugins are
// attempted and errors are joined for callers that need complete diagnostics.
func (host *Host) Close(ctx context.Context) error {
	host.opMu.Lock()
	defer host.opMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	host.mu.Lock()
	host.closed = true
	order := slices.Clone(host.order)
	host.mu.Unlock()

	var closeErr error
	for i := len(order) - 1; i >= 0; i-- {
		if err := host.remove(ctx, order[i], 0); err != nil && !errors.Is(err, ErrPluginNotInstalled) {
			closeErr = errors.Join(closeErr, err)
		}
	}
	host.mu.Lock()
	host.closeSubscriptionsLocked()
	host.mu.Unlock()
	return closeErr
}

func (host *Host) Manifests() []Manifest {
	host.mu.Lock()
	manifests := make([]Manifest, 0, len(host.plugins))
	for _, entry := range host.plugins {
		manifests = append(manifests, cloneManifest(entry.manifest))
	}
	host.mu.Unlock()
	slices.SortFunc(manifests, func(left, right Manifest) int { return stringsCompare(left.ID, right.ID) })
	return manifests
}

// Snapshots returns deterministic lifecycle diagnostics for all installed
// plugins, including plugins currently draining during removal.
func (host *Host) Snapshots() []Snapshot {
	host.mu.Lock()
	snapshots := make([]Snapshot, 0, len(host.plugins))
	for _, entry := range host.plugins {
		snapshot := Snapshot{Manifest: cloneManifest(entry.manifest), State: entry.state, ActiveCalls: entry.active}
		if entry.lastError != nil {
			snapshot.LastError = entry.lastError.Error()
		}
		snapshots = append(snapshots, snapshot)
	}
	host.mu.Unlock()
	slices.SortFunc(snapshots, func(left, right Snapshot) int { return stringsCompare(left.Manifest.ID, right.Manifest.ID) })
	return snapshots
}

// Subscribe creates a bounded lifecycle event stream.
func (host *Host) Subscribe(buffer int) (Subscription, error) {
	if buffer < 1 {
		buffer = 16
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.closed {
		return nil, ErrHostClosed
	}
	host.nextSubscriber++
	subscriber := &subscription{host: host, id: host.nextSubscriber, ch: make(chan Event, buffer)}
	subscriber.closeOnce = sync.OnceFunc(subscriber.close)
	host.subscribers[subscriber.id] = subscriber
	return subscriber, nil
}

func (host *Host) remove(ctx context.Context, pluginID string, generation uint64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	host.mu.Lock()
	entry, ok := host.plugins[pluginID]
	if !ok {
		host.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotInstalled, pluginID)
	}
	if generation != 0 && entry.generation != generation {
		host.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrRegistrationNotOwner, pluginID)
	}
	if entry.state == StateClosing && !host.closed && entry.lastError == nil {
		host.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginClosing, pluginID)
	}
	wasActive := entry.state == StateActive
	orderIndex := slices.Index(host.order, pluginID)
	if wasActive {
		entry.state = StateClosing
		for _, capability := range entry.manifest.Capabilities {
			if host.capabilities[capability.ID] == pluginID {
				delete(host.capabilities, capability.ID)
			}
		}
		if !host.closed {
			host.order = slices.DeleteFunc(host.order, func(id string) bool { return id == pluginID })
		}
		host.emitLocked(Event{Type: EventClosing, PluginID: pluginID, State: StateClosing, Manifest: entry.manifest})
	}
	entry.lastError = nil
	if entry.active == 0 {
		host.signalDrainedLocked(entry)
	}
	drained := entry.drained
	active := entry.active
	host.mu.Unlock()

	if active > 0 {
		select {
		case <-drained:
		case <-ctx.Done():
			host.mu.Lock()
			if !host.closed && entry.state == StateClosing {
				entry.state = StateActive
				entry.drained = make(chan struct{})
				entry.drainSignaled = false
				if entry.active == 0 {
					host.signalDrainedLocked(entry)
				}
				for _, capability := range entry.manifest.Capabilities {
					host.capabilities[capability.ID] = pluginID
				}
				if orderIndex < 0 || orderIndex > len(host.order) {
					orderIndex = len(host.order)
				}
				host.order = slices.Insert(host.order, orderIndex, pluginID)
				host.emitLocked(Event{Type: EventInstalled, PluginID: pluginID, State: StateActive, Manifest: entry.manifest})
			}
			host.mu.Unlock()
			return ctx.Err()
		}
	}

	closeErr := entry.endpoint.Close(ctx)
	host.mu.Lock()
	if closeErr != nil {
		entry.state = StateClosing
		entry.lastError = closeErr
		if !slices.Contains(host.order, pluginID) {
			if orderIndex < 0 || orderIndex > len(host.order) {
				orderIndex = len(host.order)
			}
			host.order = slices.Insert(host.order, orderIndex, pluginID)
		}
		host.emitLocked(Event{Type: EventFailed, PluginID: pluginID, State: StateClosing, Manifest: entry.manifest, Error: errorText(closeErr)})
		host.mu.Unlock()
		return closeErr
	}
	entry.state = StateClosed
	entry.lastError = nil
	delete(host.plugins, pluginID)
	host.order = slices.DeleteFunc(host.order, func(id string) bool { return id == pluginID })
	host.emitLocked(Event{Type: EventRemoved, PluginID: pluginID, State: StateClosed, Manifest: entry.manifest, Error: errorText(closeErr)})
	host.mu.Unlock()
	return closeErr
}

func (host *Host) signalDrainedLocked(entry *installed) {
	if entry.state != StateClosing || entry.drainSignaled {
		return
	}
	close(entry.drained)
	entry.drainSignaled = true
}

func (host *Host) emitFailure(manifest Manifest, err error) {
	host.mu.Lock()
	if !host.closed {
		host.emitLocked(Event{Type: EventFailed, PluginID: manifest.ID, State: StateClosed, Manifest: manifest, Error: errorText(err)})
	}
	host.mu.Unlock()
}

func (host *Host) emitLocked(event Event) {
	event.Manifest = cloneManifest(event.Manifest)
	for _, subscriber := range host.subscribers {
		select {
		case subscriber.ch <- event:
		default:
		}
	}
}

func (host *Host) closeSubscriptionsLocked() {
	for id, subscriber := range host.subscribers {
		delete(host.subscribers, id)
		close(subscriber.ch)
	}
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.Capabilities = slices.Clone(manifest.Capabilities)
	manifest.Dependencies = slices.Clone(manifest.Dependencies)
	return manifest
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type registration struct {
	mu         sync.Mutex
	host       *Host
	pluginID   string
	generation uint64
	closed     bool
}

func (registration *registration) PluginID() string { return registration.pluginID }

func (registration *registration) Close(ctx context.Context) error {
	registration.mu.Lock()
	defer registration.mu.Unlock()
	if registration.closed {
		return nil
	}
	registration.host.opMu.Lock()
	defer registration.host.opMu.Unlock()
	err := registration.host.remove(ctx, registration.pluginID, registration.generation)
	if err == nil || errors.Is(err, ErrPluginNotInstalled) {
		registration.closed = true
	}
	return err
}

type subscription struct {
	host      *Host
	id        uint64
	ch        chan Event
	closeOnce func()
}

func (subscription *subscription) Events() <-chan Event { return subscription.ch }

func (subscription *subscription) Close() error {
	subscription.closeOnce()
	return nil
}

func (subscription *subscription) close() {
	subscription.host.mu.Lock()
	if current, ok := subscription.host.subscribers[subscription.id]; ok && current == subscription {
		delete(subscription.host.subscribers, subscription.id)
		close(subscription.ch)
	}
	subscription.host.mu.Unlock()
}

type routedEndpoint struct {
	host       *Host
	pluginID   string
	generation uint64
	capability string
}

func (endpoint routedEndpoint) Invoke(ctx context.Context, call Call) (Result, error) {
	if call.Capability != endpoint.capability {
		return Result{}, fmt.Errorf("%w: %s", ErrCapabilityUnavailable, call.Capability)
	}
	return endpoint.host.invokeBound(ctx, endpoint.pluginID, endpoint.generation, call)
}

// Close is a no-op because the host owns the underlying endpoint lifecycle.
func (routedEndpoint) Close(context.Context) error { return nil }

func validateManifest(manifest Manifest) error {
	if strings.TrimSpace(manifest.ID) == "" || strings.TrimSpace(manifest.Version) == "" || manifest.ProtocolVersion != CurrentProtocolVersion || len(manifest.Capabilities) == 0 {
		return ErrInvalidManifest
	}
	seenCapabilities := make(map[string]struct{}, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		if strings.TrimSpace(capability.ID) == "" || strings.TrimSpace(capability.Version) == "" {
			return ErrInvalidManifest
		}
		if _, ok := seenCapabilities[capability.ID]; ok {
			return ErrInvalidManifest
		}
		seenCapabilities[capability.ID] = struct{}{}
	}
	seenDependencies := make(map[string]struct{}, len(manifest.Dependencies))
	for _, dependency := range manifest.Dependencies {
		if strings.TrimSpace(dependency.PluginID) == "" || dependency.PluginID == manifest.ID {
			return ErrInvalidManifest
		}
		if _, ok := seenDependencies[dependency.PluginID]; ok {
			return ErrInvalidManifest
		}
		seenDependencies[dependency.PluginID] = struct{}{}
	}
	return nil
}

func stringsCompare(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

var _ Runtime = (*Host)(nil)

package plugin

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type testFactory struct {
	manifest Manifest
	endpoint Endpoint
}

func (factory testFactory) Manifest() Manifest { return factory.manifest }

func (factory testFactory) Open(context.Context) (Endpoint, error) { return factory.endpoint, nil }

type testEndpoint struct {
	invoked int
	closed  int
}

func (endpoint *testEndpoint) Invoke(context.Context, Call) (Result, error) {
	endpoint.invoked++
	return Result{ContentType: "text/plain", Payload: []byte("ok")}, nil
}

func (endpoint *testEndpoint) Close(context.Context) error {
	endpoint.closed++
	return nil
}

func validManifest(id string, dependencies ...Dependency) Manifest {
	return Manifest{
		ID: id, Version: "1", ProtocolVersion: CurrentProtocolVersion,
		Capabilities: []Capability{{ID: "cap/" + id, Version: "1"}}, Dependencies: dependencies,
	}
}

func TestHostInstallInvokeAndClose(t *testing.T) {
	t.Parallel()
	host := NewHost()
	firstEndpoint := &testEndpoint{}
	secondEndpoint := &testEndpoint{}
	if err := host.Install(t.Context(), testFactory{manifest: validManifest("first"), endpoint: firstEndpoint}); err != nil {
		t.Fatal(err)
	}
	if err := host.Install(t.Context(), testFactory{manifest: validManifest("second", Dependency{PluginID: "first", Version: "1"}), endpoint: secondEndpoint}); err != nil {
		t.Fatal(err)
	}
	result, err := host.Invoke(t.Context(), Call{Capability: "cap/second", Method: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Payload) != "ok" || secondEndpoint.invoked != 1 {
		t.Fatalf("unexpected invocation result: %#v, calls=%d", result, secondEndpoint.invoked)
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if firstEndpoint.closed != 1 || secondEndpoint.closed != 1 {
		t.Fatalf("expected both endpoints to close: first=%d second=%d", firstEndpoint.closed, secondEndpoint.closed)
	}
}

func TestHostRejectsUnavailableDependencyAndCapabilityConflict(t *testing.T) {
	t.Parallel()
	host := NewHost()
	missing := testFactory{manifest: validManifest("dependent", Dependency{PluginID: "missing"}), endpoint: &testEndpoint{}}
	if err := host.Install(t.Context(), missing); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("expected dependency error, got %v", err)
	}
	if err := host.Install(t.Context(), testFactory{manifest: validManifest("first"), endpoint: &testEndpoint{}}); err != nil {
		t.Fatal(err)
	}
	conflict := validManifest("second")
	conflict.Capabilities[0].ID = "cap/first"
	if err := host.Install(t.Context(), testFactory{manifest: conflict, endpoint: &testEndpoint{}}); !errors.Is(err, ErrCapabilityAlreadyBound) {
		t.Fatalf("expected capability conflict, got %v", err)
	}
}

func TestHostManifestsAreDeterministic(t *testing.T) {
	t.Parallel()
	host := NewHost()
	for _, id := range []string{"zeta", "alpha", "middle"} {
		if err := host.Install(t.Context(), testFactory{manifest: validManifest(id), endpoint: &testEndpoint{}}); err != nil {
			t.Fatal(err)
		}
	}
	manifests := host.Manifests()
	ids := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		ids = append(ids, manifest.ID)
	}
	if !slices.Equal(ids, []string{"alpha", "middle", "zeta"}) {
		t.Fatalf("unexpected manifest order: %v", ids)
	}
}

func TestEndpointRetainsHostLifecycleAndGeneration(t *testing.T) {
	t.Parallel()
	host := NewHost()
	first := &testEndpoint{}
	manifest := validManifest("replaceable")
	if err := host.Install(t.Context(), testFactory{manifest: manifest, endpoint: first}); err != nil {
		t.Fatal(err)
	}
	endpoint, err := host.Endpoint("cap/replaceable")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Remove(t.Context(), manifest.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint.Invoke(t.Context(), Call{Capability: "cap/replaceable", Method: "ping"}); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("expected removed capability to fail through retained endpoint, got %v", err)
	}
	second := &testEndpoint{}
	if err := host.Install(t.Context(), testFactory{manifest: manifest, endpoint: second}); err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint.Invoke(t.Context(), Call{Capability: "cap/replaceable", Method: "ping"}); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("expected old endpoint generation to remain invalid, got %v", err)
	}
	newEndpoint, err := host.Endpoint("cap/replaceable")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newEndpoint.Invoke(t.Context(), Call{Capability: "cap/replaceable", Method: "ping"}); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationOnlyClosesItsInstallation(t *testing.T) {
	t.Parallel()
	host := NewHost()
	manifest := validManifest("owned")
	firstRegistration, err := host.InstallWithRegistration(t.Context(), testFactory{manifest: manifest, endpoint: &testEndpoint{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Remove(t.Context(), manifest.ID); err != nil {
		t.Fatal(err)
	}
	secondEndpoint := &testEndpoint{}
	secondRegistration, err := host.InstallWithRegistration(t.Context(), testFactory{manifest: manifest, endpoint: secondEndpoint})
	if err != nil {
		t.Fatal(err)
	}
	if err := firstRegistration.Close(t.Context()); !errors.Is(err, ErrRegistrationNotOwner) {
		t.Fatalf("expected stale registration ownership error, got %v", err)
	}
	if _, err := host.Invoke(t.Context(), Call{Capability: "cap/owned", Method: "ping"}); err != nil {
		t.Fatal(err)
	}
	if err := secondRegistration.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

type blockingEndpoint struct {
	started    chan struct{}
	release    chan struct{}
	startOnce  sync.Once
	closeCalls int
}

type retryCloseEndpoint struct {
	closeCalls int
}

func (endpoint *retryCloseEndpoint) Invoke(context.Context, Call) (Result, error) {
	return Result{Payload: []byte("ok")}, nil
}

func (endpoint *retryCloseEndpoint) Close(context.Context) error {
	endpoint.closeCalls++
	if endpoint.closeCalls == 1 {
		return errors.New("temporary close failure")
	}
	return nil
}

func newBlockingEndpoint() *blockingEndpoint {
	return &blockingEndpoint{started: make(chan struct{}), release: make(chan struct{})}
}

func (endpoint *blockingEndpoint) Invoke(ctx context.Context, _ Call) (Result, error) {
	endpoint.startOnce.Do(func() { close(endpoint.started) })
	select {
	case <-endpoint.release:
		return Result{Payload: []byte("released")}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (endpoint *blockingEndpoint) Close(context.Context) error {
	endpoint.closeCalls++
	return nil
}

func TestRemoveDrainsActiveCallsAndRollsBackOnTimeout(t *testing.T) {
	host := NewHost()
	endpoint := newBlockingEndpoint()
	manifest := validManifest("draining")
	if err := host.Install(t.Context(), testFactory{manifest: manifest, endpoint: endpoint}); err != nil {
		t.Fatal(err)
	}
	callDone := make(chan error, 1)
	go func() {
		_, err := host.Invoke(t.Context(), Call{Capability: "cap/draining", Method: "wait"})
		callDone <- err
	}()
	<-endpoint.started
	removeContext, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := host.Remove(removeContext, manifest.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected remove timeout, got %v", err)
	}
	snapshots := host.Snapshots()
	if len(snapshots) != 1 || snapshots[0].State != StateActive || snapshots[0].ActiveCalls != 1 {
		t.Fatalf("expected active plugin after rollback, got %#v", snapshots)
	}
	close(endpoint.release)
	if err := <-callDone; err != nil {
		t.Fatalf("active call failed after release: %v", err)
	}
	if err := host.Remove(t.Context(), manifest.ID); err != nil {
		t.Fatal(err)
	}
	if endpoint.closeCalls != 1 {
		t.Fatalf("expected endpoint to close once, got %d", endpoint.closeCalls)
	}
}

func TestHostLifecycleEventsAndClosedState(t *testing.T) {
	t.Parallel()
	host := NewHost()
	subscription, err := host.Subscribe(8)
	if err != nil {
		t.Fatal(err)
	}
	manifest := validManifest("events")
	if err := host.Install(t.Context(), testFactory{manifest: manifest, endpoint: &testEndpoint{}}); err != nil {
		t.Fatal(err)
	}
	if err := host.Remove(t.Context(), manifest.ID); err != nil {
		t.Fatal(err)
	}
	events := make([]EventType, 0, 3)
	for range 3 {
		event := <-subscription.Events()
		events = append(events, event.Type)
	}
	if !slices.Equal(events, []EventType{EventInstalled, EventClosing, EventRemoved}) {
		t.Fatalf("unexpected lifecycle events: %v", events)
	}
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Invoke(t.Context(), Call{Capability: "cap/events", Method: "ping"}); !errors.Is(err, ErrHostClosed) {
		t.Fatalf("expected closed host error, got %v", err)
	}
	if _, err := host.Subscribe(1); !errors.Is(err, ErrHostClosed) {
		t.Fatalf("expected closed subscription error, got %v", err)
	}
}

func TestHostCloseClosesSubscriptions(t *testing.T) {
	t.Parallel()
	host := NewHost()
	subscription, err := host.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-subscription.Events():
		if ok {
			t.Fatal("expected subscription channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription channel did not close")
	}
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHostCloseCanRetryAfterDrainTimeout(t *testing.T) {
	host := NewHost()
	endpoint := newBlockingEndpoint()
	manifest := validManifest("close-retry")
	if err := host.Install(t.Context(), testFactory{manifest: manifest, endpoint: endpoint}); err != nil {
		t.Fatal(err)
	}
	callDone := make(chan error, 1)
	go func() {
		_, err := host.Invoke(t.Context(), Call{Capability: "cap/close-retry", Method: "wait"})
		callDone <- err
	}()
	<-endpoint.started
	closeContext, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := host.Close(closeContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected close timeout, got %v", err)
	}
	if snapshots := host.Snapshots(); len(snapshots) != 1 || snapshots[0].State != StateClosing {
		t.Fatalf("expected closing plugin after timeout, got %#v", snapshots)
	}
	close(endpoint.release)
	if err := <-callDone; err != nil {
		t.Fatalf("active call failed after release: %v", err)
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(host.Manifests()) != 0 || endpoint.closeCalls != 1 {
		t.Fatalf("host retry did not finish cleanup: manifests=%#v closes=%d", host.Manifests(), endpoint.closeCalls)
	}
}

func TestRemoveRetainsPluginWhenEndpointCloseFails(t *testing.T) {
	t.Parallel()
	host := NewHost()
	endpoint := &retryCloseEndpoint{}
	manifest := validManifest("close-failure")
	if err := host.Install(t.Context(), testFactory{manifest: manifest, endpoint: endpoint}); err != nil {
		t.Fatal(err)
	}
	if err := host.Remove(t.Context(), manifest.ID); err == nil {
		t.Fatal("expected endpoint close failure")
	}
	snapshots := host.Snapshots()
	if len(snapshots) != 1 || snapshots[0].State != StateClosing || snapshots[0].LastError == "" {
		t.Fatalf("expected retained close failure snapshot, got %#v", snapshots)
	}
	if _, err := host.Invoke(t.Context(), Call{Capability: "cap/close-failure", Method: "ping"}); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("expected capability to remain withdrawn, got %v", err)
	}
	if err := host.Remove(t.Context(), manifest.ID); err != nil {
		t.Fatal(err)
	}
	if len(host.Manifests()) != 0 || endpoint.closeCalls != 2 {
		t.Fatalf("expected close retry to finish cleanup: manifests=%#v closes=%d", host.Manifests(), endpoint.closeCalls)
	}
}

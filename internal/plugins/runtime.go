// Package plugins composes the core plugin runtime used by the Meridian
// process. The registries expose domain adapters while Host owns the
// transport-neutral endpoint lifecycle.
package plugins

import (
	"context"
	"fmt"

	"github.com/meridian-labs/meridian/internal/ai"
	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/kinds/dbschema"
	"github.com/meridian-labs/meridian/internal/kinds/dependency"
	"github.com/meridian-labs/meridian/internal/kinds/openapi"
	"github.com/meridian-labs/meridian/internal/plugin"
)

type Runtime struct {
	Host  *plugin.Host
	Kinds *kinds.Registry
	AI    *ai.Registry
}

// KindFactory is the composition boundary for a Kind implementation. It is
// intentionally based on plugin.Factory so a process/RPC factory can be
// registered exactly like a built-in implementation.
type KindFactory interface {
	plugin.Factory
	Descriptor() kinds.Descriptor
}

// New installs the built-in core capabilities in one process. The endpoint
// registration deliberately goes through plugin.Host so a future process/RPC
// factory can populate the same domain registry without changing callers.
func New(ctx context.Context) (*Runtime, error) {
	host := plugin.NewHost()
	kindRegistry := kinds.NewRegistry()
	runtime := &Runtime{Host: host, Kinds: kindRegistry, AI: ai.NewRegistry()}
	for _, kindPlugin := range []KindFactory{openapi.NewPlugin(), dbschema.NewPlugin(), dependency.NewPlugin()} {
		if err := runtime.RegisterKind(ctx, kindPlugin); err != nil {
			_ = host.Close(context.Background())
			return nil, fmt.Errorf("install builtin kind plugin: %w", err)
		}
	}
	return runtime, nil
}

// RegisterKind installs a Kind factory in the common Host and binds its
// endpoint to the Kind registry. The factory may be in-process or an RPC
// proxy; callers only depend on the descriptor and endpoint contract.
func (runtime *Runtime) RegisterKind(ctx context.Context, factory KindFactory) error {
	if runtime == nil || runtime.Host == nil || runtime.Kinds == nil || factory == nil {
		return fmt.Errorf("plugin runtime is not initialized")
	}
	if err := runtime.Host.Install(ctx, factory); err != nil {
		return err
	}
	manifest := factory.Manifest()
	endpoint, err := runtime.Host.Endpoint("kind/" + factory.Descriptor().Kind)
	if err != nil {
		_ = runtime.Host.Remove(context.Background(), manifest.ID)
		return err
	}
	if err := runtime.Kinds.RegisterEndpoint(factory.Descriptor(), endpoint); err != nil {
		_ = runtime.Host.Remove(context.Background(), manifest.ID)
		return err
	}
	return nil
}

func (runtime *Runtime) Close(ctx context.Context) error {
	if runtime == nil || runtime.Host == nil {
		return nil
	}
	return runtime.Host.Close(ctx)
}

// RegisterAIProvider installs an AI provider into the common Host and exposes
// its endpoint through the AI registry. The workflow therefore uses the same
// byte-envelope route for built-in and future external providers.
func (runtime *Runtime) RegisterAIProvider(ctx context.Context, provider ai.Provider) error {
	if runtime == nil || runtime.Host == nil || runtime.AI == nil {
		return fmt.Errorf("plugin runtime is not initialized")
	}
	adapter, err := ai.NewPlugin(provider)
	if err != nil {
		return err
	}
	if err := runtime.Host.Install(ctx, adapter); err != nil {
		return err
	}
	descriptor := provider.Descriptor()
	endpoint, err := runtime.Host.Endpoint("ai/" + descriptor.ID)
	if err != nil {
		_ = runtime.Host.Remove(context.Background(), adapter.Manifest().ID)
		return err
	}
	proxy, err := ai.NewEndpointProvider(descriptor, endpoint)
	if err != nil {
		_ = runtime.Host.Remove(context.Background(), adapter.Manifest().ID)
		return err
	}
	return runtime.AI.Replace(proxy)
}

// RegisterAIEndpoint binds a pre-built AI descriptor to any plugin Factory.
// It is the external-runtime counterpart of RegisterAIProvider.
func (runtime *Runtime) RegisterAIEndpoint(ctx context.Context, factory plugin.Factory, descriptor ai.Descriptor) error {
	if runtime == nil || runtime.Host == nil || runtime.AI == nil || factory == nil {
		return fmt.Errorf("plugin runtime is not initialized")
	}
	if err := runtime.Host.Install(ctx, factory); err != nil {
		return err
	}
	manifest := factory.Manifest()
	endpoint, err := runtime.Host.Endpoint("ai/" + descriptor.ID)
	if err != nil {
		_ = runtime.Host.Remove(context.Background(), manifest.ID)
		return err
	}
	proxy, err := ai.NewEndpointProvider(descriptor, endpoint)
	if err != nil {
		_ = runtime.Host.Remove(context.Background(), manifest.ID)
		return err
	}
	return runtime.AI.Replace(proxy)
}

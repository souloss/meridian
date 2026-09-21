// Package builtin registers the Kind implementations shipped with Meridian.
package builtin

import (
	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/kinds/dbschema"
	"github.com/meridian-labs/meridian/internal/kinds/dependency"
	"github.com/meridian-labs/meridian/internal/kinds/openapi"
)

func NewRegistry() *kinds.Registry {
	registry := kinds.NewRegistry()
	_ = registry.Register(openapi.NewPlugin())
	_ = registry.Register(dbschema.NewPlugin())
	_ = registry.Register(dependency.NewPlugin())
	return registry
}

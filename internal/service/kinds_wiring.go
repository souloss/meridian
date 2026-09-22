package service

import (
	"errors"

	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/plugin"
)

// errKindRegistryNotWired 表示服务未注入 kind registry（插件主机未装配）。这是装配错误，
// 而非运行时数据错误：所有 kind 能力都必须经 plugin.Host 提供的 endpoint 访问。
var errKindRegistryNotWired = errors.New("kind registry is not wired to plugin host")

// lookupKindEndpoint 从注入的 kind registry 解析能力端点。registry 未注入时返回明确的
// 装配错误，避免空指针崩溃掩盖"服务绕过了插件主机"的装配缺陷。
func lookupKindEndpoint(registry *kinds.Registry, kind string) (kinds.Descriptor, plugin.Endpoint, error) {
	if registry == nil {
		return kinds.Descriptor{}, nil, errKindRegistryNotWired
	}
	return registry.LookupEndpoint(kind)
}

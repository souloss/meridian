package service

// 本文件集中放置资产与内容处理域的领域常量：通用 kind、角色、引用类型、健康状态、条目类型。
// 值来源：contracts/domain.yaml 与 kinds.yaml 的 itemSchemas。

// 通用资产 kind 标识（kinds.yaml 中注册的 kind 标识）。
const (
	// assetKindOpenapi 是 OpenAPI 文档类资产。
	assetKindOpenapi = "openapi"
)

// 引用类型（refType：branch/tag，决定 track 维度）。
const (
	// refTypeBranch 表示分支引用。
	refTypeBranch = "branch"
	// refTypeTag 表示标签引用。
	refTypeTag = "tag"
)

// 资产健康状态（assetHealth 枚举）。
const (
	// assetHealthOK 表示资产存在有效的最新版本且所需源均健康。
	assetHealthOK = "ok"
	// assetHealthStale 表示任一必需源处于过期状态。
	assetHealthStale = "stale"
	// assetHealthInvalid 表示无有效版本且最新规范化失败。
	assetHealthInvalid = "invalid"
)

// 源绑定状态（sourceBinding.state 枚举）。
const (
	// sourceBindingStateActive 表示绑定处于活跃状态。
	sourceBindingStateActive = "active"
)

// 注意：层作用域类型 scopeTypeRef（"ref"）/scopeTypeGlobal（"global"）已在
// layer_constants.go 中定义为 overlayScopeRef/overlayScopeGlobal，复用即可。

// 条目类型（itemType，kinds.yaml itemKey/itemSchemas 的 itemType 判别值）。
const (
	// itemTypeOperation 是 openapi kind 的操作条目类型。
	itemTypeOperation = "operation"
)

// 生产者依赖状态（dependencyStatus）。
const (
	// dependencyStatusAvailable 表示生产者依赖可用。
	dependencyStatusAvailable = "available"
	// dependencyStatusUnavailable 表示生产者依赖缺失或不可用。
	dependencyStatusUnavailable = "unavailable"
)

// 生产者网络策略（producer network）。
const (
	// producerNetworkNone 表示生产者进程禁止网络访问。
	producerNetworkNone = "none"
	// producerNetworkInherit 表示生产者进程继承宿主网络。
	producerNetworkInherit = "inherit"
)

// 仓库 known_hosts 信任策略（fetchConfig.knownHostPolicy）。
const (
	// knownHostPolicyStrict 表示仅接受已批准的已知主机密钥。
	knownHostPolicyStrict = "strict"
	// knownHostPolicyAcceptNew 表示自动接受并记录新的主机密钥。
	knownHostPolicyAcceptNew = "accept_new"
)

// 注意：服务生命周期状态（draft/published/deprecated/retired）已分别在
// ai_constants.go（lifecycleDraft/lifecyclePublished）与 service_lifecycle.go
// （serviceLifecycleDraft/Published/Deprecated/Retired）定义，复用即可。

// openapi 特有错误码（source last_error / producer 失败分类）。
const (
	// sourceErrorAssetPathNotFound 表示内置源配置的路径未匹配到任何文件。
	sourceErrorAssetPathNotFound = "asset_path_not_found"
	// sourceErrorProducerUnavailable 表示所选生产者配置不可用。
	sourceErrorProducerUnavailable = "producer_profile_unavailable"
)

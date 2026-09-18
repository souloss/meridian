package service

// 本文件集中放置 M4 系统分组、依赖图与跨 kind 搜索域的领域常量。
// 值来源：contracts/domain.yaml 的 subscriptionScopeType / kinds.yaml 的 itemSchemas。
// DB 列值常量见对应 migration 注释；技术维度常量不在此列。

const (
	// kindDbschema 是数据库结构资产 kind（itemTypes: table/column）。
	kindDbschema = "dbschema"
	// kindDependency 是服务间依赖资产 kind（itemTypes: edge）。
	kindDependency = "dependency"
)

const (
	// itemTypeTable 是 dbschema kind 的表条目类型。
	itemTypeTable = "table"
	// itemTypeColumn 是 dbschema kind 的列条目类型。
	itemTypeColumn = "column"
	// itemTypeEdge 是 dependency kind 的依赖边条目类型。
	itemTypeEdge = "edge"
)

const (
	// dbschemaSchemaVersion 是 dbschema 内容契约的固定版本。
	dbschemaSchemaVersion = "meridian-dbschema-1"
	// dependencySchemaVersion 是 dependency 内容契约的固定版本。
	dependencySchemaVersion = "meridian-dependency-1"
)

const (
	// ViewKindDepGraph 是依赖图视图解析的 kind 判别值（openapi ViewResolution.discriminator）。
	ViewKindDepGraph = "dep_graph"
	// ViewKindCatalogDashboard 是目录仪表盘视图解析的 kind 判别值。
	ViewKindCatalogDashboard = "catalog_dashboard"
)

const (
	// groupPermissionManage 是系统分组变更所需的权限点。
	groupPermissionManage = "systemgroup:manage"
)

// 视图输入模式（ViewDefinition.InputMode 判别值）。
const (
	// viewInputModeSingle 表示恰好一个版本输入。
	viewInputModeSingle = "single"
	// viewInputModeVersions 表示恰好两个版本输入（diff 视图）。
	viewInputModeVersions = "versions"
	// viewInputModeCollection 表示零或多个版本输入。
	viewInputModeCollection = "collection"
	// viewInputModeScope 表示作用域（系统分组/租户）输入。
	viewInputModeScope = "scope"
)

// 视图解析结果类别（ViewResolution.Kind 判别值）。
const (
	// viewResolutionKindDocument 表示解析为单文档。
	viewResolutionKindDocument = "document"
	// viewResolutionKindItems 表示解析为条目集合。
	viewResolutionKindItems = "items"
	// viewResolutionKindDashboard 表示解析为仪表盘。
	viewResolutionKindDashboard = "dashboard"
)

// 视图作用域类型（subscriptionScopeType 枚举）。
const (
	// viewScopeTypeSystemGroup 表示系统分组作用域。
	viewScopeTypeSystemGroup = "system_group"
	// viewScopeTypeTenant 表示租户作用域。
	viewScopeTypeTenant = "tenant"
)

// 内置视图标识（views.yaml 注册的视图 ID）。
const (
	// viewIDOperations 是 OpenAPI 操作表格视图。
	viewIDOperations = "operations"
	// viewIDDepGraph 是依赖图视图。
	viewIDDepGraph = "dep-graph"
)

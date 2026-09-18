package service

// 本文件集中放置服务可见性与相关生命周期状态的领域常量。
// 值来源：contracts/domain.yaml 的 serviceVisibility / lifecycle 段。
// 服务生命周期（draft/published/deprecated/retired）常量定义在 service_lifecycle.go；
// 资产版本生命周期（draft/published）常量定义在 ai_constants.go（lifecycleDraft/lifecyclePublished）。

// 服务可见性（serviceVisibility 枚举）。
const (
	// serviceVisibilityPrivate 表示仅租户成员可见。
	serviceVisibilityPrivate = "private"
	// serviceVisibilityInternal 表示租户内部公开。
	serviceVisibilityInternal = "internal"
	// serviceVisibilityPublic 表示匿名可读（受 lifecycle 门控）。
	serviceVisibilityPublic = "public"
)

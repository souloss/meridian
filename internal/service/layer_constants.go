package service

// 本文件集中放置 layer 编辑域（M2 overlay/provenance/回滚/排序）的领域常量，
// 按「业务领域常量」归口。DB 列值常量见对应 model 文件与 migration 注释，
// 金额/精度等技术与技术维度常量不在此列。

const (
	// layerRoleBase 是资产唯一的 base 层角色，ord 恒为 0。
	layerRoleBase = "base"
	// layerRoleOverlay 是参与合并排序的 overlay 层角色。
	layerRoleOverlay = "overlay"
	// layerOriginManual 是手动提交 overlay 的来源。
	layerOriginManual = "manual"
)

const (
	// overlayScopeGlobal 表示跨引用共享的全局层头作用域。
	overlayScopeGlobal = "global"
	// overlayScopeKeyGlobal 是全局作用域的固定 scope_key。
	overlayScopeKeyGlobal = "*"
	// overlayScopeRef 表示绑定到具体 branch/tag 的层头作用域。
	overlayScopeRef = "ref"
	// overlayRefTypeBranch 是 ref 作用域的 branch 引用类型。
	overlayRefTypeBranch = "branch"
)

const (
	// layerRevisionContentTypeYAML 是层修订内容的默认媒体类型。
	layerRevisionContentTypeYAML = "application/yaml"
	// layerRevisionReviewNotRequired 是手动/repo 修订的默认审核状态。
	layerRevisionReviewNotRequired = "not_required"
	// layerRevisionReviewApproved 是已批准修订的审核状态，允许回滚。
	layerRevisionReviewApproved = "approved"
	// layerRevisionReviewPending 是待审核候选修订的审核状态。
	layerRevisionReviewPending = "pending_review"
)

const (
	// layerOrdBase 是 base 层固定的排序号。
	layerOrdBase = 0
	// layerGenerationBase 是新建层头的初始代次。
	layerGenerationBase = 1
)

const (
	// mergeScopeRefSelectorPrefix 是精确分支选择器作用域键的拼接前缀。
	mergeScopeRefSelectorPrefix = ":"
	// mergeJobRefType 是回滚触发的合并任务的引用类型。
	mergeJobRefType = "branch"
)

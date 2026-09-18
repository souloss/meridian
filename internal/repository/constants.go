// Package repository 的包内具名常量。仅收容 service 包未导出的仓库域字面量；
// 凡是 service 包已导出的任务状态（service.JobStatus*）、错误码（service.ErrorCode*）
// 与哨兵错误（service.Err*）一律直接引用，不在本文件重复定义。
package repository

const (
	// pgUniqueViolationCode 是 PostgreSQL 唯一约束冲突（unique_violation）的 SQLSTATE 码，
	// normalizeError 用它把 23505 映射为 service.ErrDuplicate。
	pgUniqueViolationCode = "23505"

	// sha256DigestBytes 是 SHA-256 摘要的字节长度，用作幂等请求摘要的固定校验边界。
	sha256DigestBytes = 32
)

const (
	// jobStageLevelInfo 是任务阶段日志的 info 级别（jobs.stage_logs.level）。
	jobStageLevelInfo = "info"
	// jobStageLevelError 是任务阶段日志的 error 级别（jobs.stage_logs.level）。
	jobStageLevelError = "error"

	// jobStageLogMessageStarted 是任务阶段启动时写入日志的固定消息。
	jobStageLogMessageStarted = "pipeline stage started"
	// jobErrorDefaultMessage 是任务失败且无预置消息时的兜底提示。
	jobErrorDefaultMessage = "job execution failed"
	// jobStageFailedMessage 是任务阶段失败且无错误详情时的兜底提示。
	jobStageFailedMessage = "job stage failed"
	// jobErrorCodeWorkerFailed 是任务错误码的旧值（worker_failed），解析时统一归一为 internal_error。
	jobErrorCodeWorkerFailed = "worker_failed"

	// collectFailedEventType 是任务失败通知事件的 AsyncAPI 事件名（collect.failed）。
	collectFailedEventType = "collect.failed"
	// collectFailedAggregateType 是任务失败通知事件的聚合类别（job）。
	collectFailedAggregateType = "job"

	// jobTypeRepoSync 是仓库同步任务的类型（jobs.type: repo.sync），重试门禁按此类型识别可重试任务。
	jobTypeRepoSync = "repo.sync"
)

const (
	// sourceModeManual 是手动来源模式的列值（source_specs.mode: manual）。
	sourceModeManual = "manual"
	// sourceModeBuiltin 是内置仓库基座来源模式的列值（source_specs.mode: builtin）。
	sourceModeBuiltin = "builtin"
	// sourceModeAI 是 AI 生成来源模式的列值（source_specs.mode: ai）。
	sourceModeAI = "ai"

	// sourceOriginRepo 是仓库基座来源的列值（source_specs.origin: repo）。
	sourceOriginRepo = "repo"
	// sourceOriginAI 是 AI 生成来源的列值（source_specs.origin: ai_generated）。
	sourceOriginAI = "ai_generated"
	// sourceConfigOriginAPI 表示来源配置来自 API 输入（source_specs.config_origin: api）。
	sourceConfigOriginAPI = "api"

	// sourceScopeTypeGlobal 是手动源全局绑定的作用域类别（source_bindings.scope_type: global）。
	sourceScopeTypeGlobal = "global"
	// sourceScopeKeyGlobal 是全局作用域的固定 scope_key（*）。
	sourceScopeKeyGlobal = "*"
	// sourceBindingStateActive 是生效中绑定的状态列值（source_bindings.state: active）。
	sourceBindingStateActive = "active"
)

const (
	// layerRoleBase 是资产唯一 base 层的角色列值（layers.role: base）。
	layerRoleBase = "base"
	// layerOrdBase 是 base 层固定的排序号（layers.ord: 0）。
	layerOrdBase = 0

	// branchGlobAll 是覆盖所有分支的 branch_patterns 通配符（**）。
	branchGlobAll = "**"
)

const (
	// credentialSharedScopeTeam 是凭据按团队共享的作用域列值（credentials.shared_scope: team）。
	credentialSharedScopeTeam = "team"
	// credentialRotationSyncReason 是凭据轮换触发同步任务的固定原因标签。
	credentialRotationSyncReason = "credential-rotated"
)

const (
	// quotaResourceStorageBytes 是 blob 存储配额资源名（tenant quota storageBytes）。
	quotaResourceStorageBytes = "storageBytes"
	// quotaResourceRepositories 是仓库数量配额资源名（tenant quota repositories）。
	quotaResourceRepositories = "repositories"
)

const (
	// jobGenerationInitial 是新建任务的首个活跃代次（jobs.active_generation 初始值 1）。
	jobGenerationInitial = int64(1)
	// rowsAffectedOne 是乐观并发门禁期望「恰好影响一行」的行数边界。
	rowsAffectedOne = 1
)

const (
	// aiSourceTimeoutSec 是 AI 源配置的缺省超时秒数（与 service.aiDefaultTimeoutSec 同口径）。
	aiSourceTimeoutSec = 600
	// repoBaseSourceTimeoutSec 是仓库基座源配置的缺省超时秒数（domain.yaml timeoutSecondsByKind.openapi）。
	repoBaseSourceTimeoutSec = 120
	// repoBaseSourceAssetNameTemplate 是仓库基座源配置的资产名模板占位符（{file_stem}）。
	repoBaseSourceAssetNameTemplate = "{file_stem}"
	// repoBaseSourcePath 是仓库基座源配置的缺省相对路径（openapi.yaml）。
	repoBaseSourcePath = "openapi.yaml"
)

const (
	// serviceLifecycleDraft 是新建服务的初始生命周期列值（services.lifecycle: draft）。
	serviceLifecycleDraft = "draft"
)

// 去重键前缀：与 dedupeKey 拼接规则一一对应，值必须与 migration 中 jobs.dedupe_key 的产生口径一致。
const (
	// dedupeKeyPrefixAI 是 AI 生成任务去重键的前缀。
	dedupeKeyPrefixAI = "ai:"
	// dedupeKeyPrefixMerge 是合并任务去重键的前缀。
	dedupeKeyPrefixMerge = "merge:"
	// dedupeKeyPrefixDiscover 是仓库发现任务去重键的前缀。
	dedupeKeyPrefixDiscover = "discover:"
	// dedupeKeyPrefixSync 是仓库同步任务去重键的前缀。
	dedupeKeyPrefixSync = "repo:"
	// dedupeKeyRepositoryBranch 是凭据同步任务去重键的「repository:branch」段前缀。
	dedupeKeyRepositoryBranch = "repository:"
)

// searchKindFilterCount 是搜索支持单 kind 过滤所需传入的 kind 个数。
const searchKindFilterCount = 1

// 幂等/控制锁键的作用域或前缀：值必须与各自 Postgres advisory/row 锁键的拼接口径一致。
const (
	// aiGenerationIdempotencyLockPrefix 是 AI 生成幂等锁键的前缀。
	aiGenerationIdempotencyLockPrefix = "generateMissingAssetWithAi:"
	// rotationLockScopeTenant 是租户凭据轮换幂等锁键的作用域段。
	rotationLockScopeTenant = "tenant"
	// rotationLockScopePlatform 是平台凭据轮换幂等锁键的作用域段。
	rotationLockScopePlatform = "platform"
	// syncIdempotencyLockPrefix 是仓库同步幂等锁键的前缀。
	syncIdempotencyLockPrefix = "syncRepository:"
	// retryJobLockPrefix 是任务重试幂等锁键的前缀。
	retryJobLockPrefix = "retryJob:"
	// jobStageSequenceLockPrefix 是任务阶段日志序列号的咨询锁键前缀。
	jobStageSequenceLockPrefix = "job-stage:"
)

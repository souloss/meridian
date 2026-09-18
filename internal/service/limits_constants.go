package service

// 本文件集中放置跨域共用的业务边界值常量。
// 值来源：contracts/domain.yaml 与 openapi.yaml 的字段上限、分页约定；单域边界值仍保留在各自文件。

const (
	// defaultPageSizeMax 是分页请求 pageSize 的上限（分页 API 统一约定）。
	defaultPageSizeMax = 100
	// maxDisplayNameRunes 是展示名/用户名的统一长度上限（rune）。
	// 与 discovery.go 的 maxServiceDisplayNameRunes（同为 128）同口径，此处为身份/分组域的通用名。
	maxDisplayNameRunes = 128
	// maxFilterTextRunes 是审计/任务过滤等短文本筛选字段的长度上限（rune）。
	maxFilterTextRunes = 128
	// maxTokenNameRunes 是个人访问令牌（PAT）名称的长度上限（rune）。
	maxTokenNameRunes = 64
	// sourceTimeoutMinSec 是源配置超时的下限（秒）。
	sourceTimeoutMinSec = 10
	// sourceTimeoutMaxSec 是源配置超时的上限（秒）。
	sourceTimeoutMaxSec = 3600
	// maxRepositoryDepth 是仓库浅克隆深度的上限。
	maxRepositoryDepth = 10000
	// httpsDefaultPort 是 HTTPS 的默认端口，用于 URL 规范化时去除默认端口。
	httpsDefaultPort = 443
	// credentialScopeGlobal 是全局凭据的加密作用域标识（credential_crypto.go 的 Encrypt/Decrypt scopeID）。
	// 与租户凭据使用 tenantID 字符串不同，平台凭据固定使用该值。
	credentialScopeGlobal = "global"
	// quotaResourceRepositories 是仓库配额的维度标识，用于 QuotaExceededError。
	quotaResourceRepositories = "repositories"
	// contentTypeYAML 是 YAML 文档的通用媒体类型。
	contentTypeYAML = "application/yaml"
	// maxCandidatesPerAccept 是一次 AcceptCandidates 请求可接受的候选数上限。
	maxCandidatesPerAccept = 100
	// itemsFetchBatchSize 是单次拉取资产条目的批量上限（用于索引视图/差异解析）。
	itemsFetchBatchSize = 100
)

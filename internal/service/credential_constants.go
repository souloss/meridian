package service

// 本文件集中放置凭据与连接探测域的领域常量。
// 值来源：contracts/domain.yaml 的 credential / knownHost 段；DB 列值常量见对应 migration 注释。

// 凭据 kind 枚举（credentialCreateRequest 的 sshKey / httpToken 判别值）。
const (
	// credentialKindSSHKey 是 SSH 私钥类凭据。
	credentialKindSSHKey = "ssh_key"
	// credentialKindHTTPToken 是 HTTP 用户名 + 令牌类凭据。
	credentialKindHTTPToken = "http_token"
)

// 凭据共享作用域（credentialSharedScope 枚举）。
const (
	// credentialSharedScopePrivate 表示仅创建者可见。
	credentialSharedScopePrivate = "private"
	// credentialSharedScopeTenant 表示租户内全部成员可见。
	credentialSharedScopeTenant = "tenant"
	// credentialSharedScopeTeam 表示按团队 ID 列表共享。
	credentialSharedScopeTeam = "team"
)

// 已知主机来源。
const (
	// knownHostSourceManual 表示手动批准录入的已知主机。
	// 与 layer_constants.go 的 layerOriginManual（同为 "manual"）值相同但语义不同（此处是 known_hosts.source 枚举）。
	knownHostSourceManual = "manual"
)

// 连接探测错误类别（ConnectionTestResult.ErrorClass）。
const (
	// probeErrorClassDNS 表示远端主机无法解析。
	probeErrorClassDNS = "dns"
	// probeErrorClassAuth 表示远端认证失败。
	probeErrorClassAuth = "auth"
	// probeErrorClassHostKey 表示远端主机密钥未受信任。
	probeErrorClassHostKey = "host_key"
	// probeErrorClassTimeout 表示探测超时。
	probeErrorClassTimeout = "timeout"
	// probeErrorClassOther 表示未分类的连接失败。
	probeErrorClassOther = "other"
)

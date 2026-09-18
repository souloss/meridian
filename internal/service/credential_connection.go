package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"uuid"

	"golang.org/x/crypto/ssh"
)

const (
	defaultConnectionProbeTimeout = 10 * time.Second
	// maxGitRemoteBytes 是 Git 远端字符串的最大字节长度。
	maxGitRemoteBytes = 2048
	// knownHostPortMin 是主机密钥端口的最小值。
	knownHostPortMin = 1
	// knownHostPortMax 是主机密钥端口的最大值。
	knownHostPortMax = 65535
)

// ConnectionTestResult 是 Git 远端探测的脱敏结果。秘密材料与命令输出绝不承载于该结果中。
type ConnectionTestResult struct {
	// OK 表示远端接受了探测并通告了其引用。
	OK bool
	// ErrorClass 是探测失败时的契约错误类别；成功时为空。
	ErrorClass string
	// Message 是稳定、脱敏的面向运营人员的结局说明。
	Message string
	// HostKeyCandidate 在需要信任批准时承载安全的 SSH 主机身份数据。
	HostKeyCandidate *ConnectionHostKeyCandidate
}

// ConnectionHostKeyCandidate 是探测返回的安全 SSH 主机身份材料。私钥与任何凭据秘密有意缺省。
type ConnectionHostKeyCandidate struct {
	// Host 是 SSH 握手返回的规范化主机名。
	Host string
	// Port 是 SSH 握手使用的 TCP 端口。
	Port int
	// KeyType 标识受支持的 SSH 公钥算法。
	KeyType string
	// PublicKey 是可用于信任批准的 base64 RFC 4253 公钥 blob。
	PublicKey string
	// Fingerprint 是派生的 OpenSSH SHA-256 主机密钥指纹。
	Fingerprint string
}

// ConnectionProbe 执行凭据感知、有界的 Git 远端探测。当 URL 含用户控制数据时，实现不得记录它。
type ConnectionProbe interface {
	Probe(context.Context, string, *CredentialSecret) (ConnectionTestResult, error)
}

// GitConnectionProbe 使用 Git CLI 的 ls-remote 协议协商。CLI 有意隔离在该接口之后，
// 以便测试提供确定性探测。
type GitConnectionProbe struct {
	// GitBinary 是探测使用的可执行文件；为空时从 PATH 选择 git。
	GitBinary string
	// Timeout 界定完整的 Git 进程与网络操作时长。
	Timeout time.Duration
}

// NewGitConnectionProbe 构造生产环境的 Git 连接探测。
func NewGitConnectionProbe() GitConnectionProbe {
	return GitConnectionProbe{GitBinary: "git", Timeout: defaultConnectionProbeTimeout}
}

// Probe 在不克隆内容的前提下检查远端是否可达且可认证。
func (probe GitConnectionProbe) Probe(parent context.Context, remote string, secret *CredentialSecret) (ConnectionTestResult, error) {
	if err := validateGitRemote(remote); err != nil {
		return ConnectionTestResult{ErrorClass: probeErrorClassOther, Message: "repository URL is invalid"}, nil
	}
	timeout := probe.Timeout
	if timeout <= 0 {
		timeout = defaultConnectionProbeTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	gitBinary := probe.GitBinary
	if gitBinary == "" {
		gitBinary = "git"
	}
	command := exec.CommandContext(ctx, gitBinary, "ls-remote", "--symref", "--heads", remote)
	command.Env = append([]string(nil), os.Environ()...)
	cleanup, err := configureProbeCredentials(command, secret)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	defer cleanup()

	output, err := command.CombinedOutput()
	if err == nil {
		return ConnectionTestResult{OK: true, Message: "repository connection succeeded"}, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ConnectionTestResult{ErrorClass: probeErrorClassTimeout, Message: "repository connection timed out"}, nil
	}
	errorClass := classifyProbeError(string(output))
	result := ConnectionTestResult{ErrorClass: errorClass, Message: probeMessage(errorClass)}
	if errorClass == probeErrorClassHostKey {
		result.HostKeyCandidate = probeHostKeyCandidate(ctx, remote)
	}
	return result, nil
}

// probeHostKeyCandidate 在 Git 报告未受信任主机密钥后执行有界的未认证 SSH 握手以捕获
// 服务器密钥。密钥仅作为派生候选返回，绝不接受或持久化。
func probeHostKeyCandidate(ctx context.Context, remote string) *ConnectionHostKeyCandidate {
	ctx, cancel := context.WithTimeout(ctx, defaultConnectionProbeTimeout)
	defer cancel()
	host, port, ok := sshRemoteAddress(remote)
	if !ok {
		return nil
	}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	var candidate *ConnectionHostKeyCandidate
	config := &ssh.ClientConfig{
		User: "git",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			identity, parseErr := ParseKnownHostPublicKey(base64.StdEncoding.EncodeToString(key.Marshal()))
			if parseErr != nil {
				return parseErr
			}
			candidate = &ConnectionHostKeyCandidate{Host: host, Port: port, KeyType: identity.KeyType, PublicKey: base64.StdEncoding.EncodeToString(identity.PublicKey), Fingerprint: identity.Fingerprint}
			return ErrUntrustedHostKeyCandidate
		},
	}
	_, _, _, _ = ssh.NewClientConn(conn, net.JoinHostPort(host, strconv.Itoa(port)), config)
	return candidate
}

func sshRemoteAddress(remote string) (host string, port int, ok bool) {
	if strings.HasPrefix(remote, "ssh://") {
		parsed, err := url.ParseRequestURI(remote)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil {
			return "", 0, false
		}
		port = knownHostDefaultPort
		if parsed.Port() != "" {
			port, err = strconv.Atoi(parsed.Port())
			if err != nil || port < knownHostPortMin || port > knownHostPortMax {
				return "", 0, false
			}
		}
		return strings.ToLower(parsed.Hostname()), port, true
	}
	left, _, found := strings.Cut(remote, ":")
	if !found {
		return "", 0, false
	}
	_, host, found = strings.Cut(left, "@")
	if !found || host == "" {
		return "", 0, false
	}
	return strings.ToLower(host), knownHostDefaultPort, true
}

func validateGitRemote(remote string) error {
	if len(remote) == 0 || len(remote) > maxGitRemoteBytes || remote != strings.TrimSpace(remote) || strings.ContainsAny(remote, "\x00\r\n\t ") {
		return ErrInvalidGitRemote
	}
	if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "ssh://") {
		parsed, err := url.ParseRequestURI(remote)
		if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Path == "" {
			return ErrInvalidGitRemoteURI
		}
		if parsed.Scheme != "https" && parsed.Scheme != "ssh" {
			return ErrUnsupportedGitRemoteScheme
		}
		if parsed.Port() != "" {
			if _, err := url.Parse("//" + parsed.Host); err != nil {
				return ErrInvalidGitRemotePort
			}
		}
		return nil
	}
	colon := strings.IndexByte(remote, ':')
	if colon <= 0 || colon == len(remote)-1 || strings.Contains(remote[:colon], "/") || !strings.ContainsRune(remote[:colon], '@') {
		return ErrInvalidSCPGitRemote
	}
	return nil
}

func configureProbeCredentials(command *exec.Cmd, secret *CredentialSecret) (func(), error) {
	if secret == nil {
		return func() {}, nil
	}
	if secret.Kind == credentialKindHTTPToken {
		askpassDirectory, err := os.MkdirTemp("", "meridian-git-askpass-")
		if err != nil {
			return nil, fmt.Errorf("create Git askpass directory: %w", err)
		}
		askpassPath := filepath.Join(askpassDirectory, "askpass")
		const askpass = "#!/bin/sh\ncase \"$1\" in\n  *Username*) printf '%s' \"$MERIDIAN_ASKPASS_USERNAME\" ;;\n  *) printf '%s' \"$MERIDIAN_ASKPASS_TOKEN\" ;;\nesac\n"
		if err := os.WriteFile(askpassPath, []byte(askpass), 0o700); err != nil {
			_ = os.RemoveAll(askpassDirectory)
			return nil, fmt.Errorf("write Git askpass helper: %w", err)
		}
		command.Env = append(command.Env,
			"GIT_TERMINAL_PROMPT=0",
			"GIT_ASKPASS="+askpassPath,
			"MERIDIAN_ASKPASS_USERNAME="+secret.HTTPUsername,
			"MERIDIAN_ASKPASS_TOKEN="+secret.HTTPToken,
		)
		return func() { _ = os.RemoveAll(askpassDirectory) }, nil
	}
	if secret.Kind == credentialKindSSHKey {
		keyDirectory, err := os.MkdirTemp("", "meridian-git-key-")
		if err != nil {
			return nil, fmt.Errorf("create Git key directory: %w", err)
		}
		keyPath := filepath.Join(keyDirectory, "id_key")
		if err := os.WriteFile(keyPath, []byte(secret.PrivateKey), 0o600); err != nil {
			_ = os.RemoveAll(keyDirectory)
			return nil, fmt.Errorf("write Git key material: %w", err)
		}
		sshCommand := fmt.Sprintf("ssh -i %q -o IdentitiesOnly=yes -o BatchMode=%t", keyPath, secret.Passphrase == nil)
		command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND="+sshCommand)
		if secret.Passphrase != nil {
			askpassPath := filepath.Join(keyDirectory, "ssh-askpass")
			const askpass = "#!/bin/sh\nprintf '%s' \"$MERIDIAN_SSH_PASSPHRASE\"\n"
			if err := os.WriteFile(askpassPath, []byte(askpass), 0o700); err != nil {
				_ = os.RemoveAll(keyDirectory)
				return nil, fmt.Errorf("write SSH askpass helper: %w", err)
			}
			command.Env = append(command.Env,
				"SSH_ASKPASS="+askpassPath,
				"SSH_ASKPASS_REQUIRE=force",
				"MERIDIAN_SSH_PASSPHRASE="+*secret.Passphrase,
			)
		}
		return func() { _ = os.RemoveAll(keyDirectory) }, nil
	}
	return func() {}, nil
}

func classifyProbeError(output string) string {
	message := strings.ToLower(output)
	switch {
	case strings.Contains(message, "could not resolve host"),
		strings.Contains(message, "name or service not known"),
		strings.Contains(message, "temporary failure in name resolution"),
		strings.Contains(message, "no such host"):
		return probeErrorClassDNS
	case strings.Contains(message, "host key verification failed"),
		strings.Contains(message, "known_hosts"),
		strings.Contains(message, "offending"):
		return probeErrorClassHostKey
	case strings.Contains(message, "authentication failed"),
		strings.Contains(message, "permission denied"),
		strings.Contains(message, "could not read username"),
		strings.Contains(message, "authentication required"),
		strings.Contains(message, "terminal prompts disabled"):
		return probeErrorClassAuth
	default:
		return probeErrorClassOther
	}
}

func probeMessage(errorClass string) string {
	switch errorClass {
	case probeErrorClassDNS:
		return "repository host could not be resolved"
	case probeErrorClassAuth:
		return "repository authentication failed"
	case probeErrorClassHostKey:
		return "repository host key is not trusted"
	case probeErrorClassTimeout:
		return "repository connection timed out"
	default:
		return "repository connection failed"
	}
}

func (credentials *Credentials) TestTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, remote string) (ConnectionTestResult, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialRead)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	record, err := credentials.store.GetCredential(ctx, membership.TenantID, id, actor.User.ID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return ConnectionTestResult{}, err
		}
		global, globalErr := credentials.store.GetGlobalCredential(ctx, id)
		if globalErr != nil {
			return ConnectionTestResult{}, err
		}
		return credentials.probeRecord(ctx, remote, credentialScopeGlobal, global.ID, global.Kind, global.Encrypted)
	}
	return credentials.probeRecord(ctx, remote, membership.TenantID.String(), record.ID, record.Kind, record.Encrypted)
}

func (credentials *Credentials) TestGlobal(ctx context.Context, actor Principal, id uuid.UUID, remote string) (ConnectionTestResult, error) {
	if !isPlatformAdministrator(actor) {
		return ConnectionTestResult{}, ErrNotFound
	}
	record, err := credentials.store.GetGlobalCredential(ctx, id)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	return credentials.probeRecord(ctx, remote, credentialScopeGlobal, record.ID, record.Kind, record.Encrypted)
}

func (credentials *Credentials) CheckRepositoryConnection(ctx context.Context, actor Principal, tenantSlug, remote string, credentialID *uuid.UUID) (ConnectionTestResult, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryWrite)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	if credentialID == nil {
		return credentials.probe.Probe(ctx, remote, nil)
	}
	record, err := credentials.store.GetCredential(ctx, membership.TenantID, *credentialID, actor.User.ID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return ConnectionTestResult{}, err
		}
		global, globalErr := credentials.store.GetGlobalCredential(ctx, *credentialID)
		if globalErr != nil {
			return ConnectionTestResult{}, err
		}
		return credentials.probeRecord(ctx, remote, credentialScopeGlobal, global.ID, global.Kind, global.Encrypted)
	}
	return credentials.probeRecord(ctx, remote, membership.TenantID.String(), record.ID, record.Kind, record.Encrypted)
}

func (credentials *Credentials) probeRecord(ctx context.Context, remote, scope string, id uuid.UUID, kind string, encrypted EncryptedCredential) (ConnectionTestResult, error) {
	secret, err := credentials.keyring.Decrypt(scope, id, kind, encrypted)
	if err != nil {
		return ConnectionTestResult{}, fmt.Errorf("decrypt credential for connection probe: %w", err)
	}
	return credentials.probe.Probe(ctx, remote, new(secret))
}

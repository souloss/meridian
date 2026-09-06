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

const defaultConnectionProbeTimeout = 10 * time.Second

// ConnectionTestResult is the redacted outcome of a Git remote probe.
// Secret material and command output are never carried in this result.
type ConnectionTestResult struct {
	// OK indicates that the remote accepted the probe and advertised its refs.
	OK bool
	// ErrorClass is a contract error class when the probe failed; it is empty on success.
	ErrorClass string
	// Message is a stable, redacted operator-facing explanation of the outcome.
	Message string
	// HostKeyCandidate carries safe SSH host identity data when trust approval is required.
	HostKeyCandidate *ConnectionHostKeyCandidate
}

// ConnectionHostKeyCandidate is safe-to-return SSH host identity material from a probe.
// The private key and any credential secret are intentionally absent.
type ConnectionHostKeyCandidate struct {
	// Host is the normalized hostname returned by the SSH handshake.
	Host string
	// Port is the TCP port used by the SSH handshake.
	Port int
	// KeyType identifies the supported SSH public-key algorithm.
	KeyType string
	// PublicKey is the base64 RFC 4253 public-key blob safe for trust approval.
	PublicKey string
	// Fingerprint is the derived OpenSSH SHA-256 host-key fingerprint.
	Fingerprint string
}

// ConnectionProbe performs a credential-aware, bounded Git remote probe.
// Implementations must not log the URL when it contains user-controlled data.
type ConnectionProbe interface {
	Probe(context.Context, string, *CredentialSecret) (ConnectionTestResult, error)
}

// GitConnectionProbe uses the Git CLI's ls-remote protocol negotiation.
// The CLI is deliberately isolated behind this interface so tests can provide a deterministic probe.
type GitConnectionProbe struct {
	// GitBinary is the executable used for the probe; empty selects git from PATH.
	GitBinary string
	// Timeout bounds the complete Git process and network operation.
	Timeout time.Duration
}

// NewGitConnectionProbe constructs the production Git connection probe.
func NewGitConnectionProbe() GitConnectionProbe {
	return GitConnectionProbe{GitBinary: "git", Timeout: defaultConnectionProbeTimeout}
}

// Probe checks whether the remote can be reached and authenticated without cloning content.
func (probe GitConnectionProbe) Probe(parent context.Context, remote string, secret *CredentialSecret) (ConnectionTestResult, error) {
	if err := validateGitRemote(remote); err != nil {
		return ConnectionTestResult{ErrorClass: "other", Message: "repository URL is invalid"}, nil
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
		return ConnectionTestResult{ErrorClass: "timeout", Message: "repository connection timed out"}, nil
	}
	errorClass := classifyProbeError(string(output))
	result := ConnectionTestResult{ErrorClass: errorClass, Message: probeMessage(errorClass)}
	if errorClass == "host_key" {
		result.HostKeyCandidate = probeHostKeyCandidate(ctx, remote)
	}
	return result, nil
}

// probeHostKeyCandidate performs a bounded unauthenticated SSH handshake to
// capture the server key after Git reports an untrusted host key. The key is
// returned only as a derived candidate; it is never accepted or persisted.
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
			return errors.New("untrusted host key candidate captured")
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
		port = 22
		if parsed.Port() != "" {
			port, err = strconv.Atoi(parsed.Port())
			if err != nil || port < 1 || port > 65535 {
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
	return strings.ToLower(host), 22, true
}

func validateGitRemote(remote string) error {
	if len(remote) == 0 || len(remote) > 2048 || remote != strings.TrimSpace(remote) || strings.ContainsAny(remote, "\x00\r\n\t ") {
		return errors.New("invalid Git remote")
	}
	if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "ssh://") {
		parsed, err := url.ParseRequestURI(remote)
		if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Path == "" {
			return errors.New("invalid Git remote URI")
		}
		if parsed.Scheme != "https" && parsed.Scheme != "ssh" {
			return errors.New("unsupported Git remote scheme")
		}
		if parsed.Port() != "" {
			if _, err := url.Parse("//" + parsed.Host); err != nil {
				return errors.New("invalid Git remote port")
			}
		}
		return nil
	}
	colon := strings.IndexByte(remote, ':')
	if colon <= 0 || colon == len(remote)-1 || strings.Contains(remote[:colon], "/") || !strings.ContainsRune(remote[:colon], '@') {
		return errors.New("invalid SCP-like Git remote")
	}
	return nil
}

func configureProbeCredentials(command *exec.Cmd, secret *CredentialSecret) (func(), error) {
	if secret == nil {
		return func() {}, nil
	}
	if secret.Kind == "http_token" {
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
	if secret.Kind == "ssh_key" {
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
		return "dns"
	case strings.Contains(message, "host key verification failed"),
		strings.Contains(message, "known_hosts"),
		strings.Contains(message, "offending"):
		return "host_key"
	case strings.Contains(message, "authentication failed"),
		strings.Contains(message, "permission denied"),
		strings.Contains(message, "could not read username"),
		strings.Contains(message, "authentication required"),
		strings.Contains(message, "terminal prompts disabled"):
		return "auth"
	default:
		return "other"
	}
}

func probeMessage(errorClass string) string {
	switch errorClass {
	case "dns":
		return "repository host could not be resolved"
	case "auth":
		return "repository authentication failed"
	case "host_key":
		return "repository host key is not trusted"
	case "timeout":
		return "repository connection timed out"
	default:
		return "repository connection failed"
	}
}

func (credentials *Credentials) TestTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, remote string) (ConnectionTestResult, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, "credential:read")
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
		return credentials.probeRecord(ctx, remote, "global", global.ID, global.Kind, global.Encrypted)
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
	return credentials.probeRecord(ctx, remote, "global", record.ID, record.Kind, record.Encrypted)
}

func (credentials *Credentials) CheckRepositoryConnection(ctx context.Context, actor Principal, tenantSlug, remote string, credentialID *uuid.UUID) (ConnectionTestResult, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, "repository:write")
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
		return credentials.probeRecord(ctx, remote, "global", global.ID, global.Kind, global.Encrypted)
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

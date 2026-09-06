package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestGitConnectionProbeReturnsRedactedResults(t *testing.T) {
	tests := []struct {
		name        string
		stderr      string
		exitCode    string
		wantOK      bool
		wantClass   string
		wantMessage string
	}{
		{name: "success", exitCode: "0", wantOK: true, wantMessage: "repository connection succeeded"},
		{name: "dns", stderr: "fatal: Could not resolve host: example.invalid\n", exitCode: "1", wantClass: "dns", wantMessage: "repository host could not be resolved"},
		{name: "auth", stderr: "fatal: Authentication failed\n", exitCode: "1", wantClass: "auth", wantMessage: "repository authentication failed"},
		{name: "host key", stderr: "Host key verification failed.\n", exitCode: "1", wantClass: "host_key", wantMessage: "repository host key is not trusted"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			binary := writeProbeScript(t, test.stderr, test.exitCode)
			result, err := (GitConnectionProbe{GitBinary: binary, Timeout: time.Second}).Probe(t.Context(), "https://example.com/repository.git", nil)
			if err != nil {
				t.Fatalf("probe returned error: %v", err)
			}
			if result.OK != test.wantOK || result.ErrorClass != test.wantClass || result.Message != test.wantMessage {
				t.Fatalf("probe result = %#v, want ok=%t class=%q message=%q", result, test.wantOK, test.wantClass, test.wantMessage)
			}
		})
	}
}

func TestGitConnectionProbeRejectsUnsafeRemote(t *testing.T) {
	for _, remote := range []string{
		"",
		" https://example.com/repository.git",
		"https://user:secret@example.com/repository.git",
		"http://example.com/repository.git",
		"git@example.com",
		"git@example.com:",
	} {
		result, err := NewGitConnectionProbe().Probe(t.Context(), remote, nil)
		if err != nil {
			t.Fatalf("probe(%q) returned error: %v", remote, err)
		}
		if result.OK || result.ErrorClass != "other" || result.Message != "repository URL is invalid" {
			t.Fatalf("probe(%q) = %#v, want invalid result", remote, result)
		}
	}
}

func TestGitConnectionProbeTimeoutIsClassified(t *testing.T) {
	binary := writeProbeScript(t, "", "")
	probe := GitConnectionProbe{GitBinary: binary, Timeout: 5 * time.Millisecond}
	result, err := probe.Probe(context.Background(), "https://example.com/repository.git", nil)
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if result.OK || result.ErrorClass != "timeout" || result.Message != "repository connection timed out" {
		t.Fatalf("probe timeout result = %#v", result)
	}
}

func TestGitConnectionProbeDerivesHostKeyCandidate(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate SSH host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("create SSH host signer: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for SSH probe: %v", err)
	}
	var server sync.WaitGroup
	server.Go(func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		config := &ssh.ServerConfig{NoClientAuth: true}
		config.AddHostKey(signer)
		_, _, _, _ = ssh.NewServerConn(conn, config)
	})
	t.Cleanup(func() {
		_ = listener.Close()
		server.Wait()
	})
	remote := "ssh://" + listener.Addr().String() + "/repository.git"
	probe := GitConnectionProbe{GitBinary: writeProbeScript(t, "Host key verification failed.\n", "1"), Timeout: 5 * time.Second}
	result, err := probe.Probe(t.Context(), remote, nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.OK || result.ErrorClass != "host_key" || result.HostKeyCandidate == nil {
		t.Fatalf("result = %#v, want untrusted host key candidate", result)
	}
	candidate := result.HostKeyCandidate
	if candidate.Host != "127.0.0.1" || candidate.Port != listener.Addr().(*net.TCPAddr).Port || candidate.KeyType != signer.PublicKey().Type() {
		t.Fatalf("candidate identity = %#v", candidate)
	}
	if candidate.PublicKey != base64.StdEncoding.EncodeToString(signer.PublicKey().Marshal()) || candidate.Fingerprint != ssh.FingerprintSHA256(signer.PublicKey()) {
		t.Fatalf("candidate key derivation = %#v", candidate)
	}
}

func TestSSHRemoteAddress(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		remote string
		host   string
		port   int
		ok     bool
	}{
		{remote: "ssh://git.example.com/repository.git", host: "git.example.com", port: 22, ok: true},
		{remote: "ssh://[::1]:2222/repository.git", host: "::1", port: 2222, ok: true},
		{remote: "git@git.example.com:repository.git", host: "git.example.com", port: 22, ok: true},
		{remote: "https://git.example.com/repository.git"},
		{remote: "ssh://git.example.com:70000/repository.git"},
	} {
		host, port, ok := sshRemoteAddress(test.remote)
		if host != test.host || port != test.port || ok != test.ok {
			t.Errorf("sshRemoteAddress(%q) = %q, %d, %t; want %q, %d, %t", test.remote, host, port, ok, test.host, test.port, test.ok)
		}
	}
}

func writeProbeScript(t *testing.T, stderr, exitCode string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe.sh")
	contents := "#!/bin/sh\n"
	if stderr != "" {
		contents += "printf '%s' " + shellQuote(stderr) + " >&2\n"
	}
	if exitCode == "" {
		contents += "sleep 1\n"
	} else {
		contents += "exit " + exitCode + "\n"
	}
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("write probe script: %v", err)
	}
	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

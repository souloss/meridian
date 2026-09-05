package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

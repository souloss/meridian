package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	root := New()
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"version"})

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if got := output.String(); !strings.HasPrefix(got, "meridian ") {
		t.Fatalf("version output = %q", got)
	}
}

func TestReadPasswordSupportsStdinAndFile(t *testing.T) {
	t.Parallel()

	fromStdin, err := readPassword(strings.NewReader("a secure password\n"), "")
	if err != nil {
		t.Fatalf("read password from stdin: %v", err)
	}
	if fromStdin != "a secure password" {
		t.Fatalf("stdin password = %q", fromStdin)
	}

	filename := filepath.Join(t.TempDir(), "admin-password")
	if err := os.WriteFile(filename, []byte("another secure password\r\n"), 0o600); err != nil {
		t.Fatalf("write password fixture: %v", err)
	}
	fromFile, err := readPassword(strings.NewReader("ignored"), filename)
	if err != nil {
		t.Fatalf("read password from file: %v", err)
	}
	if fromFile != "another secure password" {
		t.Fatalf("file password = %q", fromFile)
	}
}

func TestAdminCreateHasNoPlaintextPasswordFlag(t *testing.T) {
	t.Parallel()

	root := New()
	admin, _, err := root.Find([]string{"admin", "create"})
	if err != nil {
		t.Fatalf("find admin create command: %v", err)
	}
	if admin.Flags().Lookup("password") != nil {
		t.Fatal("admin create exposes a plaintext --password flag")
	}
}

func TestServeRejectsArguments(t *testing.T) {
	t.Parallel()

	root := New()
	root.SetArgs([]string{"serve", "unexpected"})
	if err := root.ExecuteContext(t.Context()); err == nil {
		t.Fatal("serve accepted a positional argument")
	}
}

func TestInsecureCookiesRequireLoopbackListener(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"127.0.0.1:8080", "[::1]:8080", "localhost:8080"} {
		if !isLoopbackAddress(addr) {
			t.Errorf("loopback address %q was rejected", addr)
		}
	}
	for _, addr := range []string{":8080", "0.0.0.0:8080", "192.0.2.1:8080", "invalid"} {
		if isLoopbackAddress(addr) {
			t.Errorf("non-loopback address %q was accepted", addr)
		}
	}
}

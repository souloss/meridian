package command

import (
	"bytes"
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

func TestServeRejectsArguments(t *testing.T) {
	t.Parallel()

	root := New()
	root.SetArgs([]string{"serve", "unexpected"})
	if err := root.ExecuteContext(t.Context()); err == nil {
		t.Fatal("serve accepted a positional argument")
	}
}

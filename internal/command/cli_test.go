package command

import "testing"

// TestDiffExitCodes pins the frozen contracts/cli.yaml diff threshold mapping.
func TestDiffExitCodes(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		failOn   string
		breaking int
		risky    int
		want     int
	}{
		{"none", 0, 0, exitCodeSuccess},
		{"none", 5, 9, exitCodeSuccess},
		{"breaking", 0, 0, exitCodeSuccess},
		{"breaking", 1, 0, exitCodeThreshold},
		{"risky", 0, 1, exitCodeThreshold},
		{"risky", 0, 0, exitCodeSuccess},
		{"risky", 1, 0, exitCodeThreshold},
	} {
		if got := diffExitCode(testCase.failOn, testCase.breaking, testCase.risky); got != testCase.want {
			t.Errorf("diffExitCode(%q, %d, %d) = %d, want %d", testCase.failOn, testCase.breaking, testCase.risky, got, testCase.want)
		}
	}
}

// TestHTTPExitCodes pins the frozen contracts/cli.yaml HTTP status mapping.
func TestHTTPExitCodes(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		status int
		want   int
	}{
		{200, exitCodeSuccess},
		{201, exitCodeSuccess},
		{401, exitCodeUnauthenticated},
		{404, exitCodeNotFound},
		{409, exitCodeConflict},
		{412, exitCodeConflict},
		{500, exitCodeServerFailure},
		{503, exitCodeServerFailure},
	} {
		if got := httpExitCode(testCase.status); got != testCase.want {
			t.Errorf("httpExitCode(%d) = %d, want %d", testCase.status, got, testCase.want)
		}
	}
}

// TestCliErrorExitCode verifies the exported error carries its exit code.
func TestCliErrorExitCode(t *testing.T) {
	t.Parallel()
	if err := usageError("boom"); err == nil {
		t.Fatal("usageError returned nil")
	} else if cliErr, ok := err.(*CliError); !ok || cliErr.ExitCode() != exitCodeUsage {
		t.Fatalf("usageError exit code = %v, want %d", err, exitCodeUsage)
	}
}

// TestCliContract pins the full contracts/cli.yaml exit-code contract asserted by
// SMK-020: the diff fail-on thresholds and every error-class exit code.
func TestCliContract(t *testing.T) {
	t.Parallel()

	// diff.threshold: fail-on none passes regardless of counters; fail-on
	// breaking trips on breaking; fail-on risky trips on risky or breaking.
	for _, testCase := range []struct {
		failOn   string
		breaking int
		risky    int
		want     int
	}{
		{"none", 0, 0, exitCodeSuccess},
		{"none", 5, 9, exitCodeSuccess},
		{"breaking", 0, 0, exitCodeSuccess},
		{"breaking", 1, 0, exitCodeThreshold},
		{"risky", 0, 1, exitCodeThreshold},
		{"risky", 1, 0, exitCodeThreshold},
	} {
		if got := diffExitCode(testCase.failOn, testCase.breaking, testCase.risky); got != testCase.want {
			t.Errorf("diffExitCode(%q, %d, %d) = %d, want %d", testCase.failOn, testCase.breaking, testCase.risky, got, testCase.want)
		}
	}

	// exitCodes.error classes.
	for _, testCase := range []struct {
		name   string
		got    int
		want   int
	}{
		{"localUsageOrValidation", usageError("x").(*CliError).ExitCode(), exitCodeUsage},
		{"http401", httpError(401, "x").(*CliError).ExitCode(), exitCodeUnauthenticated},
		{"http404", httpError(404, "x").(*CliError).ExitCode(), exitCodeNotFound},
		{"http409", httpError(409, "x").(*CliError).ExitCode(), exitCodeConflict},
		{"http412", httpError(412, "x").(*CliError).ExitCode(), exitCodeConflict},
		{"network500", httpError(500, "x").(*CliError).ExitCode(), exitCodeServerFailure},
		{"network503", httpError(503, "x").(*CliError).ExitCode(), exitCodeServerFailure},
		{"waitTimeout", exitCodeWaitTimeout, exitCodeWaitTimeout},
		{"interrupt", exitCodeInterrupted, exitCodeInterrupted},
	} {
		if testCase.got != testCase.want {
			t.Errorf("%s exit code = %d, want %d", testCase.name, testCase.got, testCase.want)
		}
	}
}

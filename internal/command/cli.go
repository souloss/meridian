// Package command implements the meridian command tree.
package command

// CLI exit codes are frozen by contracts/cli.yaml exitCodes. They must not be
// renumbered without a contract change.
const (
	// exitCodeSuccess is returned when the command succeeded and no diff
	// fail-on threshold was exceeded.
	exitCodeSuccess = 0
	// exitCodeUsage is returned for local usage, input, or contract-validation errors.
	exitCodeUsage = 2
	// exitCodeThreshold is returned when the diff fail-on threshold was met.
	exitCodeThreshold = 3
	// exitCodeUnauthenticated is returned for an HTTP 401.
	exitCodeUnauthenticated = 4
	// exitCodeNotFound is returned for HTTP 404 or an unauthorized (hidden) resource.
	exitCodeNotFound = 5
	// exitCodeConflict is returned for HTTP 409 or 412.
	exitCodeConflict = 6
	// exitCodeServerFailure is returned for a network 5xx or terminal job failure.
	exitCodeServerFailure = 7
	// exitCodeWaitTimeout is returned when the client wait timed out with the
	// server job still running.
	exitCodeWaitTimeout = 8
	// exitCodeInterrupted is returned when the client was interrupted while the
	// server job was left running.
	exitCodeInterrupted = 130
)

// CliError couples an exit code to its secret-free message. It is returned by
// commands to control the process exit code.
type CliError struct {
	code    int
	message string
}

// Error satisfies the error interface with a secret-free message.
func (err *CliError) Error() string { return err.message }

// ExitCode returns the contract-defined process exit code.
func (err *CliError) ExitCode() int { return err.code }

func usageError(message string) error { return &CliError{code: exitCodeUsage, message: message} }

// httpError builds a CliError from an HTTP status code.
func httpError(status int, message string) error {
	return &CliError{code: httpExitCode(status), message: message}
}

// diffExitCode maps a diff fail-on threshold and the summary's breaking/risky
// counters to the process exit code per contracts/cli.yaml diff.threshold.
func diffExitCode(failOn string, breaking, risky int) int {
	switch failOn {
	case "breaking":
		if breaking > 0 {
			return exitCodeThreshold
		}
	case "risky":
		if breaking > 0 || risky > 0 {
			return exitCodeThreshold
		}
	}
	return exitCodeSuccess
}

// httpExitCode maps an HTTP status to the CLI exit code per contracts/cli.yaml
// exitCodes. Unknown statuses default to the server-failure code.
func httpExitCode(status int) int {
	switch {
	case status == 401:
		return exitCodeUnauthenticated
	case status == 404:
		return exitCodeNotFound
	case status == 409 || status == 412:
		return exitCodeConflict
	case status >= 500:
		return exitCodeServerFailure
	case status >= 200 && status < 300:
		return exitCodeSuccess
	default:
		return exitCodeServerFailure
	}
}

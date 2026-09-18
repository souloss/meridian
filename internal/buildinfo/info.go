// Package buildinfo 包含 HTTP 服务与 CLI 共享的构建时身份信息。
package buildinfo

import "os"

// 构建身份环境变量名常量。
const (
	// envVersion 是运行时覆盖版本号的环境变量名。
	envVersion = "MERIDIAN_VERSION"
	// envBuildCommit 是运行时覆盖提交号的环境变量名。
	envBuildCommit = "MERIDIAN_BUILD_COMMIT"
)

// 默认构建身份值，由链接期 ldflags 覆盖。
var (
	// Version 是默认版本号，未注入 ldflags 时为 "dev"。
	Version = "dev"
	// Commit 是默认提交号，未注入 ldflags 时为 "unknown"。
	Commit = "unknown"
)

// CurrentVersion 返回环境变量覆盖后生效的版本号。
func CurrentVersion() string {
	if value := os.Getenv(envVersion); value != "" {
		return value
	}
	return Version
}

// CurrentCommit 返回环境变量覆盖后生效的提交号。
func CurrentCommit() string {
	if value := os.Getenv(envBuildCommit); value != "" {
		return value
	}
	return Commit
}

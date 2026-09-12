// Package migrations 将 SQL schema 迁移嵌入 Meridian 二进制。
package migrations

import "embed"

// FS 保存应用自有的 SQL 迁移文件。
// 迁移 SQL 由 Go 二进制嵌入，部署时不依赖当前工作目录。
//
//go:embed *.sql
var FS embed.FS

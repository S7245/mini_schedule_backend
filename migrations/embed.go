// Package migrations 把 SQL migration 文件以 embed.FS 形式打进二进制，
// 让 cmd/api-* 在启动时可以独立于工作目录调用 golang-migrate up。
//
// Batch 4.5 引入：见 pds/batches/batch-04.5-migration-autoboot.md。
package migrations

import "embed"

// FS 包含本目录下所有 *.sql 文件。
// 通过 source/iofs 桥接到 golang-migrate，运行时无需读 ./migrations/ 真实目录。
//
//go:embed *.sql
var FS embed.FS

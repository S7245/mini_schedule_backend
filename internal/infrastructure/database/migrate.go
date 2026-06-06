// Package database 提供运行时的数据库基础设施工具，目前只含 migration 应用。
//
// Batch 4.5 引入：每个 cmd 启动时按 config.Database.AutoMigrateOnBoot 自动应用
// 待执行的 golang-migrate up 步骤。详见 pds/batches/batch-04.5-migration-autoboot.md。
package database

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrationsUp 应用 fs 中所有未执行的 up migration。
//
// 行为：
//   - 已是最新 → 返回 nil（log 标"up to date"）
//   - 检测到 dirty 状态 → 返回错误（per Batch 4.5 决定 #4：拒绝带破损 schema 启动）
//   - 数据库连接 / 解析 / 执行失败 → 返回错误，调用方应 log.Fatal
//
// 多 cmd 同时启动时靠 Postgres advisory lock 互斥（golang-migrate 内置）；
// 后到的 cmd 看到 schema 已就绪会直接 no-op，不会重复跑 up。
func RunMigrationsUp(dsn string, fs embed.FS, logger *slog.Logger) error {
	if dsn == "" {
		return errors.New("RunMigrationsUp: empty DSN")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// embed.FS 的根就是 `migrations/` 包目录本身（pattern `*.sql`）。
	// iofs 路径用 "." 而非 "migrations" —— 后者会再嵌套一层找不到文件。
	src, err := iofs.New(fs, ".")
	if err != nil {
		return fmt.Errorf("init migrations source: %w", err)
	}

	// 复用一份纯 DSN，避免不同 cmd 的连接参数差异污染 migrator。
	// golang-migrate 接受 "postgres://..." DSN；与 GORM 用的同一个连接串兼容。
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("init migrate instance: %w", err)
	}
	defer func() {
		// Close 同时关 source 和 database 句柄；忽略关闭错误，已经记录主流程结果。
		_, _ = m.Close()
	}()

	// 提前检测 dirty 状态，给出明确错误信息。
	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("read schema version: %w", err)
	}
	if dirty {
		return fmt.Errorf("schema_migrations is dirty at version %d; manual recovery required before boot", version)
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("migrations: schema up to date", "version", version)
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}

	newVersion, _, _ := m.Version()
	logger.Info("migrations: applied", "from_version", version, "to_version", newVersion)
	return nil
}

// 仅为编译时确保 postgres driver import 被 link 进二进制（migrate 通过 schema 自动选择）。
var _ = migratepg.Postgres{}

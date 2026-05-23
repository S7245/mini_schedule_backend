package persistence

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GormLogger 自定义 GORM 日志适配器（桥接 slog）
type GormLogger struct {
	log   *slog.Logger
	level logger.LogLevel
}

// NewGormLogger 创建 GORM 日志适配器
func NewGormLogger(log *slog.Logger, level logger.LogLevel) *GormLogger {
	return &GormLogger{log: log, level: level}
}

func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return NewGormLogger(l.log, level)
}

func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Info {
		l.log.InfoContext(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Warn {
		l.log.WarnContext(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Error {
		l.log.ErrorContext(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	args := []any{
		slog.Duration("elapsed", elapsed),
		slog.String("sql", sql),
		slog.Int64("rows", rows),
	}
	if err != nil {
		args = append(args, slog.Any("error", err))
		l.log.ErrorContext(ctx, "GORM trace", args...)
	} else {
		l.log.InfoContext(ctx, "GORM trace", args...)
	}
}

// NewDatabase 创建 GORM 数据库连接
func NewDatabase(cfg *config.DatabaseConfig, log *slog.Logger) (*gorm.DB, error) {
	gormLogLevel := logger.Warn
	switch cfg.LogLevel {
	case "silent":
		gormLogLevel = logger.Silent
	case "error":
		gormLogLevel = logger.Error
	case "warn":
		gormLogLevel = logger.Warn
	case "info":
		gormLogLevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: NewGormLogger(log, gormLogLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("database connected",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.String("dbname", cfg.DBName),
	)

	return db, nil
}

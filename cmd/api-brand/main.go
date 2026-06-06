package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/database"
	"github.com/zkw/mini-schedule/backend/migrations"

	_ "github.com/zkw/mini-schedule/backend/docs/brand" // swagger docs
)

// @title 品牌商家后台 API
// @version 1.0
// @description 品牌商家管理员接口文档
// @host localhost:8081
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	// 加载配置
	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logLevel := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: logLevel}
	var handler slog.Handler
	if cfg.Log.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	log := slog.New(handler)

	// Batch 4.5：按 config 开关在 Wire 之前应用 migration up。
	// 多 cmd 同时启动靠 Postgres advisory lock 互斥；后到的看到 schema 已就绪直接 no-op。
	if cfg.Database.AutoMigrateOnBoot {
		if err := database.RunMigrationsUp(cfg.Database.DSN(), migrations.FS, log); err != nil {
			log.Error("migrations failed; aborting boot to avoid running on broken schema",
				slog.Any("error", err))
			os.Exit(1)
		}
	}

	// Wire 注入
	app, cleanup, err := initializeBrandApp(cfg, log)
	if err != nil {
		log.Error("Failed to initialize application", slog.Any("error", err))
		os.Exit(1)
	}
	defer cleanup()

	// Swagger UI（仅 debug 模式）
	if cfg.App.Debug {
		app.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// gin 用于类型推断（避免 imported and not used）
	_ = gin.Mode()

	// HTTP 服务器
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      app,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// 优雅关闭
	go func() {
		log.Info("brand server starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("brand server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("brand server shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("brand server forced shutdown", slog.Any("error", err))
		os.Exit(1)
	}

	log.Info("brand server exited")
}

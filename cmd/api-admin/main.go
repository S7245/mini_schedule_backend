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
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/database"
	"github.com/zkw/mini-schedule/backend/migrations"

	_ "github.com/zkw/mini-schedule/backend/docs/admin" // swagger docs
)

// @title 平台管理后台 API
// @version 1.0
// @description 平台管理员接口文档
// @host localhost:8083
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

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

	// Batch 4.5：auto-apply migration up before Wire（同 api-brand 注释）。
	if cfg.Database.AutoMigrateOnBoot {
		if err := database.RunMigrationsUp(cfg.Database.DSN(), migrations.FS, log); err != nil {
			log.Error("migrations failed; aborting boot to avoid running on broken schema",
				slog.Any("error", err))
			os.Exit(1)
		}
	}

	app, cleanup, err := initializeAdminApp(cfg, log)
	if err != nil {
		log.Error("Failed to initialize application", slog.Any("error", err))
		os.Exit(1)
	}
	defer cleanup()

	// gin 用于类型推断（避免 imported and not used）
	_ = gin.Mode()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      app,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Info("admin server starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("admin server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("admin server shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("admin server forced shutdown", slog.Any("error", err))
		os.Exit(1)
	}

	log.Info("admin server exited")
}

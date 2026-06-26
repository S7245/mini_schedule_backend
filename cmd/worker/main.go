package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"github.com/zkw/mini-schedule/backend/internal/application/sessionautomation"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/worker"
)

// cmd/worker：场次状态自动化后台进程（Batch 15）。
//   - asynq Scheduler 按 worker.sweep_cron 周期 enqueue session:sweep；
//   - asynq Server 消费并调 sessionautomation.RunSweep。
//
// 部署铁律：Scheduler 多副本会重复 enqueue → 本进程须 replicas=1（Railway 第 4 服务）。
// 本进程不跑 migration（schema 由 api-* 服务负责迁移，避免多进程竞争）。
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

	log := newLogger(cfg)

	// 手动 DI（图仅 3 节点：db → bookingRepo → service）。
	db, err := persistence.NewDatabase(&cfg.Database, log)
	if err != nil {
		log.Error("failed to connect database", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if sqlDB, derr := db.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	}()

	svc := sessionautomation.NewService(persistence.NewBookingRepository(db), log)
	sweep := worker.NewSweepHandler(svc, log)

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}

	concurrency := cfg.Worker.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	sweepCron := cfg.Worker.SweepCron
	if sweepCron == "" {
		sweepCron = "@every 1m"
	}

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: concurrency,
		LogLevel:    asynq.InfoLevel,
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(worker.TaskSessionSweep, sweep.Handle)

	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{LogLevel: asynq.InfoLevel})
	if _, err := scheduler.Register(sweepCron, worker.NewSweepTask()); err != nil {
		log.Error("failed to register sweep schedule", slog.String("cron", sweepCron), slog.Any("error", err))
		os.Exit(1)
	}

	if err := srv.Start(mux); err != nil {
		log.Error("failed to start asynq server", slog.Any("error", err))
		os.Exit(1)
	}
	if err := scheduler.Start(); err != nil {
		log.Error("failed to start asynq scheduler", slog.Any("error", err))
		srv.Shutdown() // server 已起，优雅收尾再退出
		os.Exit(1)
	}
	log.Info("worker started", slog.String("sweep_cron", sweepCron), slog.Int("concurrency", concurrency))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("worker shutting down...")
	scheduler.Shutdown() // 先停生产（不再 enqueue）
	srv.Shutdown()       // 再停消费（等在途任务完成）
	log.Info("worker exited")
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	if cfg.Log.Format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

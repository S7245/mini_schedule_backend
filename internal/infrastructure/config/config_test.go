package config

import (
	"path/filepath"
	"testing"
)

// TestLoad_WorkerSection 验证 Batch 15 新增的 worker 段从 yaml 正确加载。
func TestLoad_WorkerSection(t *testing.T) {
	path := filepath.Join("..", "..", "..", "configs", "config-app.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}
	if cfg.Worker.SweepCron != "@every 1m" {
		t.Errorf("Worker.SweepCron = %q, want %q", cfg.Worker.SweepCron, "@every 1m")
	}
	if cfg.Worker.Concurrency != 4 {
		t.Errorf("Worker.Concurrency = %d, want 4", cfg.Worker.Concurrency)
	}
}

// TestLoad_WorkerEnvOverride 验证 MINI_SCHEDULE_WORKER_SWEEP_CRON 可覆盖 yaml。
func TestLoad_WorkerEnvOverride(t *testing.T) {
	t.Setenv("MINI_SCHEDULE_WORKER_SWEEP_CRON", "@every 30s")
	path := filepath.Join("..", "..", "..", "configs", "config-app.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}
	if cfg.Worker.SweepCron != "@every 30s" {
		t.Errorf("Worker.SweepCron = %q, want env override %q", cfg.Worker.SweepCron, "@every 30s")
	}
}

// TestLoad_WorkerSubscriptionFields 验证 Batch 16 新增 worker.subscription_sweep_cron / grace_days 从 yaml 加载。
func TestLoad_WorkerSubscriptionFields(t *testing.T) {
	path := filepath.Join("..", "..", "..", "configs", "config-app.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}
	if cfg.Worker.SubscriptionSweepCron != "@every 1h" {
		t.Errorf("Worker.SubscriptionSweepCron = %q, want %q", cfg.Worker.SubscriptionSweepCron, "@every 1h")
	}
	if cfg.Worker.GraceDays != 7 {
		t.Errorf("Worker.GraceDays = %d, want 7", cfg.Worker.GraceDays)
	}
}

// TestLoad_WorkerSubscriptionEnvOverride 验证 Batch 16 两个 worker env 覆盖（含 int 字段 grace_days）。
func TestLoad_WorkerSubscriptionEnvOverride(t *testing.T) {
	t.Setenv("MINI_SCHEDULE_WORKER_SUBSCRIPTION_SWEEP_CRON", "@every 5s")
	t.Setenv("MINI_SCHEDULE_WORKER_GRACE_DAYS", "3")
	path := filepath.Join("..", "..", "..", "configs", "config-app.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}
	if cfg.Worker.SubscriptionSweepCron != "@every 5s" {
		t.Errorf("Worker.SubscriptionSweepCron = %q, want env override %q", cfg.Worker.SubscriptionSweepCron, "@every 5s")
	}
	if cfg.Worker.GraceDays != 3 {
		t.Errorf("Worker.GraceDays = %d, want env override 3", cfg.Worker.GraceDays)
	}
}

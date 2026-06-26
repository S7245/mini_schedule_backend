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

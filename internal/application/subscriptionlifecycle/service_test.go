package subscriptionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

type call struct {
	name string
	id   int64
}

// scriptedRepo 脚本化 fake：List 返回固定 id 列表，转换按 id 返回预设 (ok,err)。
// 用于断言 RunSweep 的计数 / 两段编排顺序 / systemic 错误传播 / graceDays 透传，
// 不依赖真实状态机（真实自愈见 persistence 包的 DB 集成测试）。
type scriptedRepo struct {
	dueGrace         []int64
	dueGraceErr      error
	graceOK          map[int64]bool
	graceErr         map[int64]error
	dueRestricted    []int64
	dueRestrictedErr error
	restrictedOK     map[int64]bool
	restrictedErr    map[int64]error

	calls        []call
	gotGraceDays int
}

func (r *scriptedRepo) ListSubscriptionsDueForGrace(_ context.Context, _ time.Time) ([]int64, error) {
	r.calls = append(r.calls, call{"listGrace", 0})
	return r.dueGrace, r.dueGraceErr
}

func (r *scriptedRepo) TransitionSubscriptionToGrace(_ context.Context, id int64, _ time.Time, graceDays int) (bool, error) {
	r.calls = append(r.calls, call{"grace", id})
	r.gotGraceDays = graceDays
	if e := r.graceErr[id]; e != nil {
		return false, e
	}
	return r.graceOK[id], nil
}

func (r *scriptedRepo) ListSubscriptionsDueForRestricted(_ context.Context, _ time.Time) ([]int64, error) {
	r.calls = append(r.calls, call{"listRestricted", 0})
	return r.dueRestricted, r.dueRestrictedErr
}

func (r *scriptedRepo) TransitionSubscriptionToRestricted(_ context.Context, id int64, _ time.Time) (bool, error) {
	r.calls = append(r.calls, call{"restricted", id})
	if e := r.restrictedErr[id]; e != nil {
		return false, e
	}
	return r.restrictedOK[id], nil
}

func TestRunSweep_Counts(t *testing.T) {
	repo := &scriptedRepo{
		dueGrace:      []int64{1, 2, 3},
		graceOK:       map[int64]bool{1: true, 2: false},
		graceErr:      map[int64]error{3: errors.New("boom-grace")},
		dueRestricted: []int64{4, 5, 6},
		restrictedOK:  map[int64]bool{4: true, 5: false},
		restrictedErr: map[int64]error{6: errors.New("boom-restricted")},
	}
	svc := NewService(repo, 7, nil)
	sum, err := svc.RunSweep(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	if sum.Graced != 1 {
		t.Errorf("Graced = %d, want 1", sum.Graced)
	}
	if sum.Restricted != 1 {
		t.Errorf("Restricted = %d, want 1", sum.Restricted)
	}
	if sum.Skipped != 2 { // id2 (grace false) + id5 (restricted false)
		t.Errorf("Skipped = %d, want 2", sum.Skipped)
	}
	if sum.Failed != 2 { // id3 (grace err) + id6 (restricted err)
		t.Errorf("Failed = %d, want 2", sum.Failed)
	}
}

func TestRunSweep_PhaseOrdering(t *testing.T) {
	repo := &scriptedRepo{
		dueGrace:      []int64{1},
		graceOK:       map[int64]bool{1: true},
		dueRestricted: []int64{2},
		restrictedOK:  map[int64]bool{2: true},
	}
	svc := NewService(repo, 7, nil)
	if _, err := svc.RunSweep(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	want := []call{{"listGrace", 0}, {"grace", 1}, {"listRestricted", 0}, {"restricted", 2}}
	if len(repo.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", repo.calls, want)
	}
	for i := range want {
		if repo.calls[i] != want[i] {
			t.Errorf("call[%d] = %v, want %v (phase1 须整体先于 phase2)", i, repo.calls[i], want[i])
		}
	}
}

// 同一 sub 在一轮内 active→grace→restricted（编排层面：phase2 处理 phase1 刚翻的 id）。
func TestRunSweep_SelfHealOneRound(t *testing.T) {
	repo := &scriptedRepo{
		dueGrace:      []int64{1},
		graceOK:       map[int64]bool{1: true},
		dueRestricted: []int64{1}, // 真实 repo 在 phase1 翻 grace（grace_ends_at≤now）后会扫到它
		restrictedOK:  map[int64]bool{1: true},
	}
	svc := NewService(repo, 0, nil)
	sum, err := svc.RunSweep(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	if sum.Graced != 1 || sum.Restricted != 1 {
		t.Errorf("summary = %+v, want Graced=1 Restricted=1", sum)
	}
}

func TestRunSweep_SystemicGraceErrorAbortsPhase2(t *testing.T) {
	repo := &scriptedRepo{dueGraceErr: errors.New("db down")}
	svc := NewService(repo, 7, nil)
	_, err := svc.RunSweep(context.Background(), time.Now().UTC())
	if err == nil {
		t.Fatal("expected systemic error from phase1 List")
	}
	for _, c := range repo.calls {
		if c.name == "listRestricted" {
			t.Error("phase2 不应在 phase1 systemic 失败后运行")
		}
	}
}

func TestRunSweep_SystemicRestrictedErrorReturns(t *testing.T) {
	repo := &scriptedRepo{dueRestrictedErr: errors.New("db down")}
	svc := NewService(repo, 7, nil)
	if _, err := svc.RunSweep(context.Background(), time.Now().UTC()); err == nil {
		t.Fatal("expected systemic error from phase2 List")
	}
}

func TestNewService_GraceDaysDefault(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 7}, {-3, 7}, {5, 5}, {14, 14},
	}
	for _, c := range cases {
		repo := &scriptedRepo{dueGrace: []int64{1}, graceOK: map[int64]bool{1: true}}
		svc := NewService(repo, c.in, nil)
		if _, err := svc.RunSweep(context.Background(), time.Now().UTC()); err != nil {
			t.Fatalf("RunSweep: %v", err)
		}
		if repo.gotGraceDays != c.want {
			t.Errorf("NewService(graceDays=%d): transition got %d, want %d", c.in, repo.gotGraceDays, c.want)
		}
	}
}

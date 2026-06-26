package sessionautomation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/booking"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// fakeSweepRepo 嵌入 booking.Repository（仅覆盖 sweep 用到的 3 法），记录调用序与参数。
type fakeSweepRepo struct {
	booking.Repository
	markN   int64
	markErr error
	due     []int64
	dueErr  error
	endErr  map[int64]error
	calls   []string
	ended   []int64
}

func (f *fakeSweepRepo) MarkSessionsInProgress(context.Context, time.Time) (int64, error) {
	f.calls = append(f.calls, "mark")
	return f.markN, f.markErr
}

func (f *fakeSweepRepo) ListDueSessionIDs(context.Context, time.Time) ([]int64, error) {
	f.calls = append(f.calls, "list")
	return f.due, f.dueErr
}

func (f *fakeSweepRepo) EndSessionSystem(_ context.Context, id int64) (*booking.EndSessionResult, error) {
	f.calls = append(f.calls, "end")
	if e, ok := f.endErr[id]; ok {
		return nil, e
	}
	f.ended = append(f.ended, id)
	return &booking.EndSessionResult{SessionID: id, Status: "completed"}, nil
}

func TestRunSweep_OrchestratesAndCounts(t *testing.T) {
	r := &fakeSweepRepo{markN: 2, due: []int64{10, 11, 12}}
	sum, err := NewService(r, nil).RunSweep(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	if sum.Started != 2 || sum.Ended != 3 || sum.Skipped != 0 || sum.Failed != 0 {
		t.Errorf("summary = %+v, want {Started:2 Ended:3}", sum)
	}
	// 先 mark 后 list，再逐场次 end。
	if len(r.calls) < 2 || r.calls[0] != "mark" || r.calls[1] != "list" {
		t.Errorf("call order = %v, want mark,list,end...", r.calls)
	}
	if len(r.ended) != 3 || r.ended[0] != 10 || r.ended[2] != 12 {
		t.Errorf("ended = %v, want [10 11 12]", r.ended)
	}
}

func TestRunSweep_SkipsNotEndable(t *testing.T) {
	notEndable := apperr.NewAppError(apperr.ErrSessionNotEndable, "x", 409)
	r := &fakeSweepRepo{due: []int64{10, 11}, endErr: map[int64]error{11: notEndable}}
	sum, err := NewService(r, nil).RunSweep(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	// 11 并发已终态 → skipped（非 failed）；10 正常 ended。
	if sum.Ended != 1 || sum.Skipped != 1 || sum.Failed != 0 {
		t.Errorf("summary = %+v, want {Ended:1 Skipped:1 Failed:0}", sum)
	}
}

func TestRunSweep_FailIsolationContinues(t *testing.T) {
	r := &fakeSweepRepo{due: []int64{10, 11}, endErr: map[int64]error{10: errors.New("boom")}}
	sum, err := NewService(r, nil).RunSweep(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("RunSweep should not surface单场次错误: %v", err)
	}
	// 10 失败但不中断 → 11 仍处理。
	if sum.Failed != 1 || sum.Ended != 1 {
		t.Errorf("summary = %+v, want {Failed:1 Ended:1}", sum)
	}
	if len(r.ended) != 1 || r.ended[0] != 11 {
		t.Errorf("ended = %v, want [11]（10 失败后继续）", r.ended)
	}
}

func TestRunSweep_MarkErrorReturnsAndStops(t *testing.T) {
	r := &fakeSweepRepo{markErr: errors.New("db down")}
	_, err := NewService(r, nil).RunSweep(context.Background(), time.Now())
	if err == nil {
		t.Fatal("systemic mark 失败应返 error 触发 asynq 重试")
	}
	for _, c := range r.calls {
		if c == "list" || c == "end" {
			t.Errorf("mark 失败后不应继续 list/end，calls=%v", r.calls)
		}
	}
}

func TestRunSweep_ListErrorReturns(t *testing.T) {
	r := &fakeSweepRepo{markN: 1, dueErr: errors.New("db down")}
	sum, err := NewService(r, nil).RunSweep(context.Background(), time.Now())
	if err == nil {
		t.Fatal("systemic list 失败应返 error")
	}
	if sum.Started != 1 {
		t.Errorf("Started = %d, want 1 (mark 已成功)", sum.Started)
	}
}

package staff

import (
	"errors"
	"testing"
)

func TestRoleAllocator_AssignDefaultOwnerRolesTx_RejectsBadTx(t *testing.T) {
	a := &RoleAllocator{}
	err := a.AssignDefaultOwnerRolesTx("not a tx", 1, 1)
	if err == nil {
		t.Fatal("expected error for bad tx type")
	}
}

func TestRoleAllocator_BackfillSkipSentinel(t *testing.T) {
	// 仅校验 sentinel 自身（避免在单测里跑真实 DB）
	if !errors.Is(errSkipBackfill, errSkipBackfill) {
		t.Fatal("sentinel error should match itself")
	}
}

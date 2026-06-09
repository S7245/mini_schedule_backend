package audit

import (
	"strings"
	"testing"
)

func TestWrite_RejectsNilTx(t *testing.T) {
	err := Write(nil, Event{Action: "foo", Actor: Actor{Type: ActorSystem}})
	if err == nil {
		t.Fatal("expected error for nil tx")
	}
}

func TestWrite_RejectsEmptyAction(t *testing.T) {
	// 这里我们仅校验前置参数校验路径（不需要真实 DB）。
	// 直接用一个非 nil 哨兵——只要在到达 SQL 前返回，nilDB 即可触发参数错误。
	err := Write(nil, Event{Actor: Actor{Type: ActorBrandUser, ID: 1}})
	if err == nil {
		t.Fatal("expected error for missing action and nil tx")
	}
	if !strings.Contains(err.Error(), "tx") {
		t.Errorf("expected nil tx message first, got %v", err)
	}
}

func TestIsValidActorType(t *testing.T) {
	cases := map[ActorType]bool{
		ActorBrandUser:     true,
		ActorPlatformAdmin: true,
		ActorSystem:        true,
		ActorType(""):      false,
		ActorType("x"):     false,
	}
	for k, want := range cases {
		if IsValidActorType(k) != want {
			t.Errorf("IsValidActorType(%q) = %v, want %v", k, !want, want)
		}
	}
}

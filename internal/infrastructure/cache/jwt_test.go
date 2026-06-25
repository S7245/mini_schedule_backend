package cache

import (
	"testing"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
)

func testJWTService() *Service {
	return NewService(&config.JWTConfig{
		Secret:        "test-secret",
		Expire:        time.Hour,
		RefreshExpire: 24 * time.Hour,
	})
}

// TestTokenPayload_ProfileIDRoundTrip 验证 brand_learner_profile_id 经 generate→parse 往返保留（Batch 14a 桥接）。
func TestTokenPayload_ProfileIDRoundTrip(t *testing.T) {
	s := testJWTService()
	in := TokenPayload{UserID: 7, BrandID: 21, UserType: "app", ProfileID: 99}

	tok, err := s.GenerateToken(in)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	out, err := s.ParseToken(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.ProfileID != 99 {
		t.Errorf("ProfileID = %d, want 99", out.ProfileID)
	}
	if out.UserID != 7 || out.BrandID != 21 || out.UserType != "app" {
		t.Errorf("payload mismatch: %+v", out)
	}
}

// TestTokenPayload_NoProfileID 旧 token（无 profile_id）解析后 ProfileID 为 0（业务端点据此判需重登）。
func TestTokenPayload_NoProfileID(t *testing.T) {
	s := testJWTService()
	tok, err := s.GenerateToken(TokenPayload{UserID: 1, BrandID: 21, UserType: "app"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	out, err := s.ParseToken(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.ProfileID != 0 {
		t.Errorf("ProfileID = %d, want 0", out.ProfileID)
	}
}

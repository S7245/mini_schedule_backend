package validation

import (
	"strings"
	"testing"

	"github.com/zkw/mini-schedule/backend/pkg/i18n"
)

func TestTranslateUsesJSONFieldNames(t *testing.T) {
	type loginRequest struct {
		Username string `json:"username" validate:"required"`
		Password string `json:"password" validate:"required,min=6"`
	}

	v := New()
	err := v.Struct(loginRequest{Password: "123"})
	if err == nil {
		t.Fatal("Validator.Struct(loginRequest) error = nil, want validation error")
	}

	got := v.Translate(i18n.LocaleEnUS, err)
	if !strings.Contains(got, "username") {
		t.Errorf("Translate(en-US) = %q, want field name %q", got, "username")
	}
	if !strings.Contains(got, "required") {
		t.Errorf("Translate(en-US) = %q, want required message", got)
	}
	if strings.Contains(got, "loginRequest") || strings.Contains(got, "Username") {
		t.Errorf("Translate(en-US) = %q, should use JSON field names", got)
	}
}

func TestTranslateUsesChinese(t *testing.T) {
	type createRequest struct {
		Name string `json:"name" validate:"required"`
	}

	v := New()
	err := v.Struct(createRequest{})
	if err == nil {
		t.Fatal("Validator.Struct(createRequest) error = nil, want validation error")
	}

	got := v.Translate(i18n.LocaleZhCN, err)
	if !strings.Contains(got, "name") {
		t.Errorf("Translate(zh-CN) = %q, want field name %q", got, "name")
	}
	if !strings.Contains(got, "必填") && !strings.Contains(got, "必需") {
		t.Errorf("Translate(zh-CN) = %q, want required message", got)
	}
}

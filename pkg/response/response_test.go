package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

func TestErrorLocalizesAppError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		Error(c, apperr.ErrUnauthorizedF("缺少认证令牌"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var got Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Error status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got.Code != string(apperr.ErrUnauthorized) {
		t.Errorf("Error code = %q, want %q", got.Code, apperr.ErrUnauthorized)
	}
	if got.Message != "missing authentication token" {
		t.Errorf("Error message = %q, want %q", got.Message, "missing authentication token")
	}
}

func TestSuccessLocalizesMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		SuccessNoData(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/?lang=zh-CN", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var got Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}

	if got.Message != "成功" {
		t.Errorf("SuccessNoData() message = %q, want %q", got.Message, "成功")
	}
}

func TestErrorPreservesDynamicBadRequestMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		Error(c, apperr.ErrBadRequest("username is a required field"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var got Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}

	if got.Message != "username is a required field" {
		t.Errorf("Error message = %q, want %q", got.Message, "username is a required field")
	}
}

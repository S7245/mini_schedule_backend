package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

func accessTokenCookieName(userType string) string {
	switch userType {
	case "admin":
		return "admin_access_token"
	case "brand":
		return "brand_access_token"
	case "app":
		return "app_access_token"
	default:
		return ""
	}
}

func tokenFromRequest(c *gin.Context, userType string) string {
	if cookieName := accessTokenCookieName(userType); cookieName != "" {
		if token, err := c.Cookie(cookieName); err == nil && token != "" {
			return token
		}
	}

	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return ""
	}

	return tokenString
}

// JWTAuth JWT 认证中间件
func JWTAuth(jwtSvc *cache.Service, userType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := tokenFromRequest(c, userType)
		if tokenString == "" {
			response.Error(c, apperr.ErrUnauthorizedF("缺少认证令牌"))
			c.Abort()
			return
		}

		payload, err := jwtSvc.ParseToken(tokenString)
		if err != nil {
			response.Error(c, apperr.ErrUnauthorizedF("认证令牌无效或已过期"))
			c.Abort()
			return
		}

		if payload.UserType != userType {
			response.Error(c, apperr.ErrForbiddenF("无权访问该接口"))
			c.Abort()
			return
		}

		// 将用户信息注入上下文
		c.Set("user_id", payload.UserID)
		c.Set("brand_id", payload.BrandID)
		c.Set("user_type", payload.UserType)
		c.Set("profile_id", payload.ProfileID) // C 端学员 brand_learner_profile_id（Batch 14a 桥接）；非学员为 0。

		c.Next()
	}
}

// GetUserID 从上下文获取用户 ID
func GetUserID(c *gin.Context) int64 {
	if v, exists := c.Get("user_id"); exists {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

// GetBrandID 从上下文获取品牌 ID
func GetBrandID(c *gin.Context) int64 {
	if v, exists := c.Get("brand_id"); exists {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

// GetProfileID 从上下文获取 C 端学员 brand_learner_profile_id（Batch 14a）；0 = 未桥接（旧 token / 非学员）。
func GetProfileID(c *gin.Context) int64 {
	if v, exists := c.Get("profile_id"); exists {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

package cache

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
)

// TokenPayload JWT payload 结构
type TokenPayload struct {
	UserID   int64  `json:"user_id"`
	BrandID  int64  `json:"brand_id,omitempty"`
	UserType string `json:"user_type"` // "brand" / "app" / "admin"
}

// Service JWT 服务
type Service struct {
	secret        []byte
	expire        time.Duration
	refreshExpire time.Duration
}

// NewService 创建 JWT 服务
func NewService(cfg *config.JWTConfig) *Service {
	return &Service{
		secret:        []byte(cfg.Secret),
		expire:        cfg.Expire,
		refreshExpire: cfg.RefreshExpire,
	}
}

// GenerateToken 生成访问令牌
func (s *Service) GenerateToken(payload TokenPayload) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":   payload.UserID,
		"brand_id":  payload.BrandID,
		"user_type": payload.UserType,
		"iat":       now.Unix(),
		"exp":       now.Add(s.expire).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// AccessTokenMaxAge 返回访问令牌 Cookie 的有效秒数
func (s *Service) AccessTokenMaxAge() int {
	return int(s.expire.Seconds())
}

// GenerateRefreshToken 生成刷新令牌
func (s *Service) GenerateRefreshToken(payload TokenPayload) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":   payload.UserID,
		"brand_id":  payload.BrandID,
		"user_type": payload.UserType,
		"type":      "refresh",
		"iat":       now.Unix(),
		"exp":       now.Add(s.refreshExpire).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// RefreshTokenMaxAge 返回刷新令牌 Cookie 的有效秒数
func (s *Service) RefreshTokenMaxAge() int {
	return int(s.refreshExpire.Seconds())
}

// ParseToken 解析并验证令牌
func (s *Service) ParseToken(tokenString string) (*TokenPayload, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	payload := &TokenPayload{}
	if userID, ok := claims["user_id"].(float64); ok {
		payload.UserID = int64(userID)
	}
	if brandID, ok := claims["brand_id"].(float64); ok {
		payload.BrandID = int64(brandID)
	}
	if userType, ok := claims["user_type"].(string); ok {
		payload.UserType = userType
	}

	return payload, nil
}

// IsTokenExpired 判断令牌是否过期（通过解析错误判断）
func (s *Service) IsTokenExpired(tokenString string) bool {
	_, err := s.ParseToken(tokenString)
	if err != nil {
		return true
	}
	return false
}

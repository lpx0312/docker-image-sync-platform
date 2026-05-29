package services

import (
	"fmt"
	"time"

	"docker-image-sync-platform/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Claims JWT 声明
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	RoleID   uint   `json:"role_id"`
	RoleCode string `json:"role_code"`
	jwt.RegisteredClaims
}

// AuthService 认证服务
type AuthService struct{}

// NewAuthService 创建认证服务
func NewAuthService() *AuthService {
	return &AuthService{}
}

// GenerateToken 生成 JWT Token
func (s *AuthService) GenerateToken(userID uint, username string, roleID uint, roleCode string, rememberMe bool) (string, time.Time, error) {
	secret := config.AppConfig.Auth.JWTSecret
	if secret == "" {
		secret = "docker-sync-platform-jwt-secret-change-me"
	}

	expiryStr := config.AppConfig.Auth.TokenExpiry
	if rememberMe {
		expiryStr = config.AppConfig.Auth.RememberMeExpiry
	}
	if expiryStr == "" {
		if rememberMe {
			expiryStr = "168h"
		} else {
			expiryStr = "24h"
		}
	}

	expiry, err := time.ParseDuration(expiryStr)
	if err != nil {
		expiry = 24 * time.Hour
	}

	expiresAt := time.Now().Add(expiry)

	claims := &Claims{
		UserID:   userID,
		Username: username,
		RoleID:   roleID,
		RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "docker-sync-platform",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("生成Token失败: %w", err)
	}

	return tokenString, expiresAt, nil
}

// ValidateToken 验证并解析 JWT Token
func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	secret := config.AppConfig.Auth.JWTSecret
	if secret == "" {
		secret = "docker-sync-platform-jwt-secret-change-me"
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名方法: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("Token无效: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("Token解析失败")
	}

	return claims, nil
}

// HashPassword 加密密码
func (s *AuthService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("密码加密失败: %w", err)
	}
	return string(hash), nil
}

// CheckPassword 校验密码
func (s *AuthService) CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

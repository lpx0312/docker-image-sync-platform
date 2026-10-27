package services

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"docker-image-sync-platform/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken(t *testing.T) {
	withAppConfig(t, &config.Config{
		Auth: config.AuthConfig{JWTSecret: "unit-test-secret"},
	})
	svc := NewAuthService()

	token, expiresAt, err := svc.GenerateToken(42, "alice", 7, "admin", false)
	require.NoError(t, err)
	assert.True(t, expiresAt.After(time.Now()))
	assert.False(t, expiresAt.After(time.Now().Add(25*time.Hour)), "默认有效期约 24h")

	claims, err := svc.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, uint(42), claims.UserID)
	assert.Equal(t, "alice", claims.Username)
	assert.Equal(t, uint(7), claims.RoleID)
	assert.Equal(t, "admin", claims.RoleCode)
	assert.Equal(t, "docker-sync-platform", claims.Issuer)
}

func TestGenerateTokenRememberMe(t *testing.T) {
	withAppConfig(t, &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:        "s",
			RememberMeExpiry: "2h",
		},
	})
	svc := NewAuthService()
	_, expiresAt, err := svc.GenerateToken(1, "u", 1, "user", true)
	require.NoError(t, err)
	assert.True(t, expiresAt.After(time.Now().Add(time.Hour)))
	assert.True(t, expiresAt.Before(time.Now().Add(3*time.Hour)))
}

func TestGenerateTokenDefaults(t *testing.T) {
	t.Run("空secret与空expiry使用默认", func(t *testing.T) {
		withAppConfig(t, &config.Config{})
		svc := NewAuthService()
		token, _, err := svc.GenerateToken(1, "u", 1, "user", false)
		require.NoError(t, err)

		// 默认 secret 可解析
		claims, err := svc.ValidateToken(token)
		require.NoError(t, err)
		assert.Equal(t, "u", claims.Username)
	})

	t.Run("非法duration回退24h", func(t *testing.T) {
		withAppConfig(t, &config.Config{
			Auth: config.AuthConfig{TokenExpiry: "not-a-duration"},
		})
		svc := NewAuthService()
		_, expiresAt, err := svc.GenerateToken(1, "u", 1, "user", false)
		require.NoError(t, err)
		assert.True(t, expiresAt.After(time.Now().Add(23*time.Hour)))
	})

	t.Run("rememberMe空expiry默认168h", func(t *testing.T) {
		withAppConfig(t, &config.Config{})
		svc := NewAuthService()
		_, expiresAt, err := svc.GenerateToken(1, "u", 1, "user", true)
		require.NoError(t, err)
		assert.True(t, expiresAt.After(time.Now().Add(167*time.Hour)))
	})
}

func TestValidateTokenFailures(t *testing.T) {
	withAppConfig(t, &config.Config{Auth: config.AuthConfig{JWTSecret: "secret-A"}})
	svc := NewAuthService()

	t.Run("垃圾字符串", func(t *testing.T) {
		_, err := svc.ValidateToken("not-a-jwt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Token无效")
	})

	t.Run("过期token", func(t *testing.T) {
		claims := &Claims{
			UserID: 1, Username: "u", RoleID: 1, RoleCode: "user",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte("secret-A"))
		require.NoError(t, err)
		_, err = svc.ValidateToken(signed)
		require.Error(t, err)
	})

	t.Run("签名密钥不匹配", func(t *testing.T) {
		token, _, err := svc.GenerateToken(1, "u", 1, "user", false)
		require.NoError(t, err)
		withAppConfig(t, &config.Config{Auth: config.AuthConfig{JWTSecret: "secret-B"}})
		_, err = svc.ValidateToken(token)
		require.Error(t, err)
	})

	t.Run("不支持的签名方法RS256", func(t *testing.T) {
		withAppConfig(t, &config.Config{Auth: config.AuthConfig{JWTSecret: "secret-A"}})
		key, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)
		claims := &Claims{
			UserID: 1, Username: "u", RoleID: 1, RoleCode: "user",
			RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		signed, err := token.SignedString(key)
		require.NoError(t, err)
		_, err = svc.ValidateToken(signed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Token无效")
	})
}

func TestHashAndCheckPassword(t *testing.T) {
	svc := NewAuthService()

	hash, err := svc.HashPassword("s3cret!")
	require.NoError(t, err)
	assert.NotEqual(t, "s3cret!", hash)
	assert.True(t, svc.CheckPassword(hash, "s3cret!"))
	assert.False(t, svc.CheckPassword(hash, "wrong"))
	assert.False(t, svc.CheckPassword("not-a-hash", "s3cret!"))
}

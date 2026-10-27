package database

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================
// encryptSensitiveValue / decryptAESGCM / encryptAESGCM / tryDecryptAESGCM
// 单元测试：AES-256-GCM 加解密往返与各类畸形输入
// ====================================================================

func TestEncryptSensitiveValue_EmptyInput(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("GIN_MODE", "")

	got, err := encryptSensitiveValue("")
	require.NoError(t, err)
	assert.Empty(t, got, "空明文应原样返回空串（不产生 ENC: 前缀）")
}

func TestEncryptSensitiveValue_WithEnvKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "unit-test-key")
	t.Setenv("GIN_MODE", "debug")

	ciphertext, err := encryptSensitiveValue("secret-value")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(ciphertext, "ENC:"), "密文应带 ENC: 前缀, got %q", ciphertext)

	// 用相同派生密钥可解回明文
	plaintext, err := decryptAESGCM(ciphertext, deriveKey("unit-test-key"))
	require.NoError(t, err)
	assert.Equal(t, "secret-value", plaintext)

	// 随机 nonce：两次加密同一明文应产生不同密文
	ciphertext2, err := encryptSensitiveValue("secret-value")
	require.NoError(t, err)
	assert.NotEqual(t, ciphertext, ciphertext2, "随机 nonce 应使两次加密结果不同")
}

func TestEncryptSensitiveValue_DefaultKey(t *testing.T) {
	// 未设置 ENCRYPTION_KEY 且非 release 模式 -> 回退到内置默认密钥
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("GIN_MODE", "debug")

	ciphertext, err := encryptSensitiveValue("plain")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(ciphertext, "ENC:"))

	defaultKey := deriveKey("docker-sync-platform-default-key-2024")
	plaintext, err := decryptAESGCM(ciphertext, defaultKey)
	require.NoError(t, err)
	assert.Equal(t, "plain", plaintext)

	// 环境变量密钥与默认密钥互不通用
	_, err = decryptAESGCM(ciphertext, deriveKey("some-other-key"))
	assert.Error(t, err, "错误密钥不应解出默认密钥的密文")
}

func TestEncryptSensitiveValue_ReleaseModeWithoutKey(t *testing.T) {
	// 生产模式（GIN_MODE=release）且未设置 ENCRYPTION_KEY 时必须报错
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("GIN_MODE", "release")

	_, err := encryptSensitiveValue("secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "生产环境必须设置 ENCRYPTION_KEY")
}

func TestDecryptAESGCM_NonEncPassthrough(t *testing.T) {
	key := deriveKey("any-key")
	cases := []string{
		"plaintext",
		"",        // 空串同样透传
		"ENC",     // 缺冒号不算 ENC 前缀
		"ENCX:xx", // 前缀不匹配
		"base64-not-prefixed",
	}
	for _, c := range cases {
		got, err := decryptAESGCM(c, key)
		require.NoError(t, err, "无 ENC: 前缀的输入应原样透传: %q", c)
		assert.Equal(t, c, got)
	}
}

func TestDecryptAESGCM_MalformedBase64(t *testing.T) {
	_, err := decryptAESGCM("ENC:!!!not-valid-base64!!!", deriveKey("k"))
	require.Error(t, err, "非法 base64 应报错")
}

func TestDecryptAESGCM_CiphertextTooShort(t *testing.T) {
	// 合法 base64 但解码后不足 12 字节 nonce
	_, err := decryptAESGCM("ENC:AAAA", deriveKey("k"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestDecryptAESGCM_WrongKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "right-key")
	ciphertext, err := encryptSensitiveValue("payload")
	require.NoError(t, err)

	_, err = decryptAESGCM(ciphertext, deriveKey("wrong-key"))
	require.Error(t, err, "GCM 认证失败应报错（密钥错误）")
}

func TestDecryptAESGCM_InvalidKeySize(t *testing.T) {
	// AES 密钥长度非法（非 16/24/32 字节）
	ciphertext, err := encryptAESGCM("x", deriveKey("k"))
	require.NoError(t, err)

	_, err = decryptAESGCM(ciphertext, []byte("short-key"))
	require.Error(t, err, "非法密钥长度应报错")
}

func TestEncryptAESGCM_RoundTrip(t *testing.T) {
	key := deriveKey("roundtrip")
	ciphertext, err := encryptAESGCM("hello 世界", key)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ciphertext, "ENC:"))

	plaintext, err := decryptAESGCM(ciphertext, key)
	require.NoError(t, err)
	assert.Equal(t, "hello 世界", plaintext)
}

func TestEncryptAESGCM_InvalidKeySize(t *testing.T) {
	_, err := encryptAESGCM("x", []byte("10bytekey!"))
	require.Error(t, err, "非法密钥长度应报错")
}

func TestTryDecryptAESGCM(t *testing.T) {
	key := deriveKey("try-key")
	ciphertext, err := encryptAESGCM("data", key)
	require.NoError(t, err)

	assert.True(t, tryDecryptAESGCM(ciphertext, key), "正确密钥应返回 true")
	assert.False(t, tryDecryptAESGCM(ciphertext, deriveKey("other")), "错误密钥应返回 false")
	assert.False(t, tryDecryptAESGCM("ENC:####", key), "非法密文应返回 false")
}

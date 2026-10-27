package services

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEncryptionService(t *testing.T) {
	t.Run("环境变量密钥", func(t *testing.T) {
		svc := newSvcEncryptionService(t, "my-env-key")
		require.NotNil(t, svc)
		// 与环境变量密钥可往返
		enc, err := svc.Encrypt("secret")
		require.NoError(t, err)
		dec, err := svc.Decrypt(enc)
		require.NoError(t, err)
		assert.Equal(t, "secret", dec)
	})

	t.Run("默认密钥", func(t *testing.T) {
		svc := newSvcEncryptionService(t, "")
		require.NotNil(t, svc)
		enc, err := svc.Encrypt("v")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(enc, "ENC:"))
	})

	t.Run("release模式无密钥报错", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", "")
		t.Setenv("GIN_MODE", "release")
		_, err := NewEncryptionService(newSilentLogger())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ENCRYPTION_KEY")
	})
}

func TestEncryptionRoundTrip(t *testing.T) {
	svc := newSvcEncryptionService(t, "roundtrip-key")

	for _, plain := range []string{"p@ssw0rd", "中文密码🔐", strings.Repeat("x", 4096)} {
		enc, err := svc.Encrypt(plain)
		require.NoError(t, err)
		assert.True(t, svc.IsEncrypted(enc))
		assert.NotEqual(t, plain, enc)

		dec, err := svc.Decrypt(enc)
		require.NoError(t, err)
		assert.Equal(t, plain, dec)
	}

	// 随机 nonce：相同明文两次加密产生不同密文
	e1, _ := svc.Encrypt("same")
	e2, _ := svc.Encrypt("same")
	assert.NotEqual(t, e1, e2)
}

func TestEncryptEmpty(t *testing.T) {
	svc := newSvcEncryptionService(t, "k")
	_, err := svc.Encrypt("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plaintext cannot be empty")
}

func TestEncryptBadKey(t *testing.T) {
	// 同包构造非法长度密钥，触发 aes.NewCipher 失败分支
	svc := &EncryptionService{key: []byte("short"), logger: newSilentLogger()}
	_, err := svc.Encrypt("x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create cipher")
}

func TestDecryptBranches(t *testing.T) {
	svc := newSvcEncryptionService(t, "dk")

	t.Run("空串原样返回", func(t *testing.T) {
		dec, err := svc.Decrypt("")
		require.NoError(t, err)
		assert.Empty(t, dec)
	})

	t.Run("未加密数据原样返回", func(t *testing.T) {
		dec, err := svc.Decrypt("plain-value")
		require.NoError(t, err)
		assert.Equal(t, "plain-value", dec)
	})

	t.Run("畸形base64", func(t *testing.T) {
		_, err := svc.Decrypt("ENC:!!!not-base64!!!")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode base64")
	})

	t.Run("密文过短", func(t *testing.T) {
		short := base64.StdEncoding.EncodeToString([]byte("12345")) // 5 字节 < nonce 12
		_, err := svc.Decrypt("ENC:" + short)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ciphertext too short")
	})

	t.Run("密钥不匹配解密失败", func(t *testing.T) {
		other := newSvcEncryptionService(t, "another-key")
		enc, err := other.Encrypt("data")
		require.NoError(t, err)
		_, err = svc.Decrypt(enc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt")
	})

	t.Run("非法密钥长度", func(t *testing.T) {
		bad := &EncryptionService{key: []byte("short"), logger: newSilentLogger()}
		enc, err := svc.Encrypt("data")
		require.NoError(t, err)
		_, err = bad.Decrypt(enc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create cipher")
	})
}

func TestIsEncrypted(t *testing.T) {
	svc := newSvcEncryptionService(t, "k")
	assert.True(t, svc.IsEncrypted("ENC:xxxx"))
	assert.False(t, svc.IsEncrypted("plain"))
	assert.False(t, svc.IsEncrypted(""))
}

func TestEncryptIfNeeded(t *testing.T) {
	svc := newSvcEncryptionService(t, "k")

	// shouldEncrypt=false → 原样
	got, err := svc.EncryptIfNeeded("plain", false)
	require.NoError(t, err)
	assert.Equal(t, "plain", got)

	// 已加密 → 不重复加密
	enc, err := svc.Encrypt("data")
	require.NoError(t, err)
	got, err = svc.EncryptIfNeeded(enc, true)
	require.NoError(t, err)
	assert.Equal(t, enc, got)

	// 未加密且需要加密
	got, err = svc.EncryptIfNeeded("data", true)
	require.NoError(t, err)
	assert.NotEqual(t, "data", got)
	assert.True(t, svc.IsEncrypted(got))

	// 空值且需加密 → 报错（Encrypt("") 失败）
	_, err = svc.EncryptIfNeeded("", true)
	require.Error(t, err)
}

func TestDecryptIfNeeded(t *testing.T) {
	svc := newSvcEncryptionService(t, "k")

	enc, err := svc.EncryptIfNeeded("secret", true)
	require.NoError(t, err)
	dec, err := svc.DecryptIfNeeded(enc)
	require.NoError(t, err)
	assert.Equal(t, "secret", dec)

	dec, err = svc.DecryptIfNeeded("raw")
	require.NoError(t, err)
	assert.Equal(t, "raw", dec)
}

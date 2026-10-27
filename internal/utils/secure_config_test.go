package utils

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecryptSystemConfigValue_PlainAndEmpty(t *testing.T) {
	// 空密文直接返回空串
	v, err := DecryptSystemConfigValue("")
	noErr(t, err)
	isEmpty(t, v)

	// 无 ENC: 前缀按明文原样返回
	v, err = DecryptSystemConfigValue("plain-value")
	noErr(t, err)
	eq(t, "plain-value", v)

	// 小写 enc: 前缀不算加密值，原样返回
	v, err = DecryptSystemConfigValue("enc:not-encrypted")
	noErr(t, err)
	eq(t, "enc:not-encrypted", v)

	// unicode 明文
	v, err = DecryptSystemConfigValue("密码：中文✓")
	noErr(t, err)
	eq(t, "密码：中文✓", v)
}

func TestDecryptSystemConfigValue_RoundtripDefaultKey(t *testing.T) {
	// 未设置 ENCRYPTION_KEY 且非 release 模式，使用默认 key 加解密
	plaintext := "my-super-secret-ACR-password-密码"
	enc := encryptForTest(t, plaintext)
	isTrue(t, strings.HasPrefix(enc, "ENC:"))

	got, err := DecryptSystemConfigValue(enc)
	noErr(t, err)
	eq(t, plaintext, got)
}

func TestDecryptSystemConfigValue_CustomEnvKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key-123")

	enc := encryptForTest(t, "secret-with-env-key")
	got, err := DecryptSystemConfigValue(enc)
	noErr(t, err)
	eq(t, "secret-with-env-key", got)
}

func TestDecryptSystemConfigValue_KeyMismatch(t *testing.T) {
	// 用 key A 加密，换 key B 解密，应返回 gcm.Open 失败
	t.Setenv("ENCRYPTION_KEY", "key-A")
	enc := encryptForTest(t, "value-encrypted-with-A")

	t.Setenv("ENCRYPTION_KEY", "key-B")
	_, err := DecryptSystemConfigValue(enc)
	errContains(t, err, "decrypt")
}

func TestDecryptSystemConfigValue_ReleaseWithoutKey(t *testing.T) {
	// release 模式且未设置 ENCRYPTION_KEY，应直接报错
	t.Setenv("GIN_MODE", "release")

	_, err := DecryptSystemConfigValue("ENC:anything")
	errContains(t, err, "生产环境必须设置 ENCRYPTION_KEY")
}

func TestDecryptSystemConfigValue_Malformed(t *testing.T) {
	// 非 base64 内容
	_, err := DecryptSystemConfigValue("ENC:这不是base64!!!")
	errContains(t, err, "decode base64")

	// 密文过短（少于 nonce 长度）
	short := base64.StdEncoding.EncodeToString([]byte("ab"))
	_, err = DecryptSystemConfigValue("ENC:" + short)
	errContains(t, err, "ciphertext too short")

	// base64 合法但被篡改（gcm 校验失败）
	tampered := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 40)))
	_, err = DecryptSystemConfigValue("ENC:" + tampered)
	errContains(t, err, "decrypt")
}

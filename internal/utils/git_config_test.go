package utils

import (
	"errors"
	"testing"

	"docker-image-sync-platform/internal/models"
)

func TestGetConfigValueSimple(t *testing.T) {
	db := setupTestDB(t)

	// 正常路径： 配置存在
	noErr(t, db.Create(&models.SystemConfig{ConfigKey: "k1", ConfigValue: "v1"}).Error)
	v, err := GetConfigValueSimple("k1")
	noErr(t, err)
	eq(t, "v1", v)

	// 边界： 配置不存在
	v, err = GetConfigValueSimple("missing-key")
	errContains(t, err, "配置项 missing-key 不存在")
	isEmpty(t, v)

	// 边界： 空字符串 key
	_, err = GetConfigValueSimple("")
	isErr(t, err)
}

func TestGetConfigValueWithDecrypt_Plain(t *testing.T) {
	setupTestDB(t)
	writeSystemConfig(t, "plain_key", "plain-value", false)

	v, err := GetConfigValueWithDecrypt("plain_key", nil)
	noErr(t, err)
	eq(t, "plain-value", v)
}

func TestGetConfigValueWithDecrypt_Missing(t *testing.T) {
	setupTestDB(t)

	_, err := GetConfigValueWithDecrypt("no_such_key", nil)
	errContains(t, err, "配置项 no_such_key 不存在")
}

func TestGetConfigValueWithDecrypt_EncryptedNoFunc(t *testing.T) {
	setupTestDB(t)
	writeSystemConfig(t, "enc_key", "ENC:somecipher", true)

	// 已加密但未提供解密函数
	_, err := GetConfigValueWithDecrypt("enc_key", nil)
	errContains(t, err, "已加密但解密函数未提供")
}

func TestGetConfigValueWithDecrypt_EncryptedSuccess(t *testing.T) {
	setupTestDB(t)
	enc := encryptForTest(t, "decrypted-value")
	writeSystemConfig(t, "enc_key", enc, true)

	called := false
	fn := func(s string) (string, error) {
		called = true
		eq(t, enc, s)
		return "decrypted-value", nil
	}
	v, err := GetConfigValueWithDecrypt("enc_key", fn)
	noErr(t, err)
	eq(t, "decrypted-value", v)
	isTrue(t, called)
}

func TestGetConfigValueWithDecrypt_DecryptError(t *testing.T) {
	setupTestDB(t)
	writeSystemConfig(t, "enc_key", "ENC:bad", true)

	fn := func(s string) (string, error) { return "", errors.New("boom") }
	_, err := GetConfigValueWithDecrypt("enc_key", fn)
	errContains(t, err, "解密配置项 enc_key 失败")
}

func TestGetGitRepositoryType(t *testing.T) {
	db := setupTestDB(t)

	// 正常路径
	noErr(t, db.Create(&models.SystemConfig{ConfigKey: "git_repository_type", ConfigValue: "github"}).Error)
	got, err := GetGitRepositoryType()
	noErr(t, err)
	eq(t, "github", got)
}

func TestGetGitRepositoryType_DefaultGitee(t *testing.T) {
	setupTestDB(t)

	// 配置不存在时默认 gitee 且不返回错误
	got, err := GetGitRepositoryType()
	noErr(t, err)
	eq(t, "gitee", got)
}

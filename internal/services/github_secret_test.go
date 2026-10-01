package services

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/nacl/box"
)

// TestSealSecretForGitHub 验证 sealed box 输出可按 libsodium 格式解开：
// epk(32) || XSalsa20-Poly1305(nonce=blake2b-24(epk||pk))
func TestSealSecretForGitHub(t *testing.T) {
	recipientPK, recipientSK, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}
	pkB64 := base64.StdEncoding.EncodeToString(recipientPK[:])

	const secret = "Lipanxiang@1102:p@ss/word"
	sealedB64, err := sealSecretForGitHub(pkB64, secret)
	if err != nil {
		t.Fatalf("sealSecretForGitHub 报错: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		t.Fatalf("输出不是合法 Base64: %v", err)
	}
	if len(raw) < 32+box.Overhead {
		t.Fatalf("密文长度 %d 异常", len(raw))
	}

	var ephemeralPK [32]byte
	copy(ephemeralPK[:], raw[:32])

	var nonce [24]byte
	h, err := blake2b.New(24, nil)
	if err != nil {
		t.Fatalf("blake2b 初始化失败: %v", err)
	}
	h.Write(raw[:32])
	h.Write(recipientPK[:])
	copy(nonce[:], h.Sum(nil))

	plaintext, ok := box.Open(nil, raw[32:], &nonce, &ephemeralPK, recipientSK)
	if !ok {
		t.Fatal("按 libsodium sealed box 格式解密失败")
	}
	if string(plaintext) != secret {
		t.Errorf("解密结果 = %q, want %q", plaintext, secret)
	}
}

func TestSealSecretForGitHubBadKey(t *testing.T) {
	if _, err := sealSecretForGitHub("!!!not-base64!!!", "x"); err == nil {
		t.Error("非法 Base64 公钥应报错")
	}
	if _, err := sealSecretForGitHub(base64.StdEncoding.EncodeToString([]byte("short")), "x"); err == nil {
		t.Error("长度不足 32 字节的公钥应报错")
	}
}

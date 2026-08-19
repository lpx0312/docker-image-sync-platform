package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// 腾讯云 TC3-HMAC-SHA256 请求签名，用于访问腾讯云 API
// （如 CCR 个人版的 tcr.{region}.tencentcloudapi.com）。
//
// 签名流程：
//
//	CanonicalRequest = POST + \n + CanonicalURI(/) + \n + CanonicalQuery(空)
//	                   + CanonicalHeaders + \n + SignedHeaders + \n + Hex(SHA256(payload))
//	StringToSign     = "TC3-HMAC-SHA256" + \n + 时间戳 + \n + CredentialScope + \n
//	                   + Hex(SHA256(CanonicalRequest))
//	SecretDate       = HMAC-SHA256("TC3"+SecretKey, Date)
//	SecretService    = HMAC-SHA256(SecretDate, Service)
//	SecretSigning    = HMAC-SHA256(SecretService, "tc3_request")
//	Signature        = Hex(HMAC-SHA256(SecretSigning, StringToSign))
type tc3Signer struct {
	SecretID  string
	SecretKey string
}

// tc3SignedHeaders 参与签名的请求头（腾讯云规范固定为这三项，值均小写化）
const tc3SignedHeaders = "content-type;host;x-tc-action"

// tc3AuthHeader 计算指定 API 调用的 Authorization 头
//
// host 目标 API 域名（如 tcr.ap-guangzhou.tencentcloudapi.com）、service 服务名（如 tcr）、
// action 接口名（如 DescribeRepositoryFilterPersonal）、body 为 JSON 请求体。
func (s tc3Signer) tc3AuthHeader(host, service, action string, body []byte, ts time.Time) string {
	date := ts.UTC().Format("2006-01-02")
	timestamp := fmt.Sprintf("%d", ts.Unix())

	payloadHash := sha256.Sum256(body)
	canonical := "POST\n/\n\n" +
		"content-type:application/json; charset=utf-8\n" +
		"host:" + host + "\n" +
		"x-tc-action:" + lowerASCII(action) + "\n" +
		"\n" + tc3SignedHeaders + "\n" + hex.EncodeToString(payloadHash[:])

	scope := date + "/" + service + "/tc3_request"
	crHash := sha256.Sum256([]byte(canonical))
	stringToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hex.EncodeToString(crHash[:])

	secret := hmacSHA256([]byte("TC3"+s.SecretKey), date)
	secret = hmacSHA256(secret, service)
	secret = hmacSHA256(secret, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secret, stringToSign))

	return fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.SecretID, scope, tc3SignedHeaders, signature)
}

func hmacSHA256(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

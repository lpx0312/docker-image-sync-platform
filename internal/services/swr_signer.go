package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// 本文件实现华为云 API 网关的 SDK-HMAC-SHA256 请求签名，
// 用于访问 SWR 管理面 API（swr-api.{region}.myhuaweicloud.com）。
//
// 认证凭证即 SWR 登录指令（docker login）使用的永久 IAM AK/SK：
// 用户名格式为「区域@AK」，密码为 SK。注意 SWR 控制台的「临时登录指令」
// （24 小时有效）只能用于数据面 v2 协议认证，无法通过管理面签名校验。
//
// 签名流程：
//
//	CanonicalRequest = Method + \n + CanonicalURI + \n + CanonicalQuery + \n
//	                   + CanonicalHeaders + \n + SignedHeaders + \n + Hex(SHA256(payload))
//	StringToSign     = "SDK-HMAC-SHA256" + \n + 时间戳 + \n + Hex(SHA256(CanonicalRequest))
//	Signature        = Hex(HMAC-SHA256(SK, StringToSign))
type swrSigner struct {
	AccessKey string
	SecretKey string
}

// swrAKFromUsername 从 SWR 登录用户名（区域@AK）提取 AK
func swrAKFromUsername(username string) string {
	if idx := strings.LastIndex(username, "@"); idx >= 0 {
		return username[idx+1:]
	}
	return username
}

// sign 对 GET 请求（无 body）计算签名并写入鉴权头
func (s swrSigner) sign(req *http.Request, now time.Time) {
	timestamp := now.UTC().Format("20060102T150405Z")

	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	// 规范：URI 不以 / 结尾时需补 /（仅用于签名计算，不影响实际请求路径）
	if !strings.HasSuffix(canonicalURI, "/") {
		canonicalURI += "/"
	}

	var sb strings.Builder
	sb.WriteString(strings.ToUpper(req.Method))
	sb.WriteString("\n")
	sb.WriteString(canonicalURI)
	sb.WriteString("\n")
	sb.WriteString(canonicalQueryString(req.URL.Query()))
	sb.WriteString("\n")
	// CanonicalHeaders：小写 header 名 + ":" + 去空白值，每个头以 \n 结尾
	host := req.URL.Host
	sb.WriteString("host:" + strings.TrimSpace(host) + "\n")
	sb.WriteString("x-sdk-date:" + timestamp + "\n")
	sb.WriteString("\n")
	sb.WriteString("host;x-sdk-date")
	sb.WriteString("\n")

	payloadHash := sha256.Sum256(nil)
	sb.WriteString(hex.EncodeToString(payloadHash[:]))

	crHash := sha256.Sum256([]byte(sb.String()))
	stringToSign := "SDK-HMAC-SHA256\n" + timestamp + "\n" + hex.EncodeToString(crHash[:])

	mac := hmac.New(sha256.New, []byte(s.SecretKey))
	mac.Write([]byte(stringToSign))
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Host = host
	req.Header.Set("x-sdk-date", timestamp)
	req.Header.Set("Authorization", fmt.Sprintf(
		"SDK-HMAC-SHA256 Access=%s, SignedHeaders=host;x-sdk-date, Signature=%s",
		s.AccessKey, signature,
	))
}

// canonicalQueryString 查询参数按 key 排序并 URL 编码，得到规范查询串
func canonicalQueryString(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		for _, v := range query[k] {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

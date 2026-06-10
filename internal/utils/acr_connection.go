package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// TestAliyunRegistryConnection 测试阿里云 ACR 连接（含 Bearer Token 认证流程）
func TestAliyunRegistryConnection(registryURL, namespace, username, password, authServer, dockerService string) error {
	host := normalizeRegistryHost(registryURL)
	authURL := fmt.Sprintf("https://%s/v2/", host)

	client := newRegistryHTTPClient()

	req, err := http.NewRequest(http.MethodGet, authURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("User-Agent", "Docker-Image-Sync-Platform/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接镜像仓库失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return testAliyunBearerAuth(client, authURL, namespace, username, password, authServer, dockerService, resp.Header.Get("WWW-Authenticate"))
	case http.StatusForbidden:
		return fmt.Errorf("权限不足：用户没有访问该镜像仓库的权限")
	case http.StatusNotFound:
		return fmt.Errorf("镜像仓库不存在或地址错误")
	default:
		return fmt.Errorf("连接失败: HTTP %d: %s", resp.StatusCode, string(body))
	}
}

func testAliyunBearerAuth(client *http.Client, registryV2URL, namespace, username, password, authServer, dockerService, wwwAuthenticate string) error {
	realm, service := resolveAuthParams(authServer, dockerService, wwwAuthenticate, registryV2URL)
	if realm == "" {
		return fmt.Errorf("无法确定认证服务器地址，请填写认证服务器或检查仓库地址")
	}

	token, err := fetchBearerToken(client, realm, service, namespace, username, password)
	if err != nil {
		return err
	}

	apiReq, err := http.NewRequest(http.MethodGet, registryV2URL, nil)
	if err != nil {
		return fmt.Errorf("创建 API 请求失败: %w", err)
	}
	apiReq.Header.Set("Authorization", "Bearer "+token)
	apiReq.Header.Set("User-Agent", "Docker-Image-Sync-Platform/1.0")

	apiResp, err := client.Do(apiReq)
	if err != nil {
		return fmt.Errorf("使用 Bearer Token 访问仓库失败: %w", err)
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode == http.StatusOK {
		return nil
	}

	apiBody, _ := io.ReadAll(apiResp.Body)
	if apiResp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("认证失败：用户名或密码错误")
	}
	return fmt.Errorf("Bearer Token 认证失败，状态码: %d, 响应: %s", apiResp.StatusCode, string(apiBody))
}

func fetchBearerToken(client *http.Client, realm, service, namespace, username, password string) (string, error) {
	tokenURL := realm
	if service != "" {
		tokenURL += "?service=" + url.QueryEscape(service)
	}

	token, err := requestBearerTokenGET(client, tokenURL, username, password)
	if err == nil {
		return token, nil
	}

	// 部分 ACR 实例要求 POST + scope，与 AcrAPIService 保持一致
	if namespace != "" {
		if postToken, postErr := requestBearerTokenPOST(client, realm, service, namespace, username, password); postErr == nil {
			return postToken, nil
		}
	}
	return "", err
}

func requestBearerTokenGET(client *http.Client, tokenURL, username, password string) (string, error) {
	tokenReq, err := http.NewRequest(http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建 Token 请求失败: %w", err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	tokenReq.Header.Set("Authorization", "Basic "+auth)
	tokenReq.Header.Set("User-Agent", "Docker-Image-Sync-Platform/1.0")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("获取 Bearer Token 失败: %w", err)
	}
	defer tokenResp.Body.Close()

	return parseBearerTokenResponse(tokenResp)
}

func requestBearerTokenPOST(client *http.Client, realm, service, namespace, username, password string) (string, error) {
	if service == "" {
		service = "registry.aliyuncs.com:cn-hangzhou:26842"
	}
	authURL := strings.TrimSuffix(realm, "/")
	if !strings.HasSuffix(authURL, "/auth") {
		authURL += "/auth"
	}

	form := url.Values{}
	form.Set("service", service)
	form.Set("scope", fmt.Sprintf("repository:%s/_connection_test:pull", namespace))

	tokenReq, err := http.NewRequest(http.MethodPost, authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	tokenReq.Header.Set("Authorization", "Basic "+auth)
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("User-Agent", "Docker-Image-Sync-Platform/1.0")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", err
	}
	defer tokenResp.Body.Close()

	return parseBearerTokenResponse(tokenResp)
}

func parseBearerTokenResponse(tokenResp *http.Response) (string, error) {
	tokenBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 Token 响应失败: %w", err)
	}

	if tokenResp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("认证失败：用户名或密码错误")
	}
	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取 Bearer Token 失败，状态码: %d, 响应: %s", tokenResp.StatusCode, string(tokenBody))
	}

	var tokenData map[string]interface{}
	if err := json.Unmarshal(tokenBody, &tokenData); err != nil {
		return "", fmt.Errorf("解析 Token 响应失败: %w", err)
	}

	token, _ := tokenData["token"].(string)
	if token == "" {
		if accessToken, ok := tokenData["access_token"].(string); ok {
			token = accessToken
		}
	}
	if token == "" {
		return "", fmt.Errorf("认证失败：未获取到有效 Token")
	}
	return token, nil
}

func resolveAuthParams(authServer, dockerService, wwwAuthenticate, registryV2URL string) (realm, service string) {
	if authServer != "" {
		realm = strings.TrimPrefix(strings.TrimPrefix(authServer, "https://"), "http://")
		if !strings.HasPrefix(realm, "http") {
			realm = "https://" + realm
		}
		if !strings.HasSuffix(realm, "/auth") {
			realm = strings.TrimRight(realm, "/") + "/auth"
		}
		service = dockerService
		if service == "" {
			service = "registry.aliyuncs.com:cn-hangzhou:26842"
		}
		return realm, service
	}

	if wwwAuthenticate == "" {
		req, err := http.NewRequest(http.MethodGet, registryV2URL, nil)
		if err != nil {
			return "", ""
		}
		client := newRegistryHTTPClient()
		resp, err := client.Do(req)
		if err != nil {
			return "", ""
		}
		defer resp.Body.Close()
		wwwAuthenticate = resp.Header.Get("WWW-Authenticate")
	}

	if !strings.HasPrefix(wwwAuthenticate, "Bearer ") {
		return "", ""
	}

	authParams := strings.TrimPrefix(wwwAuthenticate, "Bearer ")
	for _, part := range strings.Split(authParams, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "realm="):
			realm = strings.Trim(strings.TrimPrefix(part, "realm="), "\"")
		case strings.HasPrefix(part, "service="):
			service = strings.Trim(strings.TrimPrefix(part, "service="), "\"")
		}
	}
	return realm, service
}

func normalizeRegistryHost(registryURL string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(registryURL, "https://"), "http://")
	return strings.TrimSuffix(host, "/")
}

func getRegionFromRegistry(registry string) string {
	parts := strings.Split(registry, ".")
	for _, part := range parts {
		if strings.HasPrefix(part, "cn-") || strings.HasPrefix(part, "us-") ||
			strings.HasPrefix(part, "eu-") || strings.HasPrefix(part, "ap-") {
			return part
		}
	}
	return "cn-hangzhou"
}

func newRegistryHTTPClient() *http.Client {
	transport := &http.Transport{}
	proxyURL := os.Getenv("HTTPS_PROXY")
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTP_PROXY")
	}
	if proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport}
}

// TestRegistryConnection 兼容旧调用，委托给阿里云专用测试
func TestRegistryConnection(registryURL, username, password string) error {
	host := normalizeRegistryHost(registryURL)
	region := getRegionFromRegistry(host)
	authServer := fmt.Sprintf("dockerauth.%s.aliyuncs.com", region)
	return TestAliyunRegistryConnection(registryURL, "", username, password, authServer, "registry.aliyuncs.com:"+region+":26842")
}

package services

import (
	"sync"
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsSWRRegistry 校验 SWR 地址识别（大小写不敏感）
func TestIsSWRRegistry(t *testing.T) {
	assert.True(t, IsSWRRegistry("swr.cn-east-3.myhuaweicloud.com"))
	assert.True(t, IsSWRRegistry("SWR.CN-NORTH-4.MYHUAWEICLOUD.COM"))
	assert.False(t, IsSWRRegistry("registry.cn-hangzhou.aliyuncs.com"))
	assert.False(t, IsSWRRegistry(""))
	assert.False(t, IsSWRRegistry("ccr.ccs.tencentyun.com"))
}

// TestIsCCRRegistry 校验腾讯云 CCR 地址识别
func TestIsCCRRegistry(t *testing.T) {
	assert.True(t, IsCCRRegistry("ccr.ccs.tencentyun.com"))
	assert.True(t, IsCCRRegistry("CCR.CCS.TENCENTYUN.COM"))
	assert.False(t, IsCCRRegistry("swr.cn-east-3.myhuaweicloud.com"))
	assert.False(t, IsCCRRegistry(""))
}

// TestNewRegistryAPIService 覆盖工厂分发与单例缓存（保存/恢复包级单例）
func TestNewRegistryAPIService(t *testing.T) {
	savedAcr, savedSwr, savedCcr, savedGeneric := acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance
	t.Cleanup(func() {
		acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance = savedAcr, savedSwr, savedCcr, savedGeneric
	})
	acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance = nil, nil, nil, nil

	// 类型字段分发
	swr1 := NewRegistryAPIService(models.RegistryTypeSWR, "")
	_, ok := swr1.(*SwrAPIService)
	assert.True(t, ok, "swr 类型应返回 SwrAPIService")

	ccr1 := NewRegistryAPIService(models.RegistryTypeCCR, "")
	_, ok = ccr1.(*CcrAPIService)
	assert.True(t, ok, "ccr 类型应返回 CcrAPIService")

	harbor1 := NewRegistryAPIService(models.RegistryTypeHarbor, "")
	_, ok = harbor1.(*GenericAPIService)
	assert.True(t, ok, "harbor 类型应返回 GenericAPIService")

	generic1 := NewRegistryAPIService(models.RegistryTypeGeneric, "")
	_, ok = generic1.(*GenericAPIService)
	assert.True(t, ok, "generic 类型应返回 GenericAPIService")

	acr1 := NewRegistryAPIService(models.RegistryTypeACR, "")
	_, ok = acr1.(*AcrAPIService)
	assert.True(t, ok, "acr 类型应返回 AcrAPIService")

	// 兜底：未知类型 + 普通地址 → ACR
	fallback := NewRegistryAPIService("", "registry.example.com")
	_, ok = fallback.(*AcrAPIService)
	assert.True(t, ok, "未知类型应兜底返回 AcrAPIService")

	// 地址兜底识别
	byURLSWR := NewRegistryAPIService("", "swr.cn-east-3.myhuaweicloud.com")
	_, ok = byURLSWR.(*SwrAPIService)
	assert.True(t, ok, "地址含 myhuaweicloud.com 应按 SWR 处理")

	byURLCCR := NewRegistryAPIService("", "ccr.ccs.tencentyun.com")
	_, ok = byURLCCR.(*CcrAPIService)
	assert.True(t, ok, "地址含 tencentyun.com 应按 CCR 处理")

	// 单例缓存：同类型返回同一实例
	assert.Same(t, swr1, NewRegistryAPIService(models.RegistryTypeSWR, ""))
	assert.Same(t, ccr1, NewRegistryAPIService(models.RegistryTypeCCR, ""))
	assert.Same(t, generic1, NewRegistryAPIService(models.RegistryTypeGeneric, ""))
	assert.Same(t, harbor1, NewRegistryAPIService(models.RegistryTypeHarbor, ""))
	assert.Same(t, acr1, NewRegistryAPIService(models.RegistryTypeACR, ""))
}

// TestNewRegistryAPIServiceConcurrent 并发调用工厂应返回同一实例（单例互斥保护）
func TestNewRegistryAPIServiceConcurrent(t *testing.T) {
	savedAcr, savedSwr, savedCcr, savedGeneric := acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance
	t.Cleanup(func() {
		acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance = savedAcr, savedSwr, savedCcr, savedGeneric
	})
	acrClientInstance, swrClientInstance, ccrClientInstance, genericClientInstance = nil, nil, nil, nil

	const n = 32
	results := make([]RegistryAPIClient, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = NewRegistryAPIService(models.RegistryTypeSWR, "")
		}(i)
	}
	wg.Wait()
	for _, r := range results[1:] {
		require.Same(t, results[0], r, "并发获取 SWR 客户端应命中同一单例")
	}
}

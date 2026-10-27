package handlers

// config_test.go：系统配置 Handler 测试（单元 + 边界 + 路由级）。
// 说明：GitHub/Gitee/阿里云客户端硬编码 https:// 域名，无法指向 httptest，
// 网络相关分支只测「参数校验失败 / URL 解析失败 / 连接不可达（127.0.0.1:1）」
// 等确定性错误路径；真实远端 200 分支不构造。

import (
	"net/http"
	"testing"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// cfgSetup 构造真实依赖的 ConfigHandler 并挂好全局 DB，返回处理器与测试库
func cfgSetup(t *testing.T) (*ConfigHandler, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	withGlobalDB(t, db)
	enc := hNewEncSvc(t)
	factory := services.NewGitServiceFactory(enc)
	cfgSvc := services.NewConfigService(db, enc, hNewLogrus())
	return NewConfigHandler(factory, cfgSvc), db
}

// cfgRouter 注册 ConfigHandler 全部路由
func cfgRouter(h *ConfigHandler) http.Handler {
	r := hNewEngine()
	g := r.Group("/config")
	{
		g.GET("/git-repository", h.GetGitRepositoryConfig)
		g.PUT("/git-repository", h.UpdateGitRepositoryConfig)
		g.GET("/git", h.GetGitConfig)
		g.PUT("/git/gitee", h.UpdateGiteeConfig)
		g.PUT("/git/github", h.UpdateGitHubConfig)
		g.POST("/git/test", h.TestGitConnection)
		g.GET("/aliyun", h.GetAliyunConfig)
		g.PUT("/aliyun", h.UpdateAliyunConfig)
		g.POST("/aliyun/test", h.TestAliyunConnection)
		g.GET("/status", h.GetConfigStatus)
		g.GET("/all", h.GetAllConfigs)
		g.GET("/git-optimization", h.GetGitOptimizationConfig)
		g.PUT("/git-optimization", h.UpdateGitOptimizationConfig)
		g.GET("/git-performance", h.GetGitPerformanceMetrics)
		g.GET("/git-network-test", h.TestGitNetworkQuality)
		g.POST("/git-test-operations", h.TestGitOperations)
	}
	return r
}

func TestConfigHandler_GitRepositoryConfig(t *testing.T) {
	h, db := cfgSetup(t)
	r := cfgRouter(h)

	t.Run("GET_默认gitee", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/config/git-repository", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, "gitee", data["repository_type"], "未配置时应回退默认 gitee")
	})

	t.Run("GET_读取已写配置", func(t *testing.T) {
		hSetConfig(t, db, "git_repository_type", "github")
		w, body := hPerformJSON(t, r, http.MethodGet, "/config/git-repository", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]interface{})
		assert.Equal(t, "github", data["repository_type"])
	})

	t.Run("PUT_坏JSON_400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git-repository", `not-json`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT_缺字段_400", func(t *testing.T) {
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git-repository", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT_非法类型_400", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodPut, "/config/git-repository",
			`{"repository_type":"gitlab"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "Invalid repository type")
	})

	t.Run("PUT_成功并生效", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodPut, "/config/git-repository",
			`{"repository_type":"github"}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, "github", data["repository_type"])

		var val string
		require.NoError(t, db.Raw("SELECT config_value FROM system_configs WHERE config_key = 'git_repository_type'").Scan(&val).Error)
		assert.Equal(t, "github", val)
	})

	t.Run("PUT_更新失败_500", func(t *testing.T) {
		hSetConfig(t, db, "git_repository_type", "gitee")
		hFailUpdatesOn(t, db, "system_configs")
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git-repository",
			`{"repository_type":"github"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestConfigHandler_GetGitConfig(t *testing.T) {
	h, db := cfgSetup(t)
	r := cfgRouter(h)

	t.Run("空配置200", func(t *testing.T) {
		w, body := hPerformJSON(t, r, http.MethodGet, "/config/git", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, "gitee", data["repository_type"])
		_, hasLocalPath := data["gitee"].(map[string]interface{})["local_path"]
		assert.False(t, hasLocalPath, "API模式下不应返回 local_path")
	})

	t.Run("返回已存配置", func(t *testing.T) {
		hSetConfig(t, db, "gitee_repo_url", "https://gitee.com/u/r")
		hSetConfig(t, db, "gitee_username", "gitee-user")
		hSetConfig(t, db, "github_repo_url", "https://github.com/o/p")
		w, body := hPerformJSON(t, r, http.MethodGet, "/config/git", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]interface{})
		gitee := data["gitee"].(map[string]interface{})
		github := data["github"].(map[string]interface{})
		assert.Equal(t, "https://gitee.com/u/r", gitee["repo_url"])
		assert.Equal(t, "gitee-user", gitee["username"])
		assert.Equal(t, "https://github.com/o/p", github["repo_url"])
	})
}

func TestConfigHandler_UpdateGitPlatformConfig(t *testing.T) {
	t.Run("Gitee_坏JSON_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git/gitee", ``, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Gitee_缺必填_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, body := hPerformJSON(t, r, http.MethodPut, "/config/git/gitee",
			`{"repo_url":"https://gitee.com/u/r"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "required")
	})

	t.Run("Gitee_成功200_密码加密落库", func(t *testing.T) {
		h, db := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git/gitee",
			`{"repo_url":"https://gitee.com/u/r","username":"u","password":"p","email":"u@x.com","branch":"main"}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var cfg models.SystemConfig
		require.NoError(t, db.Where("config_key = ?", "gitee_password").First(&cfg).Error)
		assert.True(t, cfg.IsEncrypted, "gitee_password 应加密存储")
		assert.NotEqual(t, "p", cfg.ConfigValue)
	})

	t.Run("Gitee_更新失败_500", func(t *testing.T) {
		h, db := cfgSetup(t)
		r := cfgRouter(h)
		hSetConfig(t, db, "gitee_repo_url", "https://gitee.com/u/r")
		hFailUpdatesOn(t, db, "system_configs")
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git/gitee",
			`{"repo_url":"https://gitee.com/u/r","username":"u","password":"p","email":"u@x.com"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("GitHub_坏JSON_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git/github", `bad`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GitHub_缺必填_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git/github", `{"username":"u"}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GitHub_成功200_token加密落库", func(t *testing.T) {
		h, db := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git/github",
			`{"repo_url":"https://github.com/o/p","username":"u","token":"tk","email":"u@x.com","branch":"main"}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var cfg models.SystemConfig
		require.NoError(t, db.Where("config_key = ?", "github_token").First(&cfg).Error)
		assert.True(t, cfg.IsEncrypted, "github_token 应加密存储")
	})

	t.Run("GitHub_更新失败_500", func(t *testing.T) {
		h, db := cfgSetup(t)
		r := cfgRouter(h)
		hSetConfig(t, db, "github_repo_url", "https://github.com/o/p")
		hFailUpdatesOn(t, db, "system_configs")
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git/github",
			`{"repo_url":"https://github.com/o/p","username":"u","token":"tk","email":"u@x.com"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestConfigHandler_TestGitConnection(t *testing.T) {
	newRouter := func(t *testing.T) http.Handler {
		h, _ := cfgSetup(t)
		return cfgRouter(h)
	}

	t.Run("坏JSON_400", func(t *testing.T) {
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("非法类型_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test",
			`{"type":"gitlab","repo_url":"https://x.com/o/r","username":"u","password":"p"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "Invalid git type")
	})

	t.Run("gitee_缺密码_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test",
			`{"type":"gitee","repo_url":"https://gitee.com/o/r","username":"u"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "password is required")
	})

	t.Run("github_缺token_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test",
			`{"type":"github","repo_url":"https://github.com/o/r","username":"u","password":"p"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "token is required")
	})

	t.Run("github_URL解析失败_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test",
			`{"type":"github","repo_url":"https://github.com/only-owner","username":"u","token":"t"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "无效的GitHub仓库URL")
	})

	t.Run("github_假token凭据校验失败_400", func(t *testing.T) {
		// 无论网络通（401）还是不通（连接错误），都是确定性失败 → 400
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test",
			`{"type":"github","repo_url":"https://github.com/o/r","username":"u","token":"definitely-fake-token"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		msg, _ := body["message"].(string)
		assert.Contains(t, msg, "Git connection test failed")
	})

	t.Run("gitee_URL解析失败_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test",
			`{"type":"gitee","repo_url":"https://gitee.com/only-owner","username":"u","password":"p"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "无效的Gitee仓库URL")
	})

	t.Run("gitee_不存在仓库或网络不可达_400", func(t *testing.T) {
		// 网络可达时 gitee 对不存在仓库/无效凭据返回 404/401，不可达时报连接错误，
		// 两种情况均为确定失败 → 400
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git/test",
			`{"type":"gitee","repo_url":"https://gitee.com/no-such-owner-xyz/no-such-repo","username":"u","password":"definitely-wrong-token"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		msg, _ := body["message"].(string)
		assert.Contains(t, msg, "Git connection test failed")
	})
}

func TestConfigHandler_testGiteeConnection(t *testing.T) {
	h, _ := cfgSetup(t)

	t.Run("URL解析失败", func(t *testing.T) {
		err := h.testGiteeConnection("://bad url", "u", "p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "无效的Gitee仓库URL")
	})

	t.Run("路径缺少owner或repo", func(t *testing.T) {
		err := h.testGiteeConnection("https://gitee.com/only", "u", "p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "无效的Gitee仓库URL")
	})

	t.Run("请求执行失败_非法凭据头", func(t *testing.T) {
		// Authorization 头含换行 → client.Do 直接报 invalid header field value，
		// 不依赖网络即可覆盖「连接Gitee API失败」分支
		// （net/http 的代理环境变量在进程首次请求后被缓存，t.Setenv 死代理方式不可靠）
		err := h.testGiteeConnection("https://gitee.com/o/r", "u", "p\r\nX-Bad: 1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "连接Gitee API失败")
	})

	t.Run("无效凭据或仓库不存在_失败", func(t *testing.T) {
		// 网络可达时 gitee 返回 401/404，不可达时为连接错误，均确定失败
		err := h.testGiteeConnection("https://gitee.com/no-such-owner-xyz/no-such-repo", "u", "definitely-wrong-token")
		require.Error(t, err)
	})
}

func TestConfigHandler_parseGiteeRepoURL(t *testing.T) {
	owner, repo, err := parseGiteeRepoURL("  https://gitee.com/owner/repo.git  ")
	require.NoError(t, err)
	assert.Equal(t, "owner", owner)
	assert.Equal(t, "repo", repo)

	owner, repo, err = parseGiteeRepoURL("https://gitee.com/o/r/extra/path")
	require.NoError(t, err)
	assert.Equal(t, "o", owner)
	assert.Equal(t, "r", repo)

	_, _, err = parseGiteeRepoURL("https://gitee.com/only")
	assert.Error(t, err)

	_, _, err = parseGiteeRepoURL("://bad")
	assert.Error(t, err)
}

func TestConfigHandler_AliyunConfig(t *testing.T) {
	t.Run("GET_空配置200", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, body := hPerformJSON(t, r, http.MethodGet, "/config/aliyun", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]interface{})
		assert.Empty(t, data["password"], "无密码时应返回空串")
	})

	t.Run("PUT_坏JSON_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/aliyun", `oops`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT_缺必填_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, body := hPerformJSON(t, r, http.MethodPut, "/config/aliyun", `{"registry":"r"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "required")
	})

	t.Run("PUT_成功后GET密码脱敏", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/aliyun",
			`{"registry":"registry.example.com","namespace":"ns","username":"u","password":"p","region":"cn-x"}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		w, body := hPerformJSON(t, r, http.MethodGet, "/config/aliyun", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]interface{})
		assert.Equal(t, "registry.example.com", data["registry_url"])
		assert.Equal(t, "***", data["password"], "有密码时应显示占位符")
		assert.Equal(t, "cn-x", data["region"])
	})

	t.Run("PUT_更新失败_500", func(t *testing.T) {
		h, db := cfgSetup(t)
		r := cfgRouter(h)
		hSetConfig(t, db, "aliyun_registry", "registry.example.com")
		hFailUpdatesOn(t, db, "system_configs")
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/aliyun",
			`{"registry":"registry.example.com","namespace":"ns","username":"u"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestConfigHandler_TestAliyunConnection(t *testing.T) {
	newRouter := func(t *testing.T) http.Handler {
		h, _ := cfgSetup(t)
		return cfgRouter(h)
	}

	t.Run("坏JSON_400", func(t *testing.T) {
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/aliyun/test", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("仓库不可达_500", func(t *testing.T) {
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/aliyun/test",
			`{"registry_url":"127.0.0.1:1","namespace":"ns","username":"u","password":"p"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("非法地址_构造请求失败_500", func(t *testing.T) {
		// 含空格的 host 使 http.NewRequest 的 URL 解析失败
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/aliyun/test",
			`{"registry_url":"bad host","namespace":"ns","username":"u","password":"p"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("走HTTP代理仍失败_500", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/aliyun/test",
			`{"registry_url":"registry.example.com","namespace":"ns","username":"u","password":"p"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("代理地址非法时忽略代理_500", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "://not-a-proxy")
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/aliyun/test",
			`{"registry_url":"127.0.0.1:1","namespace":"ns","username":"u","password":"p"}`, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestConfigHandler_testAliyunRegistryConnection(t *testing.T) {
	h, _ := cfgSetup(t)

	t.Run("连接失败", func(t *testing.T) {
		err := h.testAliyunRegistryConnection("127.0.0.1:1", "ns", "u", "p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "连接阿里云镜像仓库失败")
	})

	t.Run("Bearer认证_获取认证信息失败", func(t *testing.T) {
		t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
		t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
		err := h.testAliyunBearerAuth("127.0.0.1:1", "ns", "u", "p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取认证信息失败")
	})
}

func TestConfigHandler_GetConfigStatus(t *testing.T) {
	h, db := cfgSetup(t)
	r := cfgRouter(h)
	hSetConfig(t, db, "gitee_repo_url", "https://gitee.com/u/r")
	hSetConfig(t, db, "aliyun_namespace", "ns-x")

	w, body := hPerformJSON(t, r, http.MethodGet, "/config/status", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", body["status"])

	dbStatus := body["database"].(map[string]interface{})
	assert.Equal(t, true, dbStatus["connected"])

	cfgData := body["config"].(map[string]interface{})
	git := cfgData["git"].(map[string]interface{})
	gitee := git["gitee"].(map[string]interface{})
	assert.Equal(t, "https://gitee.com/u/r", gitee["repo_url"])
	aliyun := cfgData["aliyun"].(map[string]interface{})
	assert.Equal(t, "ns-x", aliyun["namespace"])

	enc := cfgData["encryption"].(map[string]interface{})
	assert.Equal(t, false, enc["using_default_key"], "测试设置了 ENCRYPTION_KEY，应报告 env 来源")
	assert.Equal(t, "env", enc["key_source"])

	t.Run("数据库未初始化", func(t *testing.T) {
		saved := database.DB
		database.DB = nil
		t.Cleanup(func() { database.DB = saved })

		w, body := hPerformJSON(t, r, http.MethodGet, "/config/status", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		dbStatus := body["database"].(map[string]interface{})
		assert.Equal(t, false, dbStatus["connected"])
		assert.Equal(t, "Database not initialized", dbStatus["error"])
	})
}

func TestConfigHandler_GetAllConfigs(t *testing.T) {
	h, db := cfgSetup(t)
	r := cfgRouter(h)
	hSetConfig(t, db, "github_repo_url", "https://github.com/o/p")
	hSetConfig(t, db, "aliyun_registry", "registry.example.com")

	w, body := hPerformJSON(t, r, http.MethodGet, "/config/all", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := body["data"].(map[string]interface{})

	git := data["git"].(map[string]interface{})
	github := git["github"].(map[string]interface{})
	assert.Equal(t, "https://github.com/o/p", github["repo_url"])
	assert.Equal(t, "gitee", git["repository_type"])

	aliyun := data["aliyun"].(map[string]interface{})
	assert.Equal(t, "registry.example.com", aliyun["registry"])
	_, hasPassword := aliyun["password"]
	assert.False(t, hasPassword, "聚合接口不应返回密码字段")
}

func TestConfigHandler_GitOptimization(t *testing.T) {
	t.Run("GET_200", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, body := hPerformJSON(t, r, http.MethodGet, "/config/git-optimization", "", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, true, body["success"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, true, data["use_optimized"])
		assert.Equal(t, true, data["optimization_enabled"])
		assert.Equal(t, []interface{}{"sparse", "full", "auto"}, data["available_modes"])
	})

	t.Run("PUT_开启成功200", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, body := hPerformJSON(t, r, http.MethodPut, "/config/git-optimization",
			`{"use_optimized":true}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, true, body["success"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, true, data["use_optimized"])
	})

	// 疑似 bug：use_optimized 带 binding:"required"，bool 零值 false 无法通过校验，
	// 导致"关闭优化服务"这一 API 操作永远返回 400。按真实行为断言。
	t.Run("PUT_关闭被required拒绝_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git-optimization",
			`{"use_optimized":false}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT_坏JSON_400", func(t *testing.T) {
		h, _ := cfgSetup(t)
		r := cfgRouter(h)
		w, _ := hPerformJSON(t, r, http.MethodPut, "/config/git-optimization", `x`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestConfigHandler_GetGitPerformanceMetrics(t *testing.T) {
	h, _ := cfgSetup(t)
	r := cfgRouter(h)
	w, body := hPerformJSON(t, r, http.MethodGet, "/config/git-performance", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, body["success"])
	assert.NotNil(t, body["data"])
}

func TestConfigHandler_TestGitNetworkQuality(t *testing.T) {
	h, _ := cfgSetup(t)
	r := cfgRouter(h)
	w, body := hPerformJSON(t, r, http.MethodGet, "/config/git-network-test", "", nil)
	// 网络探测依赖真实 github.com/gitee.com TCP 连通性：成功 200，全部失败 500，
	// 两种结果都符合实现，只断言状态码与结构。
	if w.Code == http.StatusOK {
		data := body["data"].(map[string]interface{})
		assert.Contains(t, []interface{}{"good", "medium", "poor", "unknown"}, data["quality"])
		assert.NotEmpty(t, data["recommendation"])
	} else {
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, false, body["success"])
	}
}

func TestConfigHandler_getQualityString(t *testing.T) {
	h, _ := cfgSetup(t)
	assert.Equal(t, "good", h.getQualityString(services.NetworkQualityGood))
	assert.Equal(t, "medium", h.getQualityString(services.NetworkQualityMedium))
	assert.Equal(t, "poor", h.getQualityString(services.NetworkQualityPoor))
	assert.Equal(t, "unknown", h.getQualityString(services.NetworkQuality(99)))
}

func TestConfigHandler_TestGitOperations(t *testing.T) {
	newRouter := func(t *testing.T) http.Handler {
		h, _ := cfgSetup(t)
		return cfgRouter(h)
	}

	t.Run("坏JSON_400", func(t *testing.T) {
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git-test-operations", `{}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺必填_400", func(t *testing.T) {
		w, _ := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git-test-operations",
			`{"repo_url":"https://github.com/o/p"}`, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("不支持的仓库类型_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git-test-operations",
			`{"repo_url":"https://gitlab.com/o/p","username":"u","token":"t","email":"e","branch":"main"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "不支持的Git仓库类型")
	})

	t.Run("Gitee仓库不支持_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git-test-operations",
			`{"repo_url":"https://gitee.com/o/p","username":"u","token":"t","email":"e","branch":"main"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "仅支持GitHub")
	})

	t.Run("GitHub_URL解析失败_400", func(t *testing.T) {
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git-test-operations",
			`{"repo_url":"https://github.com/owner","username":"u","token":"t","email":"e","branch":"main"}`, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body["message"], "解析GitHub仓库URL失败")
	})

	t.Run("GitHub_假token拉取失败_200带失败详情", func(t *testing.T) {
		// 凭据无效（401）或网络不可达均走同一失败路径，结果确定性
		w, body := hPerformJSON(t, newRouter(t), http.MethodPost, "/config/git-test-operations",
			`{"repo_url":"https://github.com/o/p","username":"u","token":"definitely-fake-token","email":"e@x.com","branch":"main"}`, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, true, body["success"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, false, data["pull_success"])
		assert.Equal(t, false, data["commit_success"])
		assert.Equal(t, false, data["push_success"])
		assert.NotEmpty(t, data["error_message"])
		msg, _ := body["message"].(string)
		assert.Contains(t, msg, "部分失败")
	})
}

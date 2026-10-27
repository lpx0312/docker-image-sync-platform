package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// 轻量断言助手（不引入新依赖）
// ---------------------------------------------------------------------------

func eq(t *testing.T, want, got interface{}) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("不相等:\n期望: %#v\n实际: %#v", want, got)
	}
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("期望无错误, 实际: %v", err)
	}
}

func contains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("%q 未包含 %q", s, sub)
	}
}

// resetConfigGlobals 隔离 viper / AppConfig 全局状态，测试结束后恢复
func resetConfigGlobals(t *testing.T) {
	t.Helper()
	oldAppConfig := AppConfig
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		AppConfig = oldAppConfig
	})
}

// loadFromYAML 把 YAML 内容写入临时文件并完整走一次加载流程
func loadFromYAML(t *testing.T, content string) error {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	noErr(t, os.WriteFile(path, []byte(content), 0644))
	return LoadConfig(path)
}

// 本机环境可能已设置 GITHUB_TOKEN（viper 绑定到 git.github.token），统一固定为可控值
func pinNoisyEnvs(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "pinned-github-token-for-test")
}

// ---------------------------------------------------------------------------
// e2e: 真实写临时 config.yaml 后完整走加载流程
// ---------------------------------------------------------------------------

func TestLoadConfig_FullYAMLFile(t *testing.T) {
	resetConfigGlobals(t)
	pinNoisyEnvs(t)

	yaml := `
server:
  port: "9090"
  mode: release

database:
  host: db.example.com
  port: 3307
  username: dbuser
  password: "dbpass!@#"
  database: syncdb
  charset: gbk
  parse_time: false
  loc: UTC

git:
  repository_type: github
  gitee:
    repo_url: https://gitee.com/u/r
    username: gitee-user
    password: gitee-pass
    token: gitee-token
    email: gitee@x.com
    branch: dev
  github:
    repo_url: https://github.com/u/r
    username: gh-user
    token: gh-token
    email: gh@x.com
    branch: main

aliyun:
  registry: registry.cn-beijing.aliyuncs.com
  namespace: other-ns
  username: ali-user
  password: ali-pass

log:
  level: debug
  file_path: /var/log/sync/app.log
  max_size: 50
  max_backups: 7
  max_age: 14

sync:
  timeout_minutes: 60
  max_concurrent_jobs: 8

auth:
  jwt_secret: my-jwt-secret
  token_expiry: 12h
  remember_me_expiry: 72h
  auto_logout_minutes: 15
  default_admin_username: root
  default_admin_password: rootpw

github_actions:
  workflow_file: build.yml
  api_timeout_seconds: 45
  status_check_interval_seconds: 120

security:
  rate_limit:
    requests_per_minute: 200
    burst: 20
  cors:
    allowed_origins:
      - https://a.com
      - https://b.com
    allowed_methods: [GET, POST]
    allowed_headers: [X-Custom]
`
	noErr(t, loadFromYAML(t, yaml))
	if AppConfig == nil {
		t.Fatalf("AppConfig 应已初始化")
	}

	c := AppConfig
	eq(t, "9090", c.Server.Port)
	eq(t, "release", c.Server.Mode)

	eq(t, "db.example.com", c.Database.Host)
	eq(t, 3307, c.Database.Port)
	eq(t, "dbuser", c.Database.Username)
	eq(t, "dbpass!@#", c.Database.Password)
	eq(t, "syncdb", c.Database.Database)
	eq(t, "gbk", c.Database.Charset)
	eq(t, false, c.Database.ParseTime)
	eq(t, "UTC", c.Database.Loc)

	eq(t, "github", c.Git.RepositoryType)
	eq(t, "https://gitee.com/u/r", c.Git.Gitee.RepoURL)
	eq(t, "gitee-user", c.Git.Gitee.Username)
	eq(t, "gitee-pass", c.Git.Gitee.Password)
	eq(t, "gitee-token", c.Git.Gitee.Token)
	eq(t, "gitee@x.com", c.Git.Gitee.Email)
	eq(t, "dev", c.Git.Gitee.Branch)
	eq(t, "https://github.com/u/r", c.Git.GitHub.RepoURL)
	eq(t, "gh-user", c.Git.GitHub.Username)
	// github.token 绑定了 GITHUB_TOKEN 环境变量，环境变量优先于配置文件（pinNoisyEnvs 已固定其值）
	eq(t, "pinned-github-token-for-test", c.Git.GitHub.Token)
	eq(t, "main", c.Git.GitHub.Branch)

	eq(t, "registry.cn-beijing.aliyuncs.com", c.Aliyun.Registry)
	eq(t, "other-ns", c.Aliyun.Namespace)
	eq(t, "ali-user", c.Aliyun.Username)
	eq(t, "ali-pass", c.Aliyun.Password)

	eq(t, "debug", c.Log.Level)
	eq(t, "/var/log/sync/app.log", c.Log.FilePath)
	eq(t, 50, c.Log.MaxSize)
	eq(t, 7, c.Log.MaxBackups)
	eq(t, 14, c.Log.MaxAge)

	eq(t, 60, c.Sync.TimeoutMinutes)
	eq(t, 8, c.Sync.MaxConcurrentJobs)

	eq(t, "my-jwt-secret", c.Auth.JWTSecret)
	eq(t, "12h", c.Auth.TokenExpiry)
	eq(t, "72h", c.Auth.RememberMeExpiry)
	eq(t, 15, c.Auth.AutoLogoutMinutes)
	eq(t, "root", c.Auth.DefaultAdminUsername)
	eq(t, "rootpw", c.Auth.DefaultAdminPassword)

	eq(t, "build.yml", c.GitHubActions.WorkflowFile)
	eq(t, 45, c.GitHubActions.APITimeoutSeconds)
	eq(t, 120, c.GitHubActions.StatusCheckIntervalSeconds)

	eq(t, 200, c.Security.RateLimit.RequestsPerMinute)
	eq(t, 20, c.Security.RateLimit.Burst)
	eq(t, []string{"https://a.com", "https://b.com"}, c.Security.CORS.AllowedOrigins)
	eq(t, []string{"GET", "POST"}, c.Security.CORS.AllowedMethods)
	eq(t, []string{"X-Custom"}, c.Security.CORS.AllowedHeaders)
}

// ---------------------------------------------------------------------------
// 边界: 空配置文件使用全部默认值
// ---------------------------------------------------------------------------

func TestLoadConfig_MinimalYAMLFile_UsesDefaults(t *testing.T) {
	resetConfigGlobals(t)
	pinNoisyEnvs(t)

	// 空配置文件： 全部走默认值
	noErr(t, loadFromYAML(t, ""))
	c := AppConfig
	eq(t, "8080", c.Server.Port)
	eq(t, "debug", c.Server.Mode)
	eq(t, "localhost", c.Database.Host)
	eq(t, 3306, c.Database.Port)
	eq(t, "utf8mb4", c.Database.Charset)
	eq(t, true, c.Database.ParseTime)
	eq(t, "Local", c.Database.Loc)
	eq(t, "info", c.Log.Level)
	eq(t, "./logs/app.log", c.Log.FilePath)
	eq(t, 100, c.Log.MaxSize)
	eq(t, 3, c.Log.MaxBackups)
	eq(t, 28, c.Log.MaxAge)
	eq(t, 30, c.Sync.TimeoutMinutes)
	// 注意: sync.max_concurrent_jobs 未设置默认值，当前实际行为为 0
	eq(t, 0, c.Sync.MaxConcurrentJobs)
	eq(t, "docker.yaml", c.GitHubActions.WorkflowFile)
	eq(t, 30, c.GitHubActions.APITimeoutSeconds)
	eq(t, 60, c.GitHubActions.StatusCheckIntervalSeconds)
	eq(t, 100, c.Security.RateLimit.RequestsPerMinute)
	eq(t, 10, c.Security.RateLimit.Burst)
	eq(t, []string{"*"}, c.Security.CORS.AllowedOrigins)
	eq(t, []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, c.Security.CORS.AllowedMethods)
	eq(t, []string{"*"}, c.Security.CORS.AllowedHeaders)
	eq(t, "docker-sync-platform-jwt-secret-change-me", c.Auth.JWTSecret)
	eq(t, "24h", c.Auth.TokenExpiry)
	eq(t, "168h", c.Auth.RememberMeExpiry)
	eq(t, 30, c.Auth.AutoLogoutMinutes)
	eq(t, "admin", c.Auth.DefaultAdminUsername)
	eq(t, "admin123", c.Auth.DefaultAdminPassword)
	eq(t, "pinned-github-token-for-test", c.Git.GitHub.Token)
}

func TestLoadConfig_MissingFile_StillSucceedsWithDefaults(t *testing.T) {
	resetConfigGlobals(t)
	pinNoisyEnvs(t)

	// 文件不存在不返回错误（仅 log 警告），使用默认值 + 环境变量
	path := filepath.Join(t.TempDir(), "not-exist.yaml")
	noErr(t, LoadConfig(path))
	eq(t, "8080", AppConfig.Server.Port)
	eq(t, "info", AppConfig.Log.Level)
}

func TestLoadConfig_MalformedYAML_FallsBackToDefaults(t *testing.T) {
	resetConfigGlobals(t)
	pinNoisyEnvs(t)

	// 畸形 YAML: ReadInConfig 失败仅告警，继续用默认值
	noErr(t, loadFromYAML(t, "server:\n  port: [unclosed\n\t bad indent"))
	eq(t, "8080", AppConfig.Server.Port)
}

func TestLoadConfig_UnmarshalTypeError(t *testing.T) {
	resetConfigGlobals(t)
	pinNoisyEnvs(t)

	// database.port 无法转换为 int，viper.Unmarshal 报错，LoadConfig 返回错误
	err := loadFromYAML(t, "database:\n  port: [not, an, int]\n")
	if err == nil {
		t.Fatalf("配置文件类型不匹配时应返回 Unmarshal 错误")
	}
	contains(t, err.Error(), "解析配置失败")
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	resetConfigGlobals(t)
	pinNoisyEnvs(t)

	t.Setenv("APP_PORT", "7777")
	t.Setenv("GIN_MODE", "release")
	t.Setenv("DB_HOST", "env-db-host")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_USERNAME", "env-user")
	t.Setenv("DB_PASSWORD", "env-pass")
	t.Setenv("DB_DATABASE", "envdb")
	t.Setenv("DB_CHARSET", "latin1")
	t.Setenv("DB_PARSE_TIME", "false")
	t.Setenv("DB_LOC", "UTC")
	t.Setenv("GITEE_REPO_URL", "https://gitee.com/env/repo")
	t.Setenv("GITHUB_REPO_URL", "https://github.com/env/repo")
	t.Setenv("ALIYUN_REGISTRY", "env-registry.io")
	t.Setenv("ALIYUN_NAMESPACE", "env-ns")
	t.Setenv("ALIYUN_USERNAME", "env-ali-user")
	t.Setenv("ALIYUN_PASSWORD", "env-ali-pass")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("LOG_FILE_PATH", "/tmp/env.log")
	t.Setenv("JWT_SECRET", "env-jwt")
	t.Setenv("DEFAULT_ADMIN_USERNAME", "env-admin")
	t.Setenv("DEFAULT_ADMIN_PASSWORD", "env-admin-pw")

	// 配置文件中的值应被环境变量覆盖（优先级: 环境变量 > 配置文件 > 默认值）
	yaml := `
server:
  port: "1111"
database:
  host: file-host
aliyun:
  namespace: file-ns
`
	noErr(t, loadFromYAML(t, yaml))

	c := AppConfig
	eq(t, "7777", c.Server.Port)
	eq(t, "release", c.Server.Mode)
	eq(t, "env-db-host", c.Database.Host)
	eq(t, 3307, c.Database.Port)
	eq(t, "env-user", c.Database.Username)
	eq(t, "env-pass", c.Database.Password)
	eq(t, "envdb", c.Database.Database)
	eq(t, "latin1", c.Database.Charset)
	eq(t, false, c.Database.ParseTime)
	eq(t, "UTC", c.Database.Loc)
	eq(t, "https://gitee.com/env/repo", c.Git.Gitee.RepoURL)
	eq(t, "https://github.com/env/repo", c.Git.GitHub.RepoURL)
	eq(t, "env-registry.io", c.Aliyun.Registry)
	eq(t, "env-ns", c.Aliyun.Namespace)
	eq(t, "env-ali-user", c.Aliyun.Username)
	eq(t, "env-ali-pass", c.Aliyun.Password)
	eq(t, "warn", c.Log.Level)
	eq(t, "/tmp/env.log", c.Log.FilePath)
	eq(t, "env-jwt", c.Auth.JWTSecret)
	eq(t, "env-admin", c.Auth.DefaultAdminUsername)
	eq(t, "env-admin-pw", c.Auth.DefaultAdminPassword)
}

func TestLoadConfig_RepeatedLoadReplacesState(t *testing.T) {
	resetConfigGlobals(t)
	pinNoisyEnvs(t)

	// 第一次加载
	noErr(t, loadFromYAML(t, "server:\n  port: \"1000\"\n"))
	eq(t, "1000", AppConfig.Server.Port)

	// 第二次加载另一个文件： 应整体替换而非残留
	noErr(t, loadFromYAML(t, "server:\n  port: \"2000\"\nlog:\n  level: error\n"))
	eq(t, "2000", AppConfig.Server.Port)
	eq(t, "error", AppConfig.Log.Level)
}

// ---------------------------------------------------------------------------
// GetDSN
// ---------------------------------------------------------------------------

func TestGetDSN(t *testing.T) {
	c := &DatabaseConfig{
		Host: "localhost", Port: 3306, Username: "root", Password: "pass",
		Database: "test", Charset: "utf8mb4", ParseTime: true, Loc: "Local",
	}
	eq(t, "root:pass@tcp(localhost:3306)/test?charset=utf8mb4&parseTime=true&loc=Local", c.GetDSN())

	// 零值与特殊字符
	c = &DatabaseConfig{}
	eq(t, ":@tcp(:0)/?charset=&parseTime=false&loc=", c.GetDSN())

	c = &DatabaseConfig{Host: "h", Port: 1, Username: "u", Password: "p@:ss/word",
		Database: "中文库", Charset: "utf8", ParseTime: false, Loc: "Asia/Shanghai"}
	eq(t, "u:p@:ss/word@tcp(h:1)/中文库?charset=utf8&parseTime=false&loc=Asia/Shanghai", c.GetDSN())
}

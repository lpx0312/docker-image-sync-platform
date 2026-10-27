package utils

import (
	"testing"

	"docker-image-sync-platform/internal/models"
)

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		// 正常路径
		{"纯镜像名", "nginx", "nginx"},
		{"library 前缀", "library/nginx", "nginx"},
		{"带 tag", "nginx:1.20", "nginx"},
		{"带 registry 与 tag", "registry.cn-hangzhou.aliyuncs.com/ns/nginx:1.20", "nginx"},
		{"多级命名空间", "registry.io/a/b/c", "c"},
		{"最新 tag", "ubuntu:latest", "ubuntu"},

		// 边界
		{"空字符串", "", ""},
		{"纯空格", "   ", ""},
		{"带首尾空格", "  nginx  ", "nginx"},
		// "localhost:5000/nginx": 冒号后包含 /，不视为 tag（当前实际行为）
		{"registry 带端口无 tag", "localhost:5000/nginx", "nginx"},
		{"registry 带端口带 tag", "localhost:5000/nginx:1.2", "nginx"},
		// 多个冒号： 只处理最后一个，且其后再无 / 时截断（当前实际行为）
		{"多个冒号", "nginx:1.20:extra", "nginx:1.20"},
		{"unicode 镜像名", "命名空间/镜像", "镜像"},
		{"仅 tag 冒号结尾", "nginx:", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eq(t, tt.want, ExtractRepoName(tt.image))
		})
	}
}

func TestGenerateACRImage_Defaults(t *testing.T) {
	setupTestDB(t)
	// 无 system_configs 记录时使用默认 registry / namespace

	ref := GenerateACRImage("nginx", "1.20")
	eq(t, "registry.cn-hangzhou.aliyuncs.com/lpx03/nginx:1.20", ref)

	// tag 为空自动补 latest
	ref = GenerateACRImage("library/nginx", "")
	eq(t, "registry.cn-hangzhou.aliyuncs.com/lpx03/nginx:latest", ref)
}

func TestGenerateACRImageWithArchitecture_SystemConfig(t *testing.T) {
	setupTestDB(t)
	writeSystemConfig(t, "aliyun_registry", "registry.example.com", false)
	writeSystemConfig(t, "aliyun_namespace", "myns", false)

	ref := GenerateACRImageWithArchitecture("library/nginx", "1.20", "arm64")
	eq(t, "registry.example.com/myns/nginx:1.20-linux-arm64", ref)
}

func TestGenerateACRImageWithArchitecture_ArchMatrix(t *testing.T) {
	setupTestDB(t)

	const defaultReg = "registry.cn-hangzhou.aliyuncs.com/lpx03"
	tests := []struct {
		name  string
		image string
		tag   string
		arch  string
		want  string
	}{
		{"无架构", "nginx", "1.20", "", defaultReg + "/nginx:1.20"},
		{"amd64 无后缀", "nginx", "1.20", "amd64", defaultReg + "/nginx:1.20"},
		{"arm64", "nginx", "1.20", "arm64", defaultReg + "/nginx:1.20-linux-arm64"},
		{"arm", "nginx", "1.20", "arm", defaultReg + "/nginx:1.20-linux-arm"},
		{"386", "nginx", "1.20", "386", defaultReg + "/nginx:1.20-linux-386"},
		{"完整平台名", "nginx", "1.20", "linux/arm64", defaultReg + "/nginx:1.20-linux-arm64"},
		{"非 linux 平台", "nginx", "1.20", "windows/amd64", defaultReg + "/nginx:1.20-windows-amd64"},
		{"未知简化架构", "nginx", "1.20", "riscv64", defaultReg + "/nginx:1.20-linux-riscv64"},
		{"多级镜像名取最后段", "reg.io/a/b/c", "t", "", defaultReg + "/c:t"},
		{"空 tag 且非默认架构", "nginx", "", "arm64", defaultReg + "/nginx:latest-linux-arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eq(t, tt.want, GenerateACRImageWithArchitecture(tt.image, tt.tag, tt.arch))
		})
	}
}

func TestBuildACRImageRef_WithRegistryRow(t *testing.T) {
	db := setupTestDB(t)

	row := models.AcrRegistry{
		RegistryURL: "registry.custom.io",
		Namespace:   "custns",
		Username:    "u",
		Password:    "p",
	}
	noErr(t, db.Create(&row).Error)

	ref := BuildACRImageRef(row.ID, "library/nginx", "1.20")
	eq(t, "registry.custom.io/custns/nginx:1.20", ref)

	// tag 为空补 latest
	ref = BuildACRImageRef(row.ID, "nginx", "")
	eq(t, "registry.custom.io/custns/nginx:latest", ref)
}

func TestBuildACRImageRef_FallbackPaths(t *testing.T) {
	setupTestDB(t)

	// acrRegistryID = 0： 直接走 GenerateACRImage（默认配置）
	ref := BuildACRImageRef(0, "library/nginx", "1.20")
	eq(t, "registry.cn-hangzhou.aliyuncs.com/lpx03/nginx:1.20", ref)

	// 不存在的 ID： 数据库查询失败，回退到默认配置
	ref = BuildACRImageRef(99999, "nginx", "latest")
	eq(t, "registry.cn-hangzhou.aliyuncs.com/lpx03/nginx:latest", ref)
}

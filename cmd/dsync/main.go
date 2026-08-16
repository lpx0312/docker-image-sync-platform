// dsync 是 docker-image-sync-platform 的命令行客户端。
//
// 通过平台 REST API（/api/v1）完成登录、ACR/镜像/Tag 查询、
// 镜像同步提交与进度跟踪等操作，适用于日常运维与脚本自动化场景。
//
// 构建方式：
//
//	make cli
//	# 产物：bin/dsync
package main

import (
	"fmt"
	"os"
)

// 版本信息，由 Makefile 通过 -ldflags 注入
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	rootCmd.Version = fmt.Sprintf("%s (built %s)", version, buildTime)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

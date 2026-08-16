package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// 全局状态：CLI 配置与全局 flag
var (
	cfgPath string // --config 覆盖配置文件路径
	jsonOut bool   // --json 以 JSON 输出
	cfg     *Config
)

// rootCmd dsync 根命令
var rootCmd = &cobra.Command{
	Use:   "dsync",
	Short: "docker-image-sync-platform 命令行客户端",
	Long: `dsync 是 docker-image-sync-platform 的命令行客户端。

通过平台 API 完成 ACR/镜像/Tag 查询、镜像同步提交与进度跟踪。
首次使用请先执行 dsync login 登录。`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = loadConfig()
		if err != nil {
			return err
		}
		// login 自带 --server 参数，允许在未配置服务器时执行
		if cmd.Name() == "login" || cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
		}
		if cfg.Server == "" {
			return fmt.Errorf("尚未配置服务器地址，请先执行: dsync login --server http://<平台地址>:8080")
		}
		return nil
	},
}

// newClient 基于当前配置构建 API 客户端。
func newClient() *Client {
	return NewClient(cfg)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "配置文件路径（默认 ~/.config/dsync/config.json）")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "以 JSON 格式输出（便于脚本处理）")
}

// Execute 执行根命令。
func Execute() error {
	return rootCmd.Execute()
}

// emitOrPrint 统一输出入口：--json 时输出 JSON，否则执行文本渲染函数。
func emitOrPrint(v any, textRenderer func()) error {
	if jsonOut {
		return emitJSON(v)
	}
	textRenderer()
	return nil
}

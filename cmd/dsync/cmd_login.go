package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// loginCmd dsync login [--server URL]
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录平台并保存凭据",
	Long: `登录 docker-image-sync-platform，凭据保存在 ~/.config/dsync/config.json（权限 0600）。

token 过期后会自动用保存的密码重新登录。密码输入时不回显。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		server, _ := cmd.Flags().GetString("server")
		server = strings.TrimRight(server, "/")
		if server == "" {
			server = cfg.Server
		}
		if server == "" {
			return fmt.Errorf("请通过 --server 指定平台地址，例如: dsync login --server http://localhost:8080")
		}
		cfg.Server = server

		reader := bufio.NewReader(os.Stdin)
		username, _ := cmd.Flags().GetString("username")
		if username == "" {
			username = cfg.Username
		}
		if username == "" {
			fmt.Print("用户名: ")
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("读取用户名失败: %w", err)
			}
			username = strings.TrimSpace(line)
		}
		if username == "" {
			return fmt.Errorf("用户名不能为空")
		}

		// 密码来源: DSYNC_PASSWORD 环境变量（自动化场景） > 交互输入（不回显）
		password := os.Getenv("DSYNC_PASSWORD")
		if password == "" {
			fmt.Print("密码: ")
			raw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("读取密码失败: %w（非交互环境可设置 DSYNC_PASSWORD 环境变量）", err)
			}
			password = string(raw)
		}
		if password == "" {
			return fmt.Errorf("密码不能为空")
		}

		client := NewClient(cfg)
		result, err := client.Login(username, password)
		if err != nil {
			return err
		}
		cfg.Username = username
		cfg.Password = password
		if err := saveConfig(cfg); err != nil {
			return err
		}

		return emitOrPrint(result, func() {
			fmt.Printf("登录成功：%s", username)
			if role, ok := result.User["role_name"].(string); ok && role != "" {
				fmt.Printf("（角色: %s）", role)
			}
			fmt.Printf("\n服务器: %s\n", cfg.Server)
		})
	},
}

// whoamiCmd dsync whoami
var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "查看当前登录用户",
	RunE: func(cmd *cobra.Command, args []string) error {
		var user map[string]any
		if err := newClient().do("GET", "/auth/me", nil, &user); err != nil {
			return err
		}
		return emitOrPrint(user, func() {
			fmt.Printf("用户: %s\n", toString(user["username"]))
			if role := toString(user["role_name"]); role != "" {
				fmt.Printf("角色: %s\n", role)
			}
			if perms, ok := user["permissions"].([]any); ok {
				fmt.Printf("权限: %d 项\n", len(perms))
			}
		})
	},
}

// logoutCmd dsync logout
var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "登出并清除本地凭据",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Token != "" {
			// 服务端登出失败不阻塞本地清理
			if err := newClient().do("POST", "/auth/logout", nil, nil); err != nil {
				fmt.Fprintf(os.Stderr, "服务端登出失败（忽略）: %v\n", err)
			}
		}
		clearAuth(cfg)
		if err := saveConfig(cfg); err != nil {
			return err
		}
		fmt.Println("已登出，本地凭据已清除")
		return nil
	},
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func init() {
	loginCmd.Flags().String("server", "", "平台地址，如 http://localhost:8080")
	loginCmd.Flags().String("username", "", "用户名（不传则交互输入）")

	rootCmd.AddCommand(loginCmd, whoamiCmd, logoutCmd)
}

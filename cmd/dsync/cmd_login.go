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

// passwdCmd dsync passwd：修改当前用户密码
var passwdCmd = &cobra.Command{
	Use:   "passwd",
	Short: "修改当前用户密码",
	Long: `修改当前登录用户的密码。密码要求：至少 8 位，且包含大写字母、小写字母、数字、特殊字符。
修改成功后本地凭据会更新为新密码（便于后续自动续登），旧 token 将失效。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 旧密码来源：环境变量 > 交互输入（不回显）
		oldPassword := os.Getenv("DSYNC_OLD_PASSWORD")
		if oldPassword == "" {
			fmt.Print("原密码: ")
			raw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("读取原密码失败: %w（非交互环境可设置 DSYNC_OLD_PASSWORD 环境变量）", err)
			}
			oldPassword = string(raw)
		}
		if oldPassword == "" {
			return fmt.Errorf("原密码不能为空")
		}

		// 新密码来源：环境变量 > 交互输入（不回显）
		newPassword := os.Getenv("DSYNC_NEW_PASSWORD")
		if newPassword == "" {
			fmt.Print("新密码: ")
			raw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("读取新密码失败: %w（非交互环境可设置 DSYNC_NEW_PASSWORD 环境变量）", err)
			}
			newPassword = string(raw)
		}
		if newPassword == "" {
			return fmt.Errorf("新密码不能为空")
		}

		req := map[string]string{
			"old_password": oldPassword,
			"new_password": newPassword,
		}
		var resp map[string]any
		if err := newClient().do("PUT", "/auth/password", req, &resp); err != nil {
			return err
		}

		// 更新本地凭据为新密码，清除旧 token（已失效），便于后续自动续登
		cfg.Password = newPassword
		cfg.Token = ""
		cfg.TokenExpiresAt = ""
		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 本地凭据更新失败: %v（请手动 dsync login）\n", err)
		}

		return emitOrPrint(resp, func() {
			fmt.Println("密码修改成功，请重新登录（或下次操作将用新密码自动续登）")
		})
	},
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func init() {
	loginCmd.Flags().String("server", "", "平台地址，如 http://localhost:8080")
	loginCmd.Flags().String("username", "", "用户名（不传则交互输入）")

	rootCmd.AddCommand(loginCmd, whoamiCmd, logoutCmd, passwdCmd)
}

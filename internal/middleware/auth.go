package middleware

import (
	"net/http"
	"strings"

	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
)

// AuthRequired JWT 认证中间件
func AuthRequired(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证信息"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "认证格式错误"})
			c.Abort()
			return
		}

		claims, err := authService.ValidateToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "认证已过期，请重新登录"})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roleID", claims.RoleID)
		c.Set("roleCode", claims.RoleCode)
		c.Next()
	}
}

// PermissionRequired 权限校验中间件（需配合 AuthRequired 使用）
func PermissionRequired(roleService *services.RoleService, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleIDValue, exists := c.Get("roleID")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "无法获取用户角色"})
			c.Abort()
			return
		}

		roleID, ok := roleIDValue.(uint)
		if !ok || roleID == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "无法获取用户角色"})
			c.Abort()
			return
		}

		if !roleService.HasPermission(roleID, permission) {
			c.JSON(http.StatusForbidden, gin.H{"error": "没有访问该功能的权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// PermissionRequiredAny 满足任一权限即可访问
func PermissionRequiredAny(roleService *services.RoleService, permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleIDValue, exists := c.Get("roleID")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "无法获取用户角色"})
			c.Abort()
			return
		}

		roleID, ok := roleIDValue.(uint)
		if !ok || roleID == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "无法获取用户角色"})
			c.Abort()
			return
		}

		for _, permission := range permissions {
			if roleService.HasPermission(roleID, permission) {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "没有访问该功能的权限"})
		c.Abort()
	}
}

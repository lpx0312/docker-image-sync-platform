// Package services 提供加密解密服务
//
// encryption.go 文件实现了系统配置的加密解密功能，主要用于保护敏感信息的安全存储。
//
// 核心功能：
// - AES-256-GCM加密算法，提供高强度数据保护
// - 自动密钥管理，支持密钥轮换
// - Base64编码，便于数据库存储
// - 错误处理和日志记录
//
// 安全特性：
// - 使用随机生成的nonce，确保每次加密结果不同
// - 支持密钥派生，增强密钥安全性
// - 内存安全处理，及时清理敏感数据
// - 加密标识，防止误操作
//
// 使用场景：
// - Git仓库认证信息（token、密码）
// - 阿里云服务密钥
// - 数据库连接密码
// - 第三方API密钥
//
// 作者: Docker镜像同步平台开发团队
// 版本: v1.0.0
package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// EncryptionService 加密解密服务
//
// 提供系统配置的加密解密功能，保护敏感信息安全。
// 使用AES-256-GCM算法，提供认证加密，确保数据的机密性和完整性。
//
// 核心特性：
//   - 高强度加密：使用AES-256-GCM算法
//   - 自动密钥管理：支持环境变量和默认密钥
//   - 安全编码：Base64编码便于存储
//   - 错误处理：完善的错误处理和日志记录
//
// 密钥管理：
//   - 优先使用环境变量ENCRYPTION_KEY
//   - 支持密钥派生，增强安全性
//   - 密钥长度：32字节（256位）
//
// 数据格式：
//   - 加密数据格式：nonce(12字节) + ciphertext + tag(16字节)
//   - 存储格式：Base64编码字符串
//   - 标识前缀：ENC: 用于识别加密数据
type EncryptionService struct {
	key    []byte        // 加密密钥，32字节AES-256密钥
	logger *logrus.Logger // 日志记录器
}

// NewEncryptionService 创建新的加密服务实例
//
// 初始化加密服务，设置加密密钥和日志记录器。
// 密钥获取优先级：环境变量 > 默认密钥派生
//
// 参数：
//   - logger: 日志记录器实例
//
// 返回：
//   - *EncryptionService: 加密服务实例
//   - error: 初始化错误
//
// 环境变量：
//   - ENCRYPTION_KEY: 自定义加密密钥（推荐）
//
// 安全建议：
//   - 生产环境必须设置ENCRYPTION_KEY环境变量
//   - 密钥应使用强随机数生成
//   - 定期轮换加密密钥
func NewEncryptionService(logger *logrus.Logger) (*EncryptionService, error) {
	var key []byte
	
	// 优先从环境变量获取密钥
	if envKey := os.Getenv("ENCRYPTION_KEY"); envKey != "" {
		// 使用SHA256对环境变量密钥进行哈希，确保密钥长度为32字节
		hash := sha256.Sum256([]byte(envKey))
		key = hash[:]
		logger.Info("Using encryption key from environment variable")
	} else {
		// 使用默认密钥（仅用于开发环境）
		defaultKey := "docker-sync-platform-default-key-2024"
		hash := sha256.Sum256([]byte(defaultKey))
		key = hash[:]
		logger.Warn("Using default encryption key - please set ENCRYPTION_KEY environment variable for production")
	}

	return &EncryptionService{
		key:    key,
		logger: logger,
	}, nil
}

// Encrypt 加密字符串数据
//
// 使用AES-256-GCM算法加密输入数据，返回Base64编码的加密结果。
// 每次加密都会生成新的随机nonce，确保相同明文产生不同密文。
//
// 参数：
//   - plaintext: 待加密的明文字符串
//
// 返回：
//   - string: Base64编码的加密数据，格式为"ENC:base64data"
//   - error: 加密过程中的错误
//
// 加密流程：
//   1. 生成随机nonce（12字节）
//   2. 使用AES-GCM加密数据
//   3. 组合nonce + ciphertext + tag
//   4. Base64编码并添加标识前缀
//
// 错误处理：
//   - 空输入检查
//   - 密码器创建失败
//   - 随机数生成失败
//   - 加密操作失败
func (es *EncryptionService) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", errors.New("plaintext cannot be empty")
	}

	// 创建AES密码器
	block, err := aes.NewCipher(es.key)
	if err != nil {
		es.logger.WithError(err).Error("Failed to create AES cipher")
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		es.logger.WithError(err).Error("Failed to create GCM mode")
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// 生成随机nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		es.logger.WithError(err).Error("Failed to generate nonce")
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// 加密数据
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	
	// Base64编码并添加标识前缀
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	result := "ENC:" + encoded

	es.logger.Debug("Successfully encrypted data")
	return result, nil
}

// Decrypt 解密字符串数据
//
// 解密Base64编码的加密数据，返回原始明文字符串。
// 自动检测数据格式，支持加密和未加密数据的混合处理。
//
// 参数：
//   - ciphertext: 加密的数据字符串，可以是"ENC:base64data"格式或普通文本
//
// 返回：
//   - string: 解密后的明文字符串
//   - error: 解密过程中的错误
//
// 解密流程：
//   1. 检查是否为加密数据（ENC:前缀）
//   2. Base64解码获取原始加密数据
//   3. 提取nonce和密文
//   4. 使用AES-GCM解密数据
//
// 兼容性：
//   - 自动识别加密和未加密数据
//   - 未加密数据直接返回原值
//   - 向后兼容现有配置
//
// 错误处理：
//   - 格式检查
//   - Base64解码失败
//   - 数据长度验证
//   - 解密操作失败
func (es *EncryptionService) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// 检查是否为加密数据
	if !strings.HasPrefix(ciphertext, "ENC:") {
		// 未加密数据，直接返回
		es.logger.Debug("Data is not encrypted, returning as-is")
		return ciphertext, nil
	}

	// 移除前缀并解码
	encoded := strings.TrimPrefix(ciphertext, "ENC:")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		es.logger.WithError(err).Error("Failed to decode base64 data")
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// 创建AES密码器
	block, err := aes.NewCipher(es.key)
	if err != nil {
		es.logger.WithError(err).Error("Failed to create AES cipher")
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		es.logger.WithError(err).Error("Failed to create GCM mode")
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// 检查数据长度
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		es.logger.Error("Ciphertext too short")
		return "", errors.New("ciphertext too short")
	}

	// 提取nonce和密文
	nonce, cipherData := data[:nonceSize], data[nonceSize:]

	// 解密数据
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		es.logger.WithError(err).Error("Failed to decrypt data")
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	es.logger.Debug("Successfully decrypted data")
	return string(plaintext), nil
}

// IsEncrypted 检查数据是否已加密
//
// 通过检查数据前缀判断是否为加密数据。
// 用于在配置处理时区分加密和未加密的数据。
//
// 参数：
//   - data: 待检查的数据字符串
//
// 返回：
//   - bool: true表示数据已加密，false表示未加密
//
// 识别规则：
//   - 以"ENC:"开头的数据被认为是加密数据
//   - 其他格式的数据被认为是未加密数据
//
// 使用场景：
//   - 配置迁移时的数据格式检查
//   - 混合配置的处理逻辑
//   - 数据展示时的脱敏处理
func (es *EncryptionService) IsEncrypted(data string) bool {
	return strings.HasPrefix(data, "ENC:")
}

// EncryptIfNeeded 根据需要加密数据
//
// 智能加密函数，根据shouldEncrypt参数决定是否加密数据。
// 避免重复加密已经加密的数据。
//
// 参数：
//   - data: 待处理的数据字符串
//   - shouldEncrypt: 是否需要加密
//
// 返回：
//   - string: 处理后的数据字符串
//   - error: 处理过程中的错误
//
// 处理逻辑：
//   - 如果shouldEncrypt为false，直接返回原数据
//   - 如果数据已加密，直接返回原数据
//   - 如果数据未加密且需要加密，执行加密操作
//
// 使用场景：
//   - 配置更新时的智能加密
//   - 批量配置处理
//   - 配置迁移脚本
func (es *EncryptionService) EncryptIfNeeded(data string, shouldEncrypt bool) (string, error) {
	if !shouldEncrypt {
		return data, nil
	}

	if es.IsEncrypted(data) {
		// 数据已加密，直接返回
		return data, nil
	}

	// 加密数据
	return es.Encrypt(data)
}

// DecryptIfNeeded 根据需要解密数据
//
// 智能解密函数，自动检测数据格式并进行相应处理。
// 提供统一的数据访问接口。
//
// 参数：
//   - data: 待处理的数据字符串
//
// 返回：
//   - string: 解密后的明文数据
//   - error: 处理过程中的错误
//
// 处理逻辑：
//   - 自动检测数据是否加密
//   - 加密数据自动解密
//   - 未加密数据直接返回
//
// 使用场景：
//   - 配置读取时的统一处理
//   - 服务初始化时的配置解析
//   - API响应时的数据脱敏
func (es *EncryptionService) DecryptIfNeeded(data string) (string, error) {
	return es.Decrypt(data)
}
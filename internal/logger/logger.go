// Package logger 提供了基于Zap的高性能日志管理功能
//
// 本包负责：
// 1. 日志系统的初始化和配置
// 2. 日志文件的轮转和管理
// 3. 多输出目标支持（文件+控制台）
// 4. 结构化日志记录和格式化
// 5. 日志级别的动态控制
//
// 技术特性：
// - Zap: 高性能结构化日志库，支持JSON和控制台格式
// - Lumberjack: 日志文件轮转，支持大小、时间、数量限制
// - 多输出: 同时输出到文件和控制台，便于开发和生产
// - 结构化: 支持键值对格式，便于日志分析和监控
//
// 配置参数：
// - Level: 日志级别（debug/info/warn/error）
// - FilePath: 日志文件路径
// - MaxSize: 单个日志文件最大大小（MB）
// - MaxBackups: 保留的历史日志文件数量
// - MaxAge: 日志文件保留天数
//
// 使用方式：
//   // 初始化日志系统
//   if err := logger.InitLogger(); err != nil {
//       log.Fatal(err)
//   }
//   defer logger.Sync()
//
//   // 获取日志实例
//   log := logger.GetLogger()
//   log.Info("应用启动", zap.String("version", "1.0.0"))
//
// Author: Docker Image Sync Platform Team
// Version: 1.0.0
package logger

import (
	"os"
	"path/filepath"

	"docker-image-sync-platform/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 全局日志实例
// 使用Zap提供的高性能结构化日志功能
// 注意：这是一个全局变量，在应用启动时初始化，整个应用生命周期内复用
var Logger *zap.Logger

// InitLogger 初始化日志系统
//
// 功能说明：
// 1. 创建日志目录（如果不存在）
// 2. 配置日志文件轮转策略（基于Lumberjack）
// 3. 设置日志级别和编码格式
// 4. 创建多输出核心（文件+控制台）
// 5. 启用调用者信息和错误堆栈跟踪
//
// 日志轮转配置：
// - 基于文件大小自动轮转，避免单个文件过大
// - 保留指定数量的历史文件，节省磁盘空间
// - 自动压缩历史文件，减少存储占用
// - 基于时间清理过期日志，维护存储健康
//
// 输出格式：
// - 文件输出：JSON格式，便于日志分析和监控系统解析
// - 控制台输出：可读格式，便于开发调试和运维查看
//
// 返回值：
//   - error: 初始化失败时返回错误信息，成功时返回nil
//
// 注意事项：
//   - 必须在应用启动时调用，且只能调用一次
//   - 日志目录权限需要确保应用有写入权限
//   - 生产环境建议使用info级别以上，避免过多调试信息
func InitLogger() error {
	// ====================================================================
	// 第一步：确保日志目录存在
	// ====================================================================
	// 从配置文件路径中提取目录部分，确保日志文件的父目录存在
	// 使用0755权限创建目录，允许所有者读写执行，组和其他用户只读执行
	logDir := filepath.Dir(config.AppConfig.Log.FilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// ====================================================================
	// 第二步：配置日志文件轮转策略
	// ====================================================================
	// 使用Lumberjack库实现日志文件的自动轮转和管理
	// 轮转策略：基于文件大小、保留数量、保留时间的综合管理
	lumberjackLogger := &lumberjack.Logger{
		Filename:   config.AppConfig.Log.FilePath, // 日志文件路径
		MaxSize:    config.AppConfig.Log.MaxSize,  // 单个文件最大大小（MB），超过后自动轮转
		MaxBackups: config.AppConfig.Log.MaxBackups, // 保留的历史文件数量，超过后删除最旧的
		MaxAge:     config.AppConfig.Log.MaxAge,   // 文件保留天数，超过后自动删除
		Compress:   true,                          // 启用压缩，节省磁盘空间
	}

	// ====================================================================
	// 第三步：设置日志级别
	// ====================================================================
	// 根据配置文件中的日志级别字符串，转换为Zap的日志级别枚举
	// 支持debug、info、warn、error四个级别，默认为info级别
	var level zapcore.Level
	switch config.AppConfig.Log.Level {
	case "debug":
		level = zapcore.DebugLevel // 调试级别：输出所有日志，包括详细的调试信息
	case "info":
		level = zapcore.InfoLevel  // 信息级别：输出一般信息、警告和错误
	case "warn":
		level = zapcore.WarnLevel  // 警告级别：只输出警告和错误信息
	case "error":
		level = zapcore.ErrorLevel // 错误级别：只输出错误信息
	default:
		level = zapcore.InfoLevel  // 默认级别：当配置无效时使用info级别
	}

	// ====================================================================
	// 第四步：配置日志编码器
	// ====================================================================
	// 基于生产环境配置创建编码器，并自定义时间和级别的编码格式
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"                    // 时间字段名称
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder  // 使用ISO8601时间格式（YYYY-MM-DDTHH:mm:ss.sssZ）
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 使用大写字母表示日志级别（INFO、ERROR等）

	// ====================================================================
	// 第五步：创建多输出核心
	// ====================================================================
	// 使用Tee模式创建多个输出目标，同时写入文件和控制台
	// 这样既便于生产环境的日志收集，也便于开发环境的实时查看
	core := zapcore.NewTee(
		// 文件输出核心：使用JSON编码器，便于日志分析工具解析
		zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig), // JSON格式，结构化存储
			zapcore.AddSync(lumberjackLogger),     // 文件输出，支持轮转
			level,                                 // 指定的日志级别
		),
		// 控制台输出核心：使用控制台编码器，便于人类阅读
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig), // 控制台格式，易于阅读
			zapcore.AddSync(os.Stdout),               // 标准输出
			level,                                    // 指定的日志级别
		),
	)

	// ====================================================================
	// 第六步：创建最终的Logger实例
	// ====================================================================
	// 基于配置好的核心创建Logger，并添加额外功能
	Logger = zap.New(core, 
		zap.AddCaller(),                        // 添加调用者信息（文件名、行号、函数名）
		zap.AddStacktrace(zapcore.ErrorLevel),  // 在错误级别及以上添加堆栈跟踪
	)

	// 记录日志系统初始化完成的信息，包含关键配置参数
	Logger.Info("日志系统初始化完成",
		zap.String("level", config.AppConfig.Log.Level),      // 当前日志级别
		zap.String("file", config.AppConfig.Log.FilePath),    // 日志文件路径
	)

	return nil
}

// GetLogger 获取全局日志实例
//
// 功能说明：
//   - 返回已初始化的全局Logger实例
//   - 提供线程安全的日志记录功能
//   - 支持结构化日志记录和多种日志级别
//
// 返回值：
//   - *zap.Logger: Zap日志实例，如果未初始化则返回nil
//
// 使用场景：
//   - 在应用的各个模块中获取日志实例
//   - 记录业务逻辑、错误信息、性能指标等
//   - 替代标准库的log包，提供更强大的功能
//
// 注意事项：
//   - 必须先调用InitLogger()进行初始化
//   - 返回的实例是全局共享的，无需重复获取
//   - 建议在包级别获取一次，然后在包内复用
//
// 示例：
//   log := logger.GetLogger()
//   log.Info("用户登录", zap.String("username", "admin"), zap.String("ip", "192.168.1.1"))
//   log.Error("数据库连接失败", zap.Error(err), zap.String("dsn", dsn))
func GetLogger() *zap.Logger {
	return Logger
}

// Sync 同步日志缓冲区
//
// 功能说明：
//   - 强制将缓冲区中的日志数据写入到目标输出（文件、控制台）
//   - 确保所有日志记录都被持久化，防止数据丢失
//   - 在应用关闭前调用，保证日志完整性
//
// 使用场景：
//   - 应用程序优雅关闭时
//   - 关键操作完成后确保日志写入
//   - 定期同步以防止缓冲区溢出
//
// 注意事项：
//   - 这是一个阻塞操作，会等待所有日志写入完成
//   - 建议在defer语句中调用，确保程序退出前执行
//   - 如果Logger未初始化，函数会安全返回而不执行任何操作
//
// 示例：
//   func main() {
//       logger.InitLogger()
//       defer logger.Sync() // 确保程序退出前同步日志
//       
//       // 应用逻辑...
//   }
func Sync() {
	if Logger != nil {
		Logger.Sync()
	}
}
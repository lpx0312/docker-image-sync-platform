package logger

import (
	"os"
	"path/filepath"

	"docker-image-sync-platform/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.Logger

// InitLogger 初始化日志
func InitLogger() error {
	// 确保日志目录存在
	logDir := filepath.Dir(config.AppConfig.Log.FilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// 配置日志轮转
	lumberjackLogger := &lumberjack.Logger{
		Filename:   config.AppConfig.Log.FilePath,
		MaxSize:    config.AppConfig.Log.MaxSize,
		MaxBackups: config.AppConfig.Log.MaxBackups,
		MaxAge:     config.AppConfig.Log.MaxAge,
		Compress:   true,
	}

	// 设置日志级别
	var level zapcore.Level
	switch config.AppConfig.Log.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// 配置编码器
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// 创建核心
	core := zapcore.NewTee(
		// 文件输出
		zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(lumberjackLogger),
			level,
		),
		// 控制台输出
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			level,
		),
	)

	// 创建logger
	Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	Logger.Info("日志系统初始化完成",
		zap.String("level", config.AppConfig.Log.Level),
		zap.String("file", config.AppConfig.Log.FilePath),
	)

	return nil
}

// GetLogger 获取logger实例
func GetLogger() *zap.Logger {
	return Logger
}

// Sync 同步日志
func Sync() {
	if Logger != nil {
		Logger.Sync()
	}
}
package log

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
	LevelPanic
)

// String 返回日志级别的字符串表示
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	case LevelPanic:
		return "PANIC"
	default:
		return "UNKNOWN"
	}
}

// Entry 日志条目
type Entry struct {
	Time    time.Time
	Level   Level
	Message string
	Fields  map[string]interface{}
	// OrderedFields 用于保留字段输出顺序（严格按代码传入顺序）。
	// 适配器可优先使用该字段来格式化输出。
	OrderedFields []Field
	Caller        *CallerInfo
	Stack         string
	Ctx           context.Context
	LoggerKey     string
}

// Field 表示一个有序字段。
// - Key/Value: 常规键值对
// - IsExtra: 表示“未配对的单独 field”（只输出 Value，不输出 key）
type Field struct {
	Key     string
	Value   interface{}
	IsExtra bool
}

// CallerInfo 调用者信息
type CallerInfo struct {
	File     string
	Line     int
	Function string
}

// Logger 日志记录器接口
type Logger interface {
	// 写入日志
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
	Panic(msg string, fields ...interface{})

	// 格式化日志
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Panicf(format string, args ...interface{})

	// ln 风格日志
	Debugln(args ...interface{})
	Infoln(args ...interface{})
	Warnln(args ...interface{})
	Errorln(args ...interface{})
	Fatalln(args ...interface{})
	Panicln(args ...interface{})

	// 带上下文的日志
	DebugContext(ctx context.Context, msg string, fields ...interface{})
	InfoContext(ctx context.Context, msg string, fields ...interface{})
	WarnContext(ctx context.Context, msg string, fields ...interface{})
	ErrorContext(ctx context.Context, msg string, fields ...interface{})
	FatalContext(ctx context.Context, msg string, fields ...interface{})
	PanicContext(ctx context.Context, msg string, fields ...interface{})

	// 字段日志
	WithFields(fields map[string]interface{}) Logger
	With(key string, value interface{}) Logger

	// 配置
	SetLevel(level Level)
	SetConfig(config Config) error
	GetConfig() Config

	// 生命周期
	Close() error

	// 获取名称
	Name() string
}

// Config 日志配置
type Config struct {
	Level       Level
	Format      string // "text" 或 "json"
	Output      string // "stdout", "stderr", "file", "combined"
	File        FileConfig
	Caller      bool
	CallDepth   int
	TimeFormat  string
	Encoder     string // "text", "json", "pretty"
	Development bool
}

// FileConfig 文件输出配置
type FileConfig struct {
	Dir       string
	MaxSize   int // MB
	MaxAge    int // 天
	MaxBackup int
	Compress  bool
}

// DefaultConfig 返回默认日志配置
func DefaultConfig() Config {
	return Config{
		Level:      LevelInfo,
		Format:     "text",
		Output:     "file",
		File:       FileConfig{Dir: "./logs", MaxSize: 100, MaxAge: 30, MaxBackup: 7, Compress: false},
		Caller:     true,
		CallDepth:  3,
		TimeFormat: "2006-01-02 15:04:05",
		Encoder:    "text",
	}
}

// WriterAdapter 日志写入适配器
type WriterAdapter interface {
	Write(entry *Entry) error
	Sync() error
	Close() error
}

// Instance 适配器工厂函数
type Instance func() Logger

var (
	adaptersLock  sync.RWMutex
	adapters      = make(map[string]Instance)
	globalMu      sync.RWMutex
	globalLoggers = make(map[string]Logger)
)

// Register 注册日志适配器
func Register(name string, adapter Instance) {
	adaptersLock.Lock()
	defer adaptersLock.Unlock()
	if adapter == nil {
		panic("logger: Register adapter is nil")
	}
	if _, ok := adapters[name]; ok {
		panic("logger: Register called twice for adapter " + name)
	}
	adapters[name] = adapter
}

// Init 初始化全局日志记录器
func Init(config Config, name ...string) error {
	if len(name) == 0 {
		name = make([]string, 1)
		name[0] = "std"
	}
	adaptersLock.RLock()
	instanceFunc, ok := adapters[name[0]]
	adaptersLock.RUnlock()

	if !ok {
		return fmt.Errorf("logger: unknown adapter %q (forgot to import?)", name)
	}

	logger := instanceFunc()
	if config == (Config{}) {
		config = DefaultConfig()
	}
	if err := logger.SetConfig(config); err != nil {
		return err
	}

	globalMu.Lock()
	globalLoggers["default"] = logger
	globalMu.Unlock()

	return nil
}

// InitNamed 初始化命名日志记录器
func InitNamed(name string, adapterName string, config Config) error {
	adaptersLock.RLock()
	instanceFunc, ok := adapters[adapterName]
	adaptersLock.RUnlock()

	if !ok {
		return fmt.Errorf("logger: unknown adapter %q (forgot to import?)", adapterName)
	}

	logger := instanceFunc()
	if config == (Config{}) {
		config = DefaultConfig()
	}
	if err := logger.SetConfig(config); err != nil {
		return err
	}

	globalMu.Lock()
	globalLoggers[name] = logger
	globalMu.Unlock()

	return nil
}

// Get 获取全局日志记录器
func Get(name ...string) Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()

	key := "default"
	if len(name) > 0 {
		key = name[0]
	}

	if logger, ok := globalLoggers[key]; ok {
		return logger
	}
	return nil
}

// Debug 使用默认日志记录器记录 DEBUG 级别日志
func Debug(msg string, fields ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Debug(msg, fields...)
	}
}

// Debugf 使用默认日志记录器记录格式化 DEBUG 日志
func Debugf(format string, args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Debugf(format, args...)
	}
}

// Debugln 使用默认日志记录器记录 ln 风格 DEBUG 日志
func Debugln(args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Debugln(args...)
	}
}

// Info 使用默认日志记录器记录 INFO 级别日志
func Info(msg string, fields ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Info(msg, fields...)
	}
}

// Infof 使用默认日志记录器记录格式化 INFO 日志
func Infof(format string, args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Infof(format, args...)
	}
}

// Infoln 使用默认日志记录器记录 ln 风格 INFO 日志
func Infoln(args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Infoln(args...)
	}
}

// Warn 使用默认日志记录器记录 WARN 级别日志
func Warn(msg string, fields ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Warn(msg, fields...)
	}
}

// Warnf 使用默认日志记录器记录格式化 WARN 日志
func Warnf(format string, args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Warnf(format, args...)
	}
}

// Warnln 使用默认日志记录器记录 ln 风格 WARN 日志
func Warnln(args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Warnln(args...)
	}
}

// Error 使用默认日志记录器记录 ERROR 级别日志
func Error(msg string, fields ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Error(msg, fields...)
	}
}

// Errorf 使用默认日志记录器记录格式化 ERROR 日志
func Errorf(format string, args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Errorf(format, args...)
	}
}

// Errorln 使用默认日志记录器记录 ln 风格 ERROR 日志
func Errorln(args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Errorln(args...)
	}
}

// Fatal 使用默认日志记录器记录 FATAL 级别日志
func Fatal(msg string, fields ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Fatal(msg, fields...)
	}
}

// Fatalf 使用默认日志记录器记录格式化 FATAL 日志
func Fatalf(format string, args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Fatalf(format, args...)
	}
}

// Fatalln 使用默认日志记录器记录 ln 风格 FATAL 日志
func Fatalln(args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Fatalln(args...)
	}
}

// Panic 使用默认日志记录器记录 PANIC 级别日志
func Panic(msg string, fields ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Panic(msg, fields...)
	}
}

// Panicf 使用默认日志记录器记录格式化 PANIC 日志
func Panicf(format string, args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Panicf(format, args...)
	}
}

// Panicln 使用默认日志记录器记录 ln 风格 PANIC 日志
func Panicln(args ...interface{}) {
	if logger := Get(); logger != nil {
		logger.Panicln(args...)
	}
}

// Close 关闭所有日志记录器
func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	for _, logger := range globalLoggers {
		if err := logger.Close(); err != nil {
			return err
		}
	}
	globalLoggers = make(map[string]Logger)
	return nil
}

package std

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jiajia556/tool-box/log"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[37m"
)

// StdLogger 标准日志记录器实现
type StdLogger struct {
	mu        sync.Mutex
	level     log.Level
	config    log.Config
	writers   []io.Writer
	fields    map[string]interface{}
	callDepth int
}

// NewStdLogger 创建标准日志记录器
func NewStdLogger() log.Logger {
	return &StdLogger{
		level:     log.LevelInfo,
		fields:    make(map[string]interface{}),
		callDepth: 3,
	}
}

func (sl *StdLogger) log(level log.Level, msg string, fields ...interface{}) {
	if level < sl.level {
		return
	}

	sl.mu.Lock()
	defer sl.mu.Unlock()

	// 合并字段
	fieldMap := make(map[string]interface{})
	for k, v := range sl.fields {
		fieldMap[k] = v
	}

	// 解析额外字段
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			fieldMap[fields[i].(string)] = fields[i+1]
		}
	}

	// 获取调用者信息
	var caller *log.CallerInfo
	if sl.config.Caller {
		caller = sl.getCallerInfo(sl.callDepth + 1)
	}

	entry := &log.Entry{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
		Fields:  fieldMap,
		Caller:  caller,
	}

	sl.writeEntry(entry)

	// FATAL 级别退出
	if level == log.LevelFatal {
		os.Exit(1)
	}

	// PANIC 级别 panic
	if level == log.LevelPanic {
		panic(msg)
	}
}

func (sl *StdLogger) logContext(ctx context.Context, level log.Level, msg string, fields ...interface{}) {
	if level < sl.level {
		return
	}

	sl.mu.Lock()
	defer sl.mu.Unlock()

	fieldMap := make(map[string]interface{})
	for k, v := range sl.fields {
		fieldMap[k] = v
	}

	// 从上下文提取 trace id
	if traceID := ctx.Value("trace_id"); traceID != nil {
		fieldMap["trace_id"] = traceID
	}

	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			fieldMap[fields[i].(string)] = fields[i+1]
		}
	}

	var caller *log.CallerInfo
	if sl.config.Caller {
		caller = sl.getCallerInfo(sl.callDepth + 1)
	}

	entry := &log.Entry{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
		Fields:  fieldMap,
		Caller:  caller,
		Ctx:     ctx,
	}

	sl.writeEntry(entry)

	if level == log.LevelFatal {
		os.Exit(1)
	}

	if level == log.LevelPanic {
		panic(msg)
	}
}

func (sl *StdLogger) getCallerInfo(depth int) *log.CallerInfo {
	pc, file, line, ok := runtime.Caller(depth)
	if !ok {
		return nil
	}

	fn := runtime.FuncForPC(pc)
	funcName := fn.Name()
	if idx := strings.LastIndex(funcName, "/"); idx >= 0 {
		funcName = funcName[idx+1:]
	}

	return &log.CallerInfo{
		File:     filepath.Base(file),
		Line:     line,
		Function: funcName,
	}
}

func (sl *StdLogger) writeEntry(entry *log.Entry) {
	var output string

	switch sl.config.Encoder {
	case "json":
		output = sl.formatJSON(entry)
	case "pretty":
		output = sl.formatPretty(entry)
	default:
		output = sl.formatText(entry)
	}

	for _, w := range sl.writers {
		fmt.Fprint(w, output)
	}
}

func (sl *StdLogger) formatText(entry *log.Entry) string {
	timeStr := entry.Time.Format(sl.config.TimeFormat)
	if timeStr == "" {
		timeStr = entry.Time.Format("2006-01-02 15:04:05")
	}

	levelStr := entry.Level.String()
	msg := timeStr + " " + levelStr + " " + entry.Message

	if entry.Caller != nil {
		msg += fmt.Sprintf(" (%s:%d)", entry.Caller.File, entry.Caller.Line)
	}

	if len(entry.Fields) > 0 {
		msg += " " + sl.formatFields(entry.Fields)
	}

	return msg + "\n"
}

func (sl *StdLogger) formatPretty(entry *log.Entry) string {
	timeStr := entry.Time.Format(sl.config.TimeFormat)
	if timeStr == "" {
		timeStr = entry.Time.Format("2006-01-02 15:04:05")
	}

	levelStr := entry.Level.String()
	levelColor := sl.getLevelColor(entry.Level)

	msg := fmt.Sprintf("%s [%s%s%s] %s", timeStr, levelColor, levelStr, colorReset, entry.Message)

	if entry.Caller != nil {
		msg += fmt.Sprintf(" %s(%s:%d)%s", colorGray, entry.Caller.File, entry.Caller.Line, colorReset)
	}

	if len(entry.Fields) > 0 {
		msg += " " + sl.formatFields(entry.Fields)
	}

	return msg + "\n"
}

func (sl *StdLogger) formatJSON(entry *log.Entry) string {
	// 简化的 JSON 格式，实际使用中可用 encoding/json
	timeStr := entry.Time.Format(sl.config.TimeFormat)
	if timeStr == "" {
		timeStr = entry.Time.Format("2006-01-02T15:04:05Z07:00")
	}

	fields := fmt.Sprintf("{\"timestamp\":\"%s\",\"level\":\"%s\",\"message\":\"%s\"",
		timeStr, entry.Level.String(), escapeJSON(entry.Message))

	if entry.Caller != nil {
		fields += fmt.Sprintf(",\"caller\":\"%s:%d\"", entry.Caller.File, entry.Caller.Line)
	}

	for k, v := range entry.Fields {
		fields += fmt.Sprintf(",\"%s\":%v", k, v)
	}

	fields += "}\n"
	return fields
}

func (sl *StdLogger) formatFields(fields map[string]interface{}) string {
	var parts []string
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " ")
}

func (sl *StdLogger) getLevelColor(level log.Level) string {
	switch level {
	case log.LevelDebug:
		return colorBlue
	case log.LevelInfo:
		return colorGreen
	case log.LevelWarn:
		return colorYellow
	case log.LevelError, log.LevelFatal, log.LevelPanic:
		return colorRed
	default:
		return colorReset
	}
}

func escapeJSON(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "\"", "\\\"")
}

// 实现 Logger 接口方法
func (sl *StdLogger) Debug(msg string, fields ...interface{}) {
	sl.log(log.LevelDebug, msg, fields...)
}
func (sl *StdLogger) Info(msg string, fields ...interface{}) {
	sl.log(log.LevelInfo, msg, fields...)
}
func (sl *StdLogger) Warn(msg string, fields ...interface{}) {
	sl.log(log.LevelWarn, msg, fields...)
}
func (sl *StdLogger) Error(msg string, fields ...interface{}) {
	sl.log(log.LevelError, msg, fields...)
}
func (sl *StdLogger) Fatal(msg string, fields ...interface{}) {
	sl.log(log.LevelFatal, msg, fields...)
}
func (sl *StdLogger) Panic(msg string, fields ...interface{}) {
	sl.log(log.LevelPanic, msg, fields...)
}
func (sl *StdLogger) DebugContext(ctx context.Context, msg string, fields ...interface{}) {
	sl.logContext(ctx, log.LevelDebug, msg, fields...)
}
func (sl *StdLogger) InfoContext(ctx context.Context, msg string, fields ...interface{}) {
	sl.logContext(ctx, log.LevelInfo, msg, fields...)
}
func (sl *StdLogger) WarnContext(ctx context.Context, msg string, fields ...interface{}) {
	sl.logContext(ctx, log.LevelWarn, msg, fields...)
}
func (sl *StdLogger) ErrorContext(ctx context.Context, msg string, fields ...interface{}) {
	sl.logContext(ctx, log.LevelError, msg, fields...)
}
func (sl *StdLogger) FatalContext(ctx context.Context, msg string, fields ...interface{}) {
	sl.logContext(ctx, log.LevelFatal, msg, fields...)
}
func (sl *StdLogger) PanicContext(ctx context.Context, msg string, fields ...interface{}) {
	sl.logContext(ctx, log.LevelPanic, msg, fields...)
}

func (sl *StdLogger) WithFields(fields map[string]interface{}) log.Logger {
	sl.mu.Lock()
	newFields := make(map[string]interface{})
	for k, v := range sl.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}
	sl.mu.Unlock()

	newLogger := *sl
	newLogger.fields = newFields
	return &newLogger
}

func (sl *StdLogger) With(key string, value interface{}) log.Logger {
	return sl.WithFields(map[string]interface{}{key: value})
}

func (sl *StdLogger) SetLevel(level log.Level) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.level = level
}

func (sl *StdLogger) SetConfig(config log.Config) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	sl.config = config
	sl.level = config.Level

	// 配置输出目标
	switch config.Output {
	case "stderr":
		sl.writers = []io.Writer{os.Stderr}
	case "file":
		f, err := os.OpenFile(config.File.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		sl.writers = []io.Writer{f}
	case "combined":
		f, err := os.OpenFile(config.File.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		sl.writers = []io.Writer{os.Stdout, f}
	default: // stdout
		sl.writers = []io.Writer{os.Stdout}
	}

	if config.CallDepth > 0 {
		sl.callDepth = config.CallDepth
	}

	return nil
}

func (sl *StdLogger) GetConfig() log.Config {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.config
}

func (sl *StdLogger) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for _, w := range sl.writers {
		if f, ok := w.(*os.File); ok {
			f.Close()
		}
	}
	return nil
}

func (sl *StdLogger) Name() string {
	return "std"
}

func init() {
	log.Register("std", NewStdLogger)
}

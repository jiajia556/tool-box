package std

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

// 内部保留 key：用于保存“未配对的单个 field”。
// 注意：输出时不会打印这个 key（text/pretty 会只打印 value；json 会输出到 "extras" 字段）。
const unpairedFieldKey = "__unpaired_field"

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
	defaultConfig := log.DefaultConfig()
	return &StdLogger{
		level:     defaultConfig.Level,
		config:    defaultConfig,
		writers:   []io.Writer{os.Stdout},
		fields:    make(map[string]interface{}),
		callDepth: defaultConfig.CallDepth,
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
	orderedFields := make([]log.Field, 0, len(sl.fields)+len(fields)/2+1)
	// 先放入 logger 自带字段（map 无法保证顺序，这里做一个稳定排序，确保输出确定性）
	if len(sl.fields) > 0 {
		keys := make([]string, 0, len(sl.fields))
		for k := range sl.fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := sl.fields[k]
			fieldMap[k] = v
			orderedFields = append(orderedFields, log.Field{Key: k, Value: v})
		}
	}

	// 解析额外字段（严格按传入顺序追加）
	orderedFields = append(orderedFields, mergeFields(fieldMap, fields...)...)

	// 获取调用者信息
	var caller *log.CallerInfo
	if sl.config.Caller {
		caller = sl.getCallerInfo(sl.callDepth + 1)
	}

	entry := &log.Entry{
		Time:          time.Now(),
		Level:         level,
		Message:       msg,
		Fields:        fieldMap,
		OrderedFields: orderedFields,
		Caller:        caller,
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
	orderedFields := make([]log.Field, 0, len(sl.fields)+len(fields)/2+2)
	if len(sl.fields) > 0 {
		keys := make([]string, 0, len(sl.fields))
		for k := range sl.fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := sl.fields[k]
			fieldMap[k] = v
			orderedFields = append(orderedFields, log.Field{Key: k, Value: v})
		}
	}

	// 从上下文提取 trace id
	if traceID := ctx.Value("trace_id"); traceID != nil {
		fieldMap["trace_id"] = traceID
		orderedFields = append(orderedFields, log.Field{Key: "trace_id", Value: traceID})
	}

	orderedFields = append(orderedFields, mergeFields(fieldMap, fields...)...)

	var caller *log.CallerInfo
	if sl.config.Caller {
		caller = sl.getCallerInfo(sl.callDepth + 1)
	}

	entry := &log.Entry{
		Time:          time.Now(),
		Level:         level,
		Message:       msg,
		Fields:        fieldMap,
		OrderedFields: orderedFields,
		Caller:        caller,
		Ctx:           ctx,
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

	// 获取工作目录
	wd, err := os.Getwd()
	if err == nil {
		baseDir := wd
		if root, ok := findProjectRoot(wd); ok {
			baseDir = root
		}
		if rel, err := filepath.Rel(baseDir, file); err == nil {
			file = rel
		}
	}

	return &log.CallerInfo{
		File:     filepath.ToSlash(file),
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

	writers := sl.writers
	// 当日志级别为 DEBUG 时，无论输出配置是什么，都同时输出到控制台(stdout)。
	if entry.Level == log.LevelDebug && !hasWriter(writers, os.Stdout) {
		writers = append(append([]io.Writer(nil), writers...), os.Stdout)
	}

	for _, w := range writers {
		if dfw, ok := w.(*dailyFileWriter); ok {
			_ = dfw.ensureForTime(entry.Time)
		}
	}

	for _, w := range writers {
		_, _ = fmt.Fprint(w, output)
	}
}

func hasWriter(writers []io.Writer, target io.Writer) bool {
	for _, w := range writers {
		if w == target {
			return true
		}
	}
	return false
}

func (sl *StdLogger) formatText(entry *log.Entry) string {
	timeStr := entry.Time.Format(sl.config.TimeFormat)
	if timeStr == "" {
		timeStr = entry.Time.Format("2006-01-02 15:04:05")
	}

	levelStr := entry.Level.String()
	msg := timeStr + " " + levelStr + " " + entry.Message
	if entry.Caller != nil {
		// 将调用位置放到最前边
		msg = fmt.Sprintf("(%s:%d) %s", entry.Caller.File, entry.Caller.Line, msg)
	}

	if len(entry.Fields) > 0 {
		msg += " " + sl.formatFields(entry)
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
		// 将调用位置放到最前边
		msg = fmt.Sprintf("%s(%s:%d)%s %s", colorGray, entry.Caller.File, entry.Caller.Line, colorReset, msg)
	}

	if len(entry.Fields) > 0 {
		msg += " " + sl.formatFields(entry)
	}

	return msg + "\n"
}

func (sl *StdLogger) formatJSON(entry *log.Entry) string {
	timeStr := entry.Time.Format(sl.config.TimeFormat)
	if timeStr == "" {
		timeStr = entry.Time.Format("2006-01-02T15:04:05Z07:00")
	}

	// JSON 输出：使用有序字段构造 JSON 字符串，保证输出顺序稳定且尽量贴近调用顺序。
	// 说明：JSON 语义上对象 key 无序，但这里保证输出字符串顺序。
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	writeKV := func(k string, v interface{}) bool {
		kb, err := json.Marshal(k)
		if err != nil {
			return false
		}
		vb, err := json.Marshal(v)
		if err != nil {
			return false
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
		return true
	}

	if !writeKV("timestamp", timeStr) || !writeKV("level", entry.Level.String()) || !writeKV("message", entry.Message) {
		return sl.formatText(entry)
	}
	if entry.Caller != nil {
		if !writeKV("caller", fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)) {
			return sl.formatText(entry)
		}
	}

	// 常规 key=value 按顺序输出；extra 收集后放到 extras
	var extras []interface{}
	if len(entry.OrderedFields) > 0 {
		for _, f := range entry.OrderedFields {
			if f.IsExtra {
				extras = append(extras, f.Value)
				continue
			}
			if f.Key == "" {
				continue
			}
			if !writeKV(f.Key, f.Value) {
				return sl.formatText(entry)
			}
		}
	} else if len(entry.Fields) > 0 {
		// 兼容：没有 OrderedFields 时仍输出 map（顺序不可控）
		for k, v := range entry.Fields {
			if k == unpairedFieldKey {
				extras = append(extras, v)
				continue
			}
			if !writeKV(k, v) {
				return sl.formatText(entry)
			}
		}
	}

	if len(extras) == 1 {
		if !writeKV("extras", extras[0]) {
			return sl.formatText(entry)
		}
	} else if len(extras) > 1 {
		if !writeKV("extras", extras) {
			return sl.formatText(entry)
		}
	}

	buf.WriteByte('}')
	buf.WriteByte('\n')
	return buf.String()
}

func (sl *StdLogger) formatFields(entry *log.Entry) string {
	// 优先使用有序字段输出
	if len(entry.OrderedFields) > 0 {
		parts := make([]string, 0, len(entry.OrderedFields))
		for _, f := range entry.OrderedFields {
			if f.IsExtra {
				parts = append(parts, fmt.Sprintf("%v", f.Value))
				continue
			}
			if f.Key == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%v", f.Key, f.Value))
		}
		return strings.Join(parts, " ")
	}

	// 兼容：没有 OrderedFields 时回退 map（顺序不可控）
	var parts []string
	var extras []string
	for k, v := range entry.Fields {
		if k == unpairedFieldKey {
			extras = append(extras, fmt.Sprintf("%v", v))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	parts = append(parts, extras...)
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
func (sl *StdLogger) Debugf(format string, args ...interface{}) {
	sl.log(log.LevelDebug, fmt.Sprintf(format, args...))
}
func (sl *StdLogger) Infof(format string, args ...interface{}) {
	sl.log(log.LevelInfo, fmt.Sprintf(format, args...))
}
func (sl *StdLogger) Warnf(format string, args ...interface{}) {
	sl.log(log.LevelWarn, fmt.Sprintf(format, args...))
}
func (sl *StdLogger) Errorf(format string, args ...interface{}) {
	sl.log(log.LevelError, fmt.Sprintf(format, args...))
}
func (sl *StdLogger) Fatalf(format string, args ...interface{}) {
	sl.log(log.LevelFatal, fmt.Sprintf(format, args...))
}
func (sl *StdLogger) Panicf(format string, args ...interface{}) {
	sl.log(log.LevelPanic, fmt.Sprintf(format, args...))
}
func (sl *StdLogger) Debugln(args ...interface{}) {
	sl.log(log.LevelDebug, lnMessage(args...))
}
func (sl *StdLogger) Infoln(args ...interface{}) {
	sl.log(log.LevelInfo, lnMessage(args...))
}
func (sl *StdLogger) Warnln(args ...interface{}) {
	sl.log(log.LevelWarn, lnMessage(args...))
}
func (sl *StdLogger) Errorln(args ...interface{}) {
	sl.log(log.LevelError, lnMessage(args...))
}
func (sl *StdLogger) Fatalln(args ...interface{}) {
	sl.log(log.LevelFatal, lnMessage(args...))
}
func (sl *StdLogger) Panicln(args ...interface{}) {
	sl.log(log.LevelPanic, lnMessage(args...))
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
	defer sl.mu.Unlock()

	newFields := make(map[string]interface{}, len(sl.fields)+len(fields))
	for k, v := range sl.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	// 不要复制 mutex（复制后可能导致未定义行为），直接构造一个新的 logger。
	newWriters := append([]io.Writer(nil), sl.writers...)
	return &StdLogger{
		level:     sl.level,
		config:    sl.config,
		writers:   newWriters,
		fields:    newFields,
		callDepth: sl.callDepth,
	}
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

	// 先关闭旧的文件 writer，避免配置切换时句柄泄漏。
	sl.closeOwnedWritersLocked()

	sl.config = config
	sl.level = config.Level
	if sl.config.File.Dir == "" {
		sl.config.File.Dir = "./logs"
	}

	// 配置输出目标
	switch config.Output {
	case "stderr":
		sl.writers = []io.Writer{os.Stderr}
	case "file":
		sl.writers = []io.Writer{newDailyFileWriter(sl.config.File.Dir)}
	case "combined":
		sl.writers = []io.Writer{os.Stdout, newDailyFileWriter(sl.config.File.Dir)}
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
	sl.closeOwnedWritersLocked()
	sl.writers = nil
	return nil
}

func (sl *StdLogger) Name() string {
	return "std"
}

func init() {
	log.Register("std", NewStdLogger)
}

func mergeFields(fieldMap map[string]interface{}, fields ...interface{}) []log.Field {
	ordered := make([]log.Field, 0, len(fields)/2+1)
	var unpaired interface{}
	hasUnpaired := false
	if len(fields)%2 == 1 {
		extra := fields[len(fields)-1]
		unpaired = extra
		hasUnpaired = true
		fields = fields[:len(fields)-1]
		if existing, ok := fieldMap[unpairedFieldKey]; ok {
			switch v := existing.(type) {
			case []interface{}:
				fieldMap[unpairedFieldKey] = append(v, extra)
			default:
				fieldMap[unpairedFieldKey] = []interface{}{v, extra}
			}
		} else {
			fieldMap[unpairedFieldKey] = extra
		}
	}

	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			break
		}
		key, ok := fields[i].(string)
		if !ok || key == "" {
			continue
		}
		fieldMap[key] = fields[i+1]
		ordered = append(ordered, log.Field{Key: key, Value: fields[i+1]})
	}

	// 孤立 field 严格保持在最后输出
	if hasUnpaired {
		ordered = append(ordered, log.Field{IsExtra: true, Value: unpaired})
	}

	return ordered
}

func (sl *StdLogger) closeOwnedWritersLocked() {
	for _, w := range sl.writers {
		if dfw, ok := w.(*dailyFileWriter); ok {
			_ = dfw.Close()
			continue
		}
		f, ok := w.(*os.File)
		if !ok {
			continue
		}
		if f == os.Stdout || f == os.Stderr {
			continue
		}
		_ = f.Close()
	}
}

type dailyFileWriter struct {
	mu          sync.Mutex
	dir         string
	currentDate string
	file        *os.File
}

func newDailyFileWriter(dir string) *dailyFileWriter {
	if dir == "" {
		dir = "./logs"
	}
	return &dailyFileWriter{dir: dir}
}

func (w *dailyFileWriter) ensureForTime(t time.Time) error {
	dateStr := t.Format("2006-01-02")
	if w.file != nil && w.currentDate == dateStr {
		return nil
	}

	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(w.dir, dateStr+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	if w.file != nil {
		_ = w.file.Close()
	}

	w.file = f
	w.currentDate = dateStr
	return nil
}

func (w *dailyFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.ensureForTime(time.Now()); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.currentDate = ""
	return err
}

func lnMessage(args ...interface{}) string {
	return strings.TrimRight(fmt.Sprintln(args...), "\n")
}

func findProjectRoot(start string) (string, bool) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

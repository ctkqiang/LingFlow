package utilities

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	APP_NAME = "LingFlow"
	VERSION  = "1.0.0"
	TZ       = "CST"
)

// LogLevel 控制输出的最低日志级别。
type LogLevel int

const (
	DEBUG   LogLevel = iota // 详细诊断信息（仅限本地开发）
	INFO                    // 通用运行消息（默认级别）
	WARN                    // 可恢复的问题，降级模式
	ERROR                   // 需要关注的故障
	VERBOSE                 // 逐请求指标，审计追踪
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case VERBOSE:
		return "VERBOSE"
	default:
		return "UNKNOWN"
	}
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPink   = "\033[35m"
	colorGreen  = "\033[32m"
	colorBold   = "\033[1m"
	colorNoBold = "\033[22m"
)

var (
	CurrentLevel   = INFO
	startTime      = time.Now()
	errorCallback  func(string)
	goroutineSeq   int
	goroutineMutex sync.Mutex

	// CloudWatchMode 为 true 时，所有日志函数在输出人类可读格式的同时，
	// 也会向 stderr 输出一行 CloudWatch 兼容的 JSON 结构化日志。
	CloudWatchMode bool
)

// SetLogLevel 解析字符串级别名称并设置全局阈值。
// 无法识别的输入默认为 INFO。通过 LOG_LEVEL 环境变量在 init() 中自动调用。
func SetLogLevel(s string) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		CurrentLevel = DEBUG
	case "INFO":
		CurrentLevel = INFO
	case "WARN":
		CurrentLevel = WARN
	case "ERROR":
		CurrentLevel = ERROR
	case "VERBOSE":
		CurrentLevel = VERBOSE
	default:
		CurrentLevel = INFO
	}
}

// RegisterErrorCallback 设置每次 ERROR 日志行触发时调用的回调函数。
func RegisterErrorCallback(callback func(string)) { errorCallback = callback }

// Bold 使用 ANSI 粗体转义序列包裹文本，用于日志行中的强调。
func Bold(text string) string { return colorBold + text + colorNoBold }

// Error 输出 ERROR 级别的日志。在 INFO 及以上级别可见。
func Error(format string, a ...interface{}) { Log(ERROR, format, a...) }

// Info 输出 INFO 级别的日志。
func Info(format string, a ...interface{}) { Log(INFO, format, a...) }

// Debug 输出 DEBUG 级别的日志。仅在 LOG_LEVEL=DEBUG 时可见。
func Debug(format string, a ...interface{}) { Log(DEBUG, format, a...) }

// Warn 输出 WARN 级别的日志。仅在 WARN 及以上级别可见。
func Warn(format string, a ...interface{}) { Log(WARN, format, a...) }

// Log 是简单的单行日志记录器。结构化操作日志优先使用 Logf；
// 临时消息使用 Log。
func Log(level LogLevel, format string, a ...interface{}) {
	if level < CurrentLevel {
		return
	}
	formattedMessage := fmt.Sprintf(format, a...)
	logLine := fmt.Sprintf("[%s] [%s] [%s] %s",
		APP_NAME, time.Now().Format("2006-01-02 15:04:05"), level.String(), formattedMessage)
	logColor := levelColor(level)
	if logColor != "" {
		fmt.Fprintf(os.Stderr, "%s%s%s\n", logColor, logLine, colorReset)
	} else {
		fmt.Fprintln(os.Stderr, logLine)
	}
	if level == ERROR && errorCallback != nil {
		errorCallback(logLine)
	}

	// CloudWatch 双输出：简单日志也同时输出 JSON 格式
	if CloudWatchMode {
		emitCloudWatchJSON("general", "log", level, "LOG", formattedMessage, 0, nil)
	}
}

// Logf 输出带有标准字段（Status、Type、Memory、Routine、Elapsed）的结构化块日志，
// 后跟调用方提供的 key=value 详情。这是操作日志的主要函数。
func Logf(component, operation string, level LogLevel, status string, elapsed time.Duration, details ...string) {
	if level < CurrentLevel {
		return
	}
	taskIdentifier := nextTaskID()
	callerFunctionName := callerName(3)
	heapAllocationMegabytes := heapAllocMB()

	header := fmt.Sprintf("[%s@%s]::%s:: (%s:%s>>%s::%s)",
		APP_NAME, nowCompact(), level.String(), component, operation, taskIdentifier, callerFunctionName)

	logRows := [][]string{
		{"Status", status},
		{"Type", "ACTION"},
		{"Memory", fmt.Sprintf("%.2fMB", heapAllocationMegabytes)},
		{"Routine", taskIdentifier},
		{"Elapsed", fmtElapsed(elapsed)},
	}
	for _, detail := range details {
		detailKey, detailValue, ok := strings.Cut(detail, "=")
		if ok {
			logRows = append(logRows, []string{strings.TrimSpace(detailKey), strings.TrimSpace(detailValue)})
		} else {
			logRows = append(logRows, []string{detail, ""})
		}
	}

	logColor := levelColor(level)
	fmt.Fprint(os.Stderr, buildBlock(header, logColor, logRows))

	if level == ERROR && errorCallback != nil {
		errorCallback(header + " " + status)
	}

	// CloudWatch 双输出：在人类可读格式之外，同时输出 JSON 结构化日志
	if CloudWatchMode {
		emitCloudWatchJSON(component, operation, level, status, header, elapsed, details)
	}
}

// LogProgress 输出 INFO 级别的 IN_PROGRESS 日志，用于中间检查点
// （启动阶段、长时间运行步骤等）。
func LogProgress(component, operation, msg string, details ...string) {
	resolved := msg
	if strings.Contains(msg, "%") && len(details) > 0 {
		verbCount := strings.Count(msg, "%s") + strings.Count(msg, "%d") + strings.Count(msg, "%v")
		if verbCount > 0 && verbCount <= len(details) {
			args := make([]interface{}, verbCount)
			for i := 0; i < verbCount; i++ {
				args[i] = details[i]
			}
			resolved = fmt.Sprintf(msg, args...)
			details = details[verbCount:]
		}
	}
	all := append([]string{"Progress=" + resolved}, details...)
	Logf(component, operation, INFO, "IN_PROGRESS", 0, all...)
}

// LogStart 为操作输出 START 标记。
func LogStart(component, operation string) {
	Logf(component, operation, INFO, "START", 0)
}

// LogSuccess 输出带有耗时的 OK 标记。
func LogSuccess(component, operation string, elapsed time.Duration, details ...string) {
	Logf(component, operation, INFO, "OK", elapsed, details...)
}

// LogError 输出带有错误信息的 FAIL 标记。
func LogError(component, operation string, err error, elapsed time.Duration, details ...string) {
	all := append([]string{"Error=" + err.Error()}, details...)
	Logf(component, operation, ERROR, "FAIL", elapsed, all...)
}

// LogWarn 输出 WARN 标记。
func LogWarn(component, operation, msg string, elapsed time.Duration, details ...string) {
	all := append([]string{"Warn=" + msg}, details...)
	Logf(component, operation, WARN, "WARN", elapsed, all...)
}

// Mask 脱敏敏感值，显示前几个字符后跟 [REDACTED]。
// 适用于日志中的令牌、密钥和个人身份信息。
func Mask(s string) string {
	runes := []rune(s)
	if len(runes) <= 4 {
		return "****"
	}
	visibleCharacterCount := 10
	if len(runes) <= visibleCharacterCount {
		visibleCharacterCount = len(runes) / 3
	}
	return string(runes[:visibleCharacterCount]) + "[REDACTED]"
}

// RetryWithBackoff 以固定退避间隔执行操作，最多重试 maxAttempts 次。
// 耗尽重试次数后返回最后一个错误。
func RetryWithBackoff(name string, maxAttempts int, backoff time.Duration, fn func() error) error {
	var lastError error
	for attemptIndex := 0; attemptIndex < maxAttempts; attemptIndex++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastError = err
			Warn("%s attempt %d/%d failed: %v — retrying in %v", name, attemptIndex+1, maxAttempts, err, backoff)
			time.Sleep(backoff)
		}
	}
	return fmt.Errorf("%s: exhausted %d retries: %w", name, maxAttempts, lastError)
}

func init() {
	SetLogLevel(os.Getenv("LOG_LEVEL"))
	// 如果环境变量 CLOUDWATCH_LOGGING=true，自动启用 CloudWatch JSON 模式
	if strings.EqualFold(os.Getenv("CLOUDWATCH_LOGGING"), "true") {
		CloudWatchMode = true
	}
}

func levelColor(logLevel LogLevel) string {
	switch logLevel {
	case DEBUG:
		return colorYellow
	case INFO:
		return colorBlue
	case WARN:
		return colorPink
	case ERROR:
		return colorRed
	case VERBOSE:
		return colorGreen
	default:
		return ""
	}
}

func nowCompact() string {
	currentTime := time.Now()
	return fmt.Sprintf("%d%02d%02d:%02d:%02d:%02d%s",
		currentTime.Year(), currentTime.Month(), currentTime.Day(), currentTime.Hour(), currentTime.Minute(), currentTime.Second(), TZ)
}

func nextTaskID() string {
	goroutineMutex.Lock()
	sequenceIdentifier := goroutineSeq
	goroutineSeq++
	goroutineMutex.Unlock()
	return fmt.Sprintf("TASK-%03d", sequenceIdentifier)
}

func callerName(depth int) string {
	pc, _, _, ok := runtime.Caller(depth)
	if !ok {
		return "Unknown"
	}
	functionName := runtime.FuncForPC(pc).Name()
	if separatorIndex := strings.LastIndexByte(functionName, '.'); separatorIndex >= 0 {
		return functionName[separatorIndex+1:]
	}
	return functionName
}

func heapAllocMB() float64 {
	var memoryStats runtime.MemStats
	runtime.ReadMemStats(&memoryStats)
	return float64(memoryStats.Alloc) / 1024 / 1024
}

func fmtElapsed(duration time.Duration) string {
	switch {
	case duration == 0:
		return "0μs"
	case duration < time.Microsecond:
		return fmt.Sprintf("%.2fμs", float64(duration.Nanoseconds())/1000.0)
	case duration < time.Millisecond:
		return fmt.Sprintf("%.2fμs", float64(duration.Microseconds()))
	case duration < time.Second:
		return fmt.Sprintf("%.2fms", float64(duration.Milliseconds()))
	default:
		return fmt.Sprintf("%.2fs", duration.Seconds())
	}
}

func buildBlock(header, color string, rows [][]string) string {
	var sb strings.Builder
	keyW := 0
	for _, r := range rows {
		if len(r) == 2 && len(r[0]) > keyW {
			keyW = len(r[0])
		}
	}
	sb.WriteString(color)
	sb.WriteString(colorBold)
	sb.WriteString(header)
	sb.WriteString(colorReset)
	sb.WriteString("\n")
	for _, r := range rows {
		if len(r) == 2 {
			fmt.Fprintf(&sb, "%s  | %-*s : %s%s\n", color, keyW, r[0], r[1], colorReset)
		}
	}
	return sb.String()
}

func GetEnv(key, fallback string) string {
	if envValue := os.Getenv(key); envValue != "" {
		return envValue
	}
	return fallback
}

// ---------------------------------------------------------------------------
// CloudWatch JSON 结构化日志
// ---------------------------------------------------------------------------

// SetCloudWatchMode 启用或禁用 CloudWatch JSON 双输出模式。
// 启用后，所有日志函数在输出人类可读格式的同时，也会向 stderr 输出 NDJSON 行。
func SetCloudWatchMode(enabled bool) {
	CloudWatchMode = enabled
}

// NewTraceID 生成唯一的追踪标识符，格式为 trc-{unix_nano}-{random_hex_8}。
// 用于在分布式系统中关联同一请求的多条日志。
func NewTraceID() string {
	randomBytes := make([]byte, 4)
	_, _ = rand.Read(randomBytes)
	return fmt.Sprintf("trc-%d-%s", time.Now().UnixNano(), hex.EncodeToString(randomBytes))
}

// runtimeSnapshot 捕获当前运行时指标快照，包括协程数、堆内存、系统内存、
// GC 次数、CPU 核心数以及进程运行时长。
func runtimeSnapshot() map[string]interface{} {
	var memoryStats runtime.MemStats
	runtime.ReadMemStats(&memoryStats)
	return map[string]interface{}{
		"goroutine_count":    runtime.NumGoroutine(),
		"heap_alloc_bytes":   memoryStats.Alloc,
		"heap_alloc_mb":      float64(memoryStats.Alloc) / 1024 / 1024,
		"sys_memory_bytes":   memoryStats.Sys,
		"sys_memory_mb":      float64(memoryStats.Sys) / 1024 / 1024,
		"num_gc":             memoryStats.NumGC,
		"cpu_count":          runtime.NumCPU(),
		"uptime_ns":          time.Since(startTime).Nanoseconds(),
		"uptime_human":       fmtElapsed(time.Since(startTime)),
	}
}

// LogJSON 输出一行 CloudWatch 兼容的 JSON 结构化日志到 stderr（NDJSON 格式）。
// 每行包含纳秒级时间戳、运行时指标、调用者信息、追踪 ID 等完整上下文。
func LogJSON(component, operation string, level LogLevel, status, message string, elapsed time.Duration, details map[string]string) {
	now := time.Now()
	hostname, _ := os.Hostname()

	var memoryStats runtime.MemStats
	runtime.ReadMemStats(&memoryStats)

	entry := map[string]interface{}{
		"timestamp":          now.Format(time.RFC3339Nano),
		"timestamp_unix_nano": now.UnixNano(),
		"level":              level.String(),
		"component":          component,
		"operation":          operation,
		"status":             status,
		"message":            message,
		"elapsed_ns":         elapsed.Nanoseconds(),
		"elapsed_human":      fmtElapsed(elapsed),
		"memory_alloc_bytes": memoryStats.Alloc,
		"memory_alloc_mb":    float64(memoryStats.Alloc) / 1024 / 1024,
		"goroutine_count":    runtime.NumGoroutine(),
		"task_id":            nextTaskID(),
		"caller":             callerName(2),
		"trace_id":           NewTraceID(),
		"details":            details,
		"app":                APP_NAME,
		"version":            VERSION,
		"pid":                os.Getpid(),
		"hostname":           hostname,
	}

	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"error":"json_marshal_failed","raw":"%s"}`+"\n", message)
		return
	}
	fmt.Fprintln(os.Stderr, string(jsonBytes))
}

// emitCloudWatchJSON 是内部辅助函数，当 CloudWatchMode 为 true 时，
// 将结构化日志以 JSON 格式输出到 stderr。由现有日志函数调用。
func emitCloudWatchJSON(component, operation string, level LogLevel, status, message string, elapsed time.Duration, details []string) {
	detailMap := make(map[string]string, len(details))
	for _, detail := range details {
		key, value, ok := strings.Cut(detail, "=")
		if ok {
			detailMap[strings.TrimSpace(key)] = strings.TrimSpace(value)
		} else {
			detailMap[detail] = ""
		}
	}
	LogJSON(component, operation, level, status, message, elapsed, detailMap)
}

// LogVerbose 以 VERBOSE 级别输出最详细的日志，包含所有运行时指标。
// 适用于需要完整诊断信息的场景，如性能分析和审计追踪。
func LogVerbose(component, operation, msg string, details ...string) {
	snapshot := runtimeSnapshot()

	// 将运行时快照注入到详情中
	runtimeDetails := []string{
		fmt.Sprintf("goroutines=%d", snapshot["goroutine_count"]),
		fmt.Sprintf("heap_mb=%.2f", snapshot["heap_alloc_mb"]),
		fmt.Sprintf("sys_mb=%.2f", snapshot["sys_memory_mb"]),
		fmt.Sprintf("num_gc=%d", snapshot["num_gc"]),
		fmt.Sprintf("cpu_count=%d", snapshot["cpu_count"]),
		fmt.Sprintf("uptime_ns=%d", snapshot["uptime_ns"]),
		fmt.Sprintf("uptime=%s", snapshot["uptime_human"]),
		fmt.Sprintf("message=%s", msg),
	}
	allDetails := append(runtimeDetails, details...)
	Logf(component, operation, VERBOSE, "VERBOSE", 0, allDetails...)
}

// LogNano 与 Logf 类似，但在人类可读模式下也始终包含纳秒级精度的计时信息。
// 适用于需要极高精度时间测量的性能关键路径。
func LogNano(component, operation string, level LogLevel, status string, elapsed time.Duration, details ...string) {
	// 在详情中注入纳秒级计时
	nanoDetails := []string{
		fmt.Sprintf("elapsed_ns=%d", elapsed.Nanoseconds()),
		fmt.Sprintf("elapsed_nano=%dns", elapsed.Nanoseconds()),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	}
	allDetails := append(nanoDetails, details...)
	Logf(component, operation, level, status, elapsed, allDetails...)
}

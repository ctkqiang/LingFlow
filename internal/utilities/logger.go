package utilities

import (
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
	CurrentLevel  = INFO
	startTime     = time.Now()
	errorCallback func(string)
	goroutineSeq  int
	goroutineMutex sync.Mutex
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

func init() { SetLogLevel(os.Getenv("LOG_LEVEL")) }

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

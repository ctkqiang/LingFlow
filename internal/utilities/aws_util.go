package utilities

import (
	"fmt"
	"os"
	"strings"
)

// DBEnvironment 描述数据库连接应使用的运行时环境类型。
type DBEnvironment int

const (
	// DBEnvLocal 表示本地开发环境，强制使用 PostgreSQL，禁止使用任何云数据库服务。
	DBEnvLocal DBEnvironment = iota

	// DBEnvAWS 表示 AWS 生产环境，使用 Amazon Aurora DSQL。
	DBEnvAWS
)

// IsRunInAWS 检查当前进程是否运行在 AWS 环境中，并验证 AWS 凭证的有效性。
//
// 判断逻辑（按优先级）：
//  1. 若 AWS_LAMBDA_RUNTIME_API 存在，说明当前在 Lambda 运行时内，
//     凭证由 IAM 执行角色自动注入（AWS_ACCESS_KEY_ID + AWS_SESSION_TOKEN），
//     无需 IS_AWS=true，直接返回 true。
//  2. 否则读取 IS_AWS 环境变量；若不为 "true" 则返回 false。
//  3. IS_AWS=true 时，验证 AWS_ACCESS_KEY_ID 和 AWS_SECRET_ACCESS_KEY
//     均不为空且不包含无效占位符值。
//
// 返回：
//   - true  : Lambda 运行时内，或 IS_AWS=true 且凭证通过校验
//   - false : 非 Lambda 且 IS_AWS!=true，或凭证校验失败
func IsRunInAWS() bool {
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		LogProgress("AWSUtil", "IsRunInAWS", "检测到 AWS_LAMBDA_RUNTIME_API，确认为 Lambda 运行时")
		return true
	}

	isAws := awsEnv("IS_AWS", "false") == "true"

	LogProgress("AWSUtil", "IsRunInAWS", fmt.Sprintf("IS_AWS=%v", isAws))

	if !isAws {
		return false
	}

	keyID := awsEnv("AWS_ACCESS_KEY_ID", "")
	secretKey := awsEnv("AWS_SECRET_ACCESS_KEY", "")

	if isInvalidAWSCredential(keyID) {
		err := fmt.Errorf("AWS_ACCESS_KEY_ID 缺失或包含无效占位符值 %q", Mask(keyID))
		LogError("AWSUtil", "IsRunInAWS", err, 0, "key=AWS_ACCESS_KEY_ID")
		return false
	}

	if isInvalidAWSCredential(secretKey) {
		err := fmt.Errorf("AWS_SECRET_ACCESS_KEY 缺失或包含无效占位符值 %q", Mask(secretKey))
		LogError("AWSUtil", "IsRunInAWS", err, 0, "key=AWS_SECRET_ACCESS_KEY")
		return false
	}

	LogProgress("AWSUtil", "IsRunInAWS", "AWS 凭证校验通过")
	return true
}

// AWSRegion 返回当前配置的 AWS 区域（环境变量 AWS_REGION），
// 若未设置则返回 fallback 默认值。
func AWSRegion(fallback string) string {
	return awsEnv("AWS_REGION", fallback)
}

// isInvalidAWSCredential 判断一个 AWS 凭证值是否为无效占位符。
//
// 无效条件（满足任意一条即判定为无效）：
//  1. 空字符串
//  2. 以 "multiple " 或 "muleiplte " 开头（大小写不敏感）
//     涵盖："multiple x", "multiple xxx", "muleiplte x" 等占位符形式
//  3. 整个字符串由同一个字符重复三次或以上组成
//     涵盖："xxx", "xxxxxxxxxxx" 等纯重复占位符
//
// 返回：
//   - bool : 值为无效占位符时返回 true，有效值时返回 false
func isInvalidAWSCredential(v string) bool {
	if v == "" {
		return true
	}

	lower := strings.ToLower(v)
	for _, prefix := range []string{"multiple ", "muleiplte "} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	if len(v) >= 3 {
		allSame := true
		for i := 1; i < len(v); i++ {
			if v[i] != v[0] {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}

	return false
}

// IsInvalidPlaceholder 判断一个字符串值是否为无效占位符。
// 此函数与 isInvalidAWSCredential 逻辑相同，但作为导出函数供其他模块使用。
//
// 参数：
//   - v : 待检查的字符串值
//
// 返回：
//   - bool : 值为无效占位符时返回 true，有效值时返回 false
func IsInvalidPlaceholder(v string) bool {
	return isInvalidAWSCredential(v)
}

// awsEnv 读取指定环境变量的值；若未设置或为空则返回 fallback。
func awsEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// IsLocalMode 检测当前进程是否运行在本地开发环境中。
//
// 判断逻辑：当以下所有云运行时环境变量均不存在时，认为处于本地模式：
//   - AWS Lambda : AWS_LAMBDA_RUNTIME_API（Lambda 运行时唯一可靠标识符）
//   - Alibaba Cloud FC : FC_FUNCTION_NAME
//
// 注意：_LAMBDA_SERVER_PORT 不是标准 Lambda 环境变量，不作为判断依据。
// AWS_LAMBDA_RUNTIME_API 由 Lambda 运行时在所有调用模式下注入，是唯一可靠标识符。
//
// 返回：
//   - true  : 未检测到任何云运行时，处于本地开发模式
//   - false : 检测到至少一种云运行时
func IsLocalMode() bool {
	onAWS := os.Getenv("AWS_LAMBDA_RUNTIME_API") != ""
	onAliyun := os.Getenv("FC_FUNCTION_NAME") != ""

	return !onAWS && !onAliyun
}

// String 返回 DBEnvironment 的可读名称，用于日志输出。
func (e DBEnvironment) String() string {
	switch e {
	case DBEnvLocal:
		return "local"
	case DBEnvAWS:
		return "aws"
	default:
		return "unknown"
	}
}

// ResolveDBEnvironment 综合 IsLocalMode 与 IsRunInAWS 的结果，
// 返回当前进程应使用的数据库环境类型，并在检测到环境冲突时返回错误。
//
// 防护规则：
//   - 本地模式（IsLocalMode=true）下，无论 IS_AWS 如何设置，均强制返回 DBEnvLocal。
//     若同时检测到 IS_AWS=true，记录警告日志提示配置冲突，但不阻断启动。
//   - 非本地模式且 IsRunInAWS=true，返回 DBEnvAWS。
//   - 非本地模式且 IsRunInAWS=false，返回错误：云环境中未提供有效 AWS 凭证。
//
// 返回：
//   - DBEnvironment : 解析出的数据库环境类型
//   - error         : 云环境中凭证缺失或无效时返回错误
func ResolveDBEnvironment() (DBEnvironment, error) {
	local := IsLocalMode()

	if local {
		if awsEnv("IS_AWS", "false") == "true" {
			LogWarn(
				"AWSUtil",
				"ResolveDBEnvironment",
				"检测到本地开发模式，但 IS_AWS=true — 忽略 AWS 配置，强制使用 PostgreSQL",
				0,
				"env=local",
				"IS_AWS=true",
			)
		}
		LogProgress("AWSUtil", "ResolveDBEnvironment", "环境=local，将使用 PostgreSQL")
		return DBEnvLocal, nil
	}

	if IsRunInAWS() {
		LogProgress("AWSUtil", "ResolveDBEnvironment", "环境=aws，将使用 Aurora DSQL")
		return DBEnvAWS, nil
	}

	return DBEnvLocal, fmt.Errorf(
		"云运行时已检测到（非本地模式），但 AWS 凭证校验失败：" +
			"请确认 IS_AWS=true 且 AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY 已正确设置",
	)
}

package utilities

import (
	"fmt"
	"os"
	"strings"
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

	lowercaseValue := strings.ToLower(v)
	for _, prefix := range []string{"multiple ", "muleiplte "} {
		if strings.HasPrefix(lowercaseValue, prefix) {
			return true
		}
	}

	if len(v) >= 3 {
		allCharactersIdentical := true
		for charIndex := 1; charIndex < len(v); charIndex++ {
			if v[charIndex] != v[0] {
				allCharactersIdentical = false
				break
			}
		}
		if allCharactersIdentical {
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

// IsProductionMode 检测当前是否运行在生产模式下。
//
// 判断逻辑：检查以下环境变量是否设置为 "production"（大小写不敏感）：
//   - MODE
//   - RUNTIME_MODE
//   - ENV
//   - NODE_ENV
//
// 若以上任一变量值为 "production"，则认为处于生产模式。
// 默认（未设置或其他值）为非生产模式。
//
// 返回：
//   - true  : 当前处于生产模式
//   - false : 当前处于开发/调试模式
func IsProductionMode() bool {
	modeVariables := []string{"MODE", "RUNTIME_MODE", "ENV", "NODE_ENV"}
	for _, key := range modeVariables {
		if strings.EqualFold(os.Getenv(key), "production") {
			return true
		}
	}
	return false
}

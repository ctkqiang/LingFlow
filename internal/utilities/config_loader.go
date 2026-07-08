package utilities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/joho/godotenv"
)

const (
	// configLoaderComponent 是本文件在日志中使用的组件名称标识。
	configLoaderComponent = "ConfigLoader"

	// secretsManagerTimeout 是单次 Secrets Manager 或 S3 API 调用的超时时间。
	secretsManagerTimeout = 10 * time.Second
)

// requiredKeys 是应用正常运行所必须存在的环境变量列表。
// LoadConfig 在加载完成后会逐一校验这些键，任意一个缺失都会返回错误。
var requiredKeys = []string{
	"DB_HOST",
	"DB_PORT",
	"DB_USER",
	"DB_PASSWORD",
	"DB_NAME",
}

// LoadConfig 根据运行时环境选择配置加载策略。
//
// 非 AWS 环境：
//   - 从本地 .env 文件加载；文件不存在时回退到系统环境变量。
//
// AWS 环境（Lambda 运行时或 IS_AWS=true），按以下优先级依次尝试：
//  1. S3 .env 文件（S3_ENV_BUCKET + S3_ENV_KEY 均已配置时启用）
//  2. AWS Secrets Manager（SECRET_ARN 或 SECRET_NAME 已配置时启用）
//  3. 回退到 Lambda 系统环境变量（IAM 执行角色注入）
//
// 任意一级加载成功后即停止，不继续尝试下一级。
// 加载完成后统一校验必需键是否存在。
//
// 参数：
//   - ctx : 根上下文，用于 S3 / Secrets Manager API 调用超时控制
//
// 返回：
//   - error : 配置加载失败或必需键缺失时返回包装后的错误，成功时返回 nil
func LoadConfig(ctx context.Context) error {
	start := time.Now()
	LogStart(configLoaderComponent, "LoadConfig")

	if !IsRunInAWS() {
		if err := loadFromDotEnv(); err != nil {
			LogWarn(configLoaderComponent, "LoadConfig",
				fmt.Sprintf(".env 加载失败，回退到系统环境变量: %v", err),
				time.Since(start), "fallback=system-env",
			)
		}
	} else {
		if err := loadAWSConfig(ctx, start); err != nil {
			LogError(configLoaderComponent, "LoadConfig", err, time.Since(start), "step=loadAWSConfig")
			return fmt.Errorf("AWS 配置加载失败: %w", err)
		}
	}

	if err := validateRequiredKeys(); err != nil {
		LogError(configLoaderComponent, "LoadConfig", err, time.Since(start), "step=validate")
		return err
	}

	LogSuccess(configLoaderComponent, "LoadConfig", time.Since(start),
		fmt.Sprintf("aws=%v", IsRunInAWS()),
	)
	return nil
}

// loadAWSConfig 在 AWS 环境中按优先级依次尝试三种配置来源。
// 任意一种成功后立即返回，不继续尝试后续来源。
//
// 优先级：S3 .env > Secrets Manager > 系统环境变量（Lambda IAM 注入）
//
// 参数：
//   - ctx   : 根上下文
//   - start : 外层计时起点，用于日志输出耗时
//
// 返回：
//   - error : 所有配置来源均失败时返回错误，成功时返回 nil
func loadAWSConfig(ctx context.Context, start time.Time) error {
	// 第一优先级：S3 .env 文件。
	if getS3Bucket() != "" && getS3Key() != "" {
		if err := loadFromS3Env(ctx); err != nil {
			LogWarn(configLoaderComponent, "loadAWSConfig",
				fmt.Sprintf("S3 .env 加载失败，尝试 Secrets Manager: %v", err),
				time.Since(start), "source=s3",
			)
		} else {
			return nil
		}
	}

	// 第二优先级：Secrets Manager。
	if getSecretName() != "" {
		if err := loadFromSecretsManager(ctx); err != nil {
			LogWarn(configLoaderComponent, "loadAWSConfig",
				fmt.Sprintf("Secrets Manager 加载失败，回退到系统环境变量: %v", err),
				time.Since(start), "source=secrets-manager",
			)
		} else {
			return nil
		}
	}

	// 第三优先级：Lambda 系统环境变量（IAM 执行角色自动注入，无需任何操作）。
	// 在此阶段不返回错误，因为系统环境变量可能已包含必要的配置。
	// 最终校验由 validateRequiredKeys 在 LoadConfig 中统一执行。
	LogProgress(configLoaderComponent, "loadAWSConfig",
		"使用 Lambda 系统环境变量（IAM 执行角色注入）",
	)
	return nil
}

// loadFromDotEnv 从本地 .env 文件加载配置到进程环境变量。
// 若文件不存在，仅打印 WARN 日志，不返回错误（允许纯系统环境变量运行）。
//
// 返回：
//   - error : 文件存在但解析失败时返回错误；文件不存在时返回 nil
func loadFromDotEnv() error {
	start := time.Now()
	LogStart(configLoaderComponent, "loadFromDotEnv")

	if err := godotenv.Load(); err != nil {
		if os.IsNotExist(err) {
			LogWarn(configLoaderComponent, "loadFromDotEnv",
				"未找到 .env 文件，使用系统环境变量",
				time.Since(start), "file=.env",
			)
			return nil
		}
		return fmt.Errorf("解析 .env 文件失败: %w", err)
	}

	LogSuccess(configLoaderComponent, "loadFromDotEnv", time.Since(start), "file=.env")
	return nil
}

// loadFromS3Env 从 S3 下载 .env 文件并将其内容解析注入到进程环境变量。
//
// 所需环境变量：
//   - S3_ENV_BUCKET : 存储 .env 文件的 S3 存储桶名称
//   - S3_ENV_KEY    : .env 文件在存储桶中的对象键（如 "config/.env" 或 ".env"）
//
// 认证方式：使用 Lambda IAM 执行角色，无需静态凭证。
// 执行角色必须拥有 s3:GetObject 权限，策略示例：
//
//	{
//	  "Effect": "Allow",
//	  "Action": "s3:GetObject",
//	  "Resource": "arn:aws:s3:::<bucket>/<key>"
//	}
//
// 参数：
//   - ctx : 根上下文，调用超时由 secretsManagerTimeout 控制
//
// 返回：
//   - error : S3 调用失败或 .env 解析失败时返回包装后的错误
func loadFromS3Env(ctx context.Context) error {
	start := time.Now()
	LogStart(configLoaderComponent, "loadFromS3Env")

	bucket := getS3Bucket()
	key := getS3Key()
	region := AWSRegion("ap-east-1")

	LogProgress(configLoaderComponent, "loadFromS3Env",
		"正在从 S3 下载 .env 文件",
		fmt.Sprintf("bucket=%s", bucket),
		fmt.Sprintf("key=%s", key),
		fmt.Sprintf("region=%s", region),
	)

	callCtx, cancel := context.WithTimeout(ctx, secretsManagerTimeout)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(callCtx,
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("加载 AWS SDK 配置失败: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	output, err := client.GetObject(callCtx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("S3 GetObject 失败 (bucket=%s key=%s): %w", bucket, key, err)
	}
	defer output.Body.Close()

	body, err := io.ReadAll(output.Body)
	if err != nil {
		return fmt.Errorf("读取 S3 对象内容失败: %w", err)
	}

	// godotenv.Read 从 io.Reader 解析 .env 格式，返回 map[string]string。
	envMap, err := godotenv.Parse(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("解析 S3 .env 内容失败: %w", err)
	}

	injected := 0
	skipped := 0
	for k, v := range envMap {
		// 已存在的键不覆盖（Lambda IAM 注入的凭证优先）。
		if os.Getenv(k) != "" {
			skipped++
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("os.Setenv(%s) 失败: %w", k, err)
		}
		injected++
	}

	LogSuccess(configLoaderComponent, "loadFromS3Env", time.Since(start),
		fmt.Sprintf("bucket=%s", bucket),
		fmt.Sprintf("key=%s", key),
		fmt.Sprintf("injected=%d", injected),
		fmt.Sprintf("skipped=%d", skipped),
	)
	return nil
}

// loadFromSecretsManager 从 AWS Secrets Manager 读取密钥并注入到进程环境变量。
//
// 密钥标识符由 SECRET_ARN（优先）或 SECRET_NAME 环境变量指定。
// 密钥值必须是 JSON 对象，每个键值对将被写入 os.Setenv。
// 已存在于系统环境变量中的键不会被覆盖（Lambda IAM 注入的凭证优先）。
//
// 参数：
//   - ctx : 根上下文，调用超时由 secretsManagerTimeout 控制
//
// 返回：
//   - error : API 调用失败或 JSON 解析失败时返回包装后的错误
func loadFromSecretsManager(ctx context.Context) error {
	start := time.Now()
	LogStart(configLoaderComponent, "loadFromSecretsManager")

	secretName := getSecretName()
	if secretName == "" {
		return fmt.Errorf(
			"未配置 Secrets Manager 密钥标识符：" +
				"请设置 SECRET_ARN（完整 ARN）或 SECRET_NAME（密钥名称）",
		)
	}

	region := AWSRegion("ap-east-1")

	LogProgress(configLoaderComponent, "loadFromSecretsManager",
		"正在从 Secrets Manager 加载配置",
		fmt.Sprintf("secret=%s", Mask(secretName)),
		fmt.Sprintf("region=%s", region),
	)

	callCtx, cancel := context.WithTimeout(ctx, secretsManagerTimeout)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(callCtx,
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("加载 AWS SDK 配置失败: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	output, err := client.GetSecretValue(callCtx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	})
	if err != nil {
		return fmt.Errorf("Secrets Manager GetSecretValue 失败 (secret=%s): %w", Mask(secretName), err)
	}

	if output.SecretString == nil {
		return fmt.Errorf("Secrets Manager 返回空密钥值 (secret=%s)", Mask(secretName))
	}

	if err := injectSecretToEnv(*output.SecretString); err != nil {
		return fmt.Errorf("注入密钥到环境变量失败: %w", err)
	}

	LogSuccess(configLoaderComponent, "loadFromSecretsManager", time.Since(start),
		fmt.Sprintf("secret=%s", Mask(secretName)),
	)
	return nil
}

// injectSecretToEnv 将 JSON 格式的密钥字符串解析并注入到进程环境变量。
//
// 注入规则：
//   - 仅注入值为字符串类型的键（跳过嵌套对象和数组）
//   - 若系统环境变量中已存在同名键，跳过注入（不覆盖 Lambda IAM 注入的凭证）
//
// 参数：
//   - secretJSON : AWS Secrets Manager 返回的 SecretString，必须是 JSON 对象
//
// 返回：
//   - error : JSON 解析失败时返回错误
func injectSecretToEnv(secretJSON string) error {
	var secrets map[string]interface{}
	if err := json.Unmarshal([]byte(secretJSON), &secrets); err != nil {
		return fmt.Errorf("解析密钥 JSON 失败: %w", err)
	}

	injected := 0
	skipped := 0

	for key, val := range secrets {
		strVal, ok := val.(string)
		if !ok {
			skipped++
			continue
		}
		if os.Getenv(key) != "" {
			skipped++
			continue
		}
		if err := os.Setenv(key, strVal); err != nil {
			return fmt.Errorf("os.Setenv(%s) 失败: %w", key, err)
		}
		injected++
	}

	LogProgress(configLoaderComponent, "injectSecretToEnv",
		"注入完成",
		fmt.Sprintf("injected=%d", injected),
		fmt.Sprintf("skipped=%d", skipped),
	)
	return nil
}

// validateRequiredKeys 校验所有必需的环境变量键是否存在且非空。
// 在 LoadConfig 的最后阶段调用，无论配置来源如何都会执行。
//
// 校验规则：
//   - 环境变量必须存在且非空字符串
//   - 环境变量值不能是占位符（如 "xxxxxxxxxx"、"multiple x" 等）
//
// 返回：
//   - error : 任意必需键缺失或值为无效占位符时返回包含所有无效键名的错误
func validateRequiredKeys() error {
	var invalid []string
	for _, key := range requiredKeys {
		val := os.Getenv(key)
		if val == "" || IsInvalidPlaceholder(val) {
			invalid = append(invalid, key)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("缺少必需的环境变量或值为无效占位符: %v", invalid)
	}
	return nil
}

// getSecretName 返回 AWS Secrets Manager 中存储应用配置的密钥名称或完整 ARN。
//
// 读取优先级：SECRET_ARN > SECRET_NAME > 空字符串（触发错误）
//
// 返回：
//   - string : 密钥名称或完整 ARN，未配置时返回空字符串
func getSecretName() string {
	if arn := os.Getenv("SECRET_ARN"); arn != "" {
		return arn
	}
	return os.Getenv("SECRET_NAME")
}

// getS3Bucket 返回存储 .env 文件的 S3 存储桶名称（环境变量 S3_ENV_BUCKET）。
//
// 返回：
//   - string : 存储桶名称，未配置时返回空字符串
func getS3Bucket() string {
	return os.Getenv("S3_ENV_BUCKET")
}

// getS3Key 返回 .env 文件在 S3 存储桶中的对象键（环境变量 S3_ENV_KEY）。
// 默认值为 ".env"，适用于将 .env 直接存放在存储桶根目录的场景。
//
// 返回：
//   - string : 对象键，未配置时返回 ".env"
func getS3Key() string {
	if key := os.Getenv("S3_ENV_KEY"); key != "" {
		return key
	}
	return ".env"
}

package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const (
	defaultSkillsS3Prefix = "skills/"
	skillsFileExt         = ".md"
)

type S3SkillLoader struct {
	bucket string
	prefix string
	region string
	client *s3.Client
}

func NewS3SkillLoader() *S3SkillLoader {
	const component = "S3SkillLoader"
	const op = "NewS3SkillLoader"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	// ── 解析环境变量 SKILLS_S3_BUCKET / AWS_SKILLS_S3_BUCKET ──
	rawBucket := os.Getenv("SKILLS_S3_BUCKET")
	utilities.LogVerbose(component, op, "解析环境变量 SKILLS_S3_BUCKET",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("SKILLS_S3_BUCKET=%s", utilities.Mask(rawBucket)),
	)

	if rawBucket == "" {
		rawBucket = os.Getenv("AWS_SKILLS_S3_BUCKET")
		utilities.LogVerbose(component, op, "SKILLS_S3_BUCKET 为空，回退到 AWS_SKILLS_S3_BUCKET",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("AWS_SKILLS_S3_BUCKET=%s", utilities.Mask(rawBucket)),
		)
	}

	// ── ARN 提取 ──
	bucket := extractBucketFromARN(rawBucket)
	utilities.LogVerbose(component, op, "ARN 提取结果",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("raw_input=%s", utilities.Mask(rawBucket)),
		fmt.Sprintf("extracted_bucket=%s", utilities.Mask(bucket)),
		fmt.Sprintf("is_arn=%t", rawBucket != bucket),
	)

	// ── 解析前缀和区域 ──
	prefix := utilities.GetEnv("SKILLS_S3_PREFIX", defaultSkillsS3Prefix)
	utilities.LogVerbose(component, op, "解析环境变量 SKILLS_S3_PREFIX",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("SKILLS_S3_PREFIX=%s", prefix),
		fmt.Sprintf("default=%s", defaultSkillsS3Prefix),
	)

	rawS3Region := utilities.GetEnv("S3_REGION", "")
	rawAWSRegion := utilities.GetEnv("AWS_REGION", "ap-east-1")
	region := rawS3Region
	if region == "" {
		region = rawAWSRegion
	}
	utilities.LogVerbose(component, op, "解析区域环境变量",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("S3_REGION=%s", rawS3Region),
		fmt.Sprintf("AWS_REGION=%s", rawAWSRegion),
		fmt.Sprintf("resolved_region=%s", region),
	)

	if bucket == "" {
		utilities.LogWarn(component, op,
			"SKILLS_S3_BUCKET / AWS_SKILLS_S3_BUCKET 未设置，将使用本地模式（无技能）",
			time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return nil
	}

	// ── 最终配置汇总 ──
	utilities.LogVerbose(component, op, "S3 技能加载器最终配置",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", bucket),
		fmt.Sprintf("prefix=%s", prefix),
		fmt.Sprintf("region=%s", region),
	)

	utilities.LogSuccess(component, op, time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", bucket),
		fmt.Sprintf("prefix=%s", prefix),
		fmt.Sprintf("region=%s", region),
	)

	return &S3SkillLoader{
		bucket: bucket,
		prefix: prefix,
		region: region,
	}
}

// extractBucketFromARN 从 S3 ARN 中提取 bucket 名称。
// 支持格式: arn:aws:s3:::bucket-name
func extractBucketFromARN(input string) string {
	input = strings.TrimSpace(input)
	const arnPrefix = "arn:aws:s3:::"
	if strings.HasPrefix(input, arnPrefix) {
		return strings.TrimPrefix(input, arnPrefix)
	}
	return input
}

func (loader *S3SkillLoader) getClient(ctx context.Context) (*s3.Client, error) {
	const component = "S3SkillLoader"
	const op = "getClient"
	traceID := utilities.NewTraceID()

	if loader.client != nil {
		utilities.LogVerbose(component, op, "复用已有 S3 客户端实例",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("region=%s", loader.region),
		)
		return loader.client, nil
	}

	start := time.Now()
	utilities.LogStart(component, op)
	utilities.LogProgress(component, op, "正在加载 AWS SDK 默认配置",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("region=%s", loader.region),
	)

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(loader.region),
	)
	if err != nil {
		utilities.LogError(component, op, err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("region=%s", loader.region),
		)
		return nil, fmt.Errorf("加载 AWS SDK 配置失败: %w", err)
	}

	utilities.LogVerbose(component, op, "AWS SDK 配置加载成功，正在创建 S3 客户端",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("region=%s", loader.region),
	)

	loader.client = s3.NewFromConfig(cfg)

	utilities.LogSuccess(component, op, time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("region=%s", loader.region),
	)

	return loader.client, nil
}

func (loader *S3SkillLoader) ListSkills(ctx context.Context) ([]string, error) {
	const component = "S3SkillLoader"
	const op = "ListSkills"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	if loader == nil {
		utilities.LogWarn(component, op, "加载器为 nil，返回空列表", 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return []string{}, nil
	}

	utilities.LogVerbose(component, op, "开始列举 S3 技能对象",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("prefix=%s", loader.prefix),
	)

	client, err := loader.getClient(ctx)
	if err != nil {
		utilities.LogError(component, op, err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return nil, err
	}

	var skillNames []string
	var continuationToken *string
	pageNumber := 0

	for {
		pageNumber++
		pageStart := time.Now()

		utilities.LogProgress(component, op, "正在请求 S3 分页数据",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("page=%d", pageNumber),
			fmt.Sprintf("has_continuation_token=%t", continuationToken != nil),
		)

		output, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(loader.bucket),
			Prefix:            aws.String(loader.prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			utilities.LogError(component, op, fmt.Errorf("S3 ListObjectsV2 失败: %w", err), time.Since(start),
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("bucket=%s", loader.bucket),
				fmt.Sprintf("prefix=%s", loader.prefix),
				fmt.Sprintf("page=%d", pageNumber),
			)
			return nil, fmt.Errorf("S3 ListObjectsV2 失败 (bucket=%s prefix=%s): %w",
				loader.bucket, loader.prefix, err)
		}

		objectsInPage := len(output.Contents)
		utilities.LogVerbose(component, op, "分页数据返回",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("page=%d", pageNumber),
			fmt.Sprintf("objects_in_page=%d", objectsInPage),
			fmt.Sprintf("is_truncated=%t", aws.ToBool(output.IsTruncated)),
			fmt.Sprintf("page_elapsed_ns=%d", time.Since(pageStart).Nanoseconds()),
		)

		for _, obj := range output.Contents {
			key := *obj.Key
			if strings.HasSuffix(strings.ToLower(key), skillsFileExt) {
				skillName := extractSkillName(key, loader.prefix)
				if skillName != "" {
					skillNames = append(skillNames, skillName)
					utilities.LogVerbose(component, op, "提取到技能名称",
						fmt.Sprintf("trace_id=%s", traceID),
						fmt.Sprintf("key=%s", key),
						fmt.Sprintf("skill_name=%s", skillName),
					)
				}
			}
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}
		continuationToken = output.NextContinuationToken
	}

	elapsed := time.Since(start)
	utilities.LogNano(component, op, utilities.INFO, "OK", elapsed,
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("prefix=%s", loader.prefix),
		fmt.Sprintf("total_skills=%d", len(skillNames)),
		fmt.Sprintf("total_pages=%d", pageNumber),
		fmt.Sprintf("total_ns=%d", elapsed.Nanoseconds()),
	)

	return skillNames, nil
}

func (loader *S3SkillLoader) LoadSkill(ctx context.Context, skillIdentifier string) (models.SkillDefinition, error) {
	const component = "S3SkillLoader"
	const op = "LoadSkill"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	if loader == nil {
		utilities.LogError(component, op, fmt.Errorf("S3 技能加载器未初始化"), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return models.SkillDefinition{}, fmt.Errorf("S3 技能加载器未初始化")
	}

	client, err := loader.getClient(ctx)
	if err != nil {
		utilities.LogError(component, op, err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("skill=%s", skillIdentifier),
		)
		return models.SkillDefinition{}, err
	}

	// ── 构建 S3 Key ──
	skillKey := loader.buildSkillKey(skillIdentifier)
	utilities.LogVerbose(component, op, "S3 Key 构建完成",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill_identifier=%s", skillIdentifier),
		fmt.Sprintf("skill_key=%s", skillKey),
		fmt.Sprintf("bucket=%s", loader.bucket),
	)

	// ── GetObject 请求 ──
	utilities.LogProgress(component, op, "正在发送 S3 GetObject 请求",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("key=%s", skillKey),
	)

	getStart := time.Now()
	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(loader.bucket),
		Key:    aws.String(skillKey),
	})
	if err != nil {
		utilities.LogError(component, op, fmt.Errorf("S3 GetObject 失败: %w", err), time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("bucket=%s", loader.bucket),
			fmt.Sprintf("key=%s", skillKey),
		)
		return models.SkillDefinition{}, fmt.Errorf("S3 GetObject 失败 (bucket=%s key=%s): %w",
			loader.bucket, skillKey, err)
	}
	defer output.Body.Close()

	// ── 记录响应元数据 ──
	contentLength := int64(0)
	if output.ContentLength != nil {
		contentLength = *output.ContentLength
	}
	utilities.LogVerbose(component, op, "S3 GetObject 响应接收",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("content_length=%d", contentLength),
		fmt.Sprintf("get_elapsed_ns=%d", time.Since(getStart).Nanoseconds()),
	)

	content, err := io.ReadAll(output.Body)
	if err != nil {
		utilities.LogError(component, op, fmt.Errorf("读取技能文件内容失败: %w", err), time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("key=%s", skillKey),
		)
		return models.SkillDefinition{}, fmt.Errorf("读取技能文件内容失败: %w", err)
	}

	// ── 解析技能定义 ──
	utilities.LogProgress(component, op, "正在解析技能定义",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill=%s", skillIdentifier),
		fmt.Sprintf("content_size=%d", len(content)),
	)

	skillDef := parseSkillDefinition(skillIdentifier, content)

	utilities.LogVerbose(component, op, "技能定义解析完成",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("display_name=%s", skillDef.SkillDisplayName),
		fmt.Sprintf("category=%s", skillDef.SkillCategory),
		fmt.Sprintf("keywords_count=%d", len(skillDef.SearchKeywords)),
		fmt.Sprintf("description_len=%d", len(skillDef.SkillDescription)),
	)

	elapsed := time.Since(start)
	utilities.LogNano(component, op, utilities.INFO, "OK", elapsed,
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill=%s", skillIdentifier),
		fmt.Sprintf("size=%d", len(content)),
		fmt.Sprintf("total_ns=%d", elapsed.Nanoseconds()),
	)

	return skillDef, nil
}

func (loader *S3SkillLoader) LoadAllSkills(ctx context.Context) ([]models.SkillDefinition, error) {
	const component = "S3SkillLoader"
	const op = "LoadAllSkills"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	if loader == nil {
		utilities.LogWarn(component, op, "加载器为 nil，返回空列表", 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return []models.SkillDefinition{}, nil
	}

	skillNames, err := loader.ListSkills(ctx)
	if err != nil {
		utilities.LogError(component, op, err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return nil, err
	}

	utilities.LogProgress(component, op, "开始逐个加载技能定义",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("total_skills=%d", len(skillNames)),
	)

	var skills []models.SkillDefinition
	successCount := 0
	failCount := 0

	for idx, skillName := range skillNames {
		skillStart := time.Now()
		utilities.LogVerbose(component, op, "正在加载单个技能",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("index=%d/%d", idx+1, len(skillNames)),
			fmt.Sprintf("skill=%s", skillName),
		)

		skill, err := loader.LoadSkill(ctx, skillName)
		if err != nil {
			failCount++
			utilities.LogError(component, op, err, time.Since(skillStart),
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("skill=%s", skillName),
				fmt.Sprintf("index=%d/%d", idx+1, len(skillNames)),
				fmt.Sprintf("success_so_far=%d", successCount),
				fmt.Sprintf("fail_so_far=%d", failCount),
			)
			continue
		}
		successCount++
		skills = append(skills, skill)

		utilities.LogVerbose(component, op, "单个技能加载成功",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("skill=%s", skillName),
			fmt.Sprintf("index=%d/%d", idx+1, len(skillNames)),
			fmt.Sprintf("running_success=%d", successCount),
			fmt.Sprintf("running_fail=%d", failCount),
			fmt.Sprintf("skill_elapsed_ns=%d", time.Since(skillStart).Nanoseconds()),
		)
	}

	elapsed := time.Since(start)
	utilities.LogNano(component, op, utilities.INFO, "OK", elapsed,
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("loaded=%d", len(skills)),
		fmt.Sprintf("total=%d", len(skillNames)),
		fmt.Sprintf("failed=%d", failCount),
		fmt.Sprintf("total_ns=%d", elapsed.Nanoseconds()),
	)

	return skills, nil
}

// UploadSkill 将技能 Markdown 内容上传到 S3。
// 调用方必须先通过 SkillExists 检查重名，否则可能覆盖已有技能。
//
// 参数：
//   - ctx        : 上下文
//   - skillName  : 技能标识符（不含前缀和扩展名），例如 "trade_analyzer"
//   - content    : 完整的 Markdown 文件内容
//
// 返回：S3 PutObject 失败时返回包装后的错误，成功时返回 nil。
func (loader *S3SkillLoader) UploadSkill(ctx context.Context, skillName string, content []byte) error {
	const component = "S3SkillLoader"
	const op = "UploadSkill"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	if loader == nil {
		utilities.LogError(component, op, fmt.Errorf("S3 技能加载器未初始化"), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return fmt.Errorf("S3 技能加载器未初始化")
	}
	if loader.bucket == "" {
		utilities.LogError(component, op, fmt.Errorf("S3 bucket 未配置"), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return fmt.Errorf("S3 bucket 未配置，无法上传技能")
	}

	client, err := loader.getClient(ctx)
	if err != nil {
		utilities.LogError(component, op, err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return err
	}

	// ── 构建 S3 Key ──
	skillKey := loader.buildSkillKey(skillName)
	contentType := "text/markdown; charset=utf-8"

	utilities.LogVerbose(component, op, "准备上传技能文件",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("key=%s", skillKey),
		fmt.Sprintf("content_size=%d", len(content)),
		fmt.Sprintf("content_type=%s", contentType),
	)

	// ── PutObject 请求 ──
	putStart := time.Now()
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(loader.bucket),
		Key:         aws.String(skillKey),
		Body:        bytes.NewReader(content),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		utilities.LogError(component, op, fmt.Errorf("S3 PutObject 失败: %w", err), time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("bucket=%s", loader.bucket),
			fmt.Sprintf("key=%s", skillKey),
			fmt.Sprintf("region=%s", loader.region),
			fmt.Sprintf("content_size=%d", len(content)),
		)
		return fmt.Errorf("S3 PutObject 失败 (bucket=%s key=%s region=%s): %w",
			loader.bucket, skillKey, loader.region, err)
	}

	elapsed := time.Since(start)
	utilities.LogVerbose(component, op, "S3 PutObject 成功",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("put_elapsed_ns=%d", time.Since(putStart).Nanoseconds()),
	)

	utilities.LogNano(component, op, utilities.INFO, "OK", elapsed,
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("key=%s", skillKey),
		fmt.Sprintf("size=%d", len(content)),
		fmt.Sprintf("total_ns=%d", elapsed.Nanoseconds()),
	)

	return nil
}

// SkillExists 检查指定技能是否已存在于 S3。
// 通过 HeadObject 请求探测对象是否存在，避免下载完整内容。
//
// 返回值：
//   - bool : 技能存在则为 true
//   - error : S3 API 调用失败时返回错误（404 视为不存在，不算错误）
func (loader *S3SkillLoader) SkillExists(ctx context.Context, skillName string) (bool, error) {
	const component = "S3SkillLoader"
	const op = "SkillExists"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	if loader == nil || loader.bucket == "" {
		utilities.LogWarn(component, op, "加载器为 nil 或 bucket 为空，默认返回不存在", 0,
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("loader_nil=%t", loader == nil),
		)
		return false, nil
	}

	client, err := loader.getClient(ctx)
	if err != nil {
		utilities.LogError(component, op, err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return false, err
	}

	skillKey := loader.buildSkillKey(skillName)
	utilities.LogVerbose(component, op, "正在发送 S3 HeadObject 请求",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("key=%s", skillKey),
		fmt.Sprintf("skill=%s", skillName),
	)

	headStart := time.Now()
	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(loader.bucket),
		Key:    aws.String(skillKey),
	})
	headElapsed := time.Since(headStart)

	if err != nil {
		// S3 在对象不存在时返回 404 错误，归一化为 "NotFound"。
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey") {
			utilities.LogVerbose(component, op, "技能不存在（404/NoSuchKey）",
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("skill=%s", skillName),
				fmt.Sprintf("key=%s", skillKey),
				fmt.Sprintf("status=NOT_FOUND"),
				fmt.Sprintf("error_code=%s", apiErr.ErrorCode()),
				fmt.Sprintf("head_elapsed_ns=%d", headElapsed.Nanoseconds()),
			)
			utilities.LogNano(component, op, utilities.INFO, "OK", time.Since(start),
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("result=not_found"),
			)
			return false, nil
		}

		// ── 非 404 错误，归类为 S3 API 故障 ──
		utilities.LogError(component, op, fmt.Errorf("S3 HeadObject 异常: %w", err), time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("bucket=%s", loader.bucket),
			fmt.Sprintf("key=%s", skillKey),
			fmt.Sprintf("region=%s", loader.region),
			fmt.Sprintf("status=ERROR"),
			fmt.Sprintf("head_elapsed_ns=%d", headElapsed.Nanoseconds()),
		)
		return false, fmt.Errorf("S3 HeadObject 失败 (bucket=%s key=%s region=%s): %w",
			loader.bucket, skillKey, loader.region, err)
	}

	// ── 技能存在 ──
	utilities.LogVerbose(component, op, "技能已存在",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill=%s", skillName),
		fmt.Sprintf("key=%s", skillKey),
		fmt.Sprintf("status=EXISTS"),
		fmt.Sprintf("head_elapsed_ns=%d", headElapsed.Nanoseconds()),
	)
	utilities.LogNano(component, op, utilities.INFO, "OK", time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("result=exists"),
	)
	return true, nil
}

// StorageURI 返回技能文件在 S3 中的完整 URI，例如 "s3://bucket/skills/foo.md"。
// 用于在响应中告知用户新技能的存储位置。
func (loader *S3SkillLoader) StorageURI(skillName string) string {
	if loader == nil || loader.bucket == "" {
		return ""
	}
	return fmt.Sprintf("s3://%s/%s", loader.bucket, loader.buildSkillKey(skillName))
}

// DeleteSkill 从 S3 删除指定技能文件。
// 用于创建流程中清理预留的空占位文件。
func (loader *S3SkillLoader) DeleteSkill(ctx context.Context, skillName string) error {
	const component = "S3SkillLoader"
	const op = "DeleteSkill"
	traceID := utilities.NewTraceID()
	start := time.Now()
	utilities.LogStart(component, op)

	if loader == nil || loader.bucket == "" {
		utilities.LogError(component, op, fmt.Errorf("S3 技能加载器未初始化"), 0,
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return fmt.Errorf("S3 技能加载器未初始化")
	}

	client, err := loader.getClient(ctx)
	if err != nil {
		utilities.LogError(component, op, err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
		)
		return err
	}

	skillKey := loader.buildSkillKey(skillName)
	utilities.LogVerbose(component, op, "正在发送 S3 DeleteObject 请求",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("key=%s", skillKey),
		fmt.Sprintf("skill=%s", skillName),
	)

	deleteStart := time.Now()
	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(loader.bucket),
		Key:    aws.String(skillKey),
	})
	if err != nil {
		utilities.LogError(component, op, fmt.Errorf("S3 DeleteObject 失败: %w", err), time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("bucket=%s", loader.bucket),
			fmt.Sprintf("key=%s", skillKey),
		)
		return fmt.Errorf("S3 DeleteObject 失败 (bucket=%s key=%s): %w",
			loader.bucket, skillKey, err)
	}

	elapsed := time.Since(start)
	utilities.LogNano(component, op, utilities.INFO, "OK", elapsed,
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("key=%s", skillKey),
		fmt.Sprintf("delete_elapsed_ns=%d", time.Since(deleteStart).Nanoseconds()),
		fmt.Sprintf("total_ns=%d", elapsed.Nanoseconds()),
	)

	return nil
}

func (loader *S3SkillLoader) buildSkillKey(skillIdentifier string) string {
	const component = "S3SkillLoader"
	const op = "buildSkillKey"

	baseName := strings.TrimPrefix(skillIdentifier, "/")
	result := loader.prefix + baseName + skillsFileExt

	utilities.LogVerbose(component, op, "技能 Key 转换",
		fmt.Sprintf("input=%s", skillIdentifier),
		fmt.Sprintf("base_name=%s", baseName),
		fmt.Sprintf("prefix=%s", loader.prefix),
		fmt.Sprintf("output=%s", result),
	)

	return result
}

func extractSkillName(key string, prefix string) string {
	const component = "S3SkillLoader"
	const op = "extractSkillName"

	utilities.LogVerbose(component, op, "开始解析技能名称",
		fmt.Sprintf("key=%s", key),
		fmt.Sprintf("prefix=%s", prefix),
	)

	relativeKey := strings.TrimPrefix(key, prefix)
	if relativeKey == "" {
		utilities.LogVerbose(component, op, "相对路径为空，跳过",
			fmt.Sprintf("key=%s", key),
		)
		return ""
	}

	if strings.Contains(relativeKey, "/") {
		utilities.LogVerbose(component, op, "相对路径包含子目录，跳过",
			fmt.Sprintf("relative_key=%s", relativeKey),
		)
		return ""
	}

	baseName := strings.TrimSuffix(relativeKey, skillsFileExt)
	baseName = strings.TrimSuffix(baseName, strings.ToUpper(skillsFileExt))
	if baseName == "" {
		utilities.LogVerbose(component, op, "去除扩展名后为空，跳过",
			fmt.Sprintf("relative_key=%s", relativeKey),
		)
		return ""
	}

	result := "/" + baseName
	utilities.LogVerbose(component, op, "技能名称解析完成",
		fmt.Sprintf("key=%s", key),
		fmt.Sprintf("result=%s", result),
	)

	return result
}

func parseSkillDefinition(identifier string, content []byte) models.SkillDefinition {
	const component = "S3SkillLoader"
	const op = "parseSkillDefinition"
	traceID := utilities.NewTraceID()
	start := time.Now()

	utilities.LogVerbose(component, op, "开始解析技能定义元数据",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("identifier=%s", identifier),
		fmt.Sprintf("content_size=%d", len(content)),
	)

	displayName := strings.TrimPrefix(identifier, "/")
	lines := strings.SplitN(string(content), "\n", 20)

	description := ""
	category := "general"
	var keywords []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "## "), "# "))
			if title != "" {
				displayName = title
				utilities.LogVerbose(component, op, "提取到显示名称",
					fmt.Sprintf("trace_id=%s", traceID),
					fmt.Sprintf("display_name=%s", displayName),
				)
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "description:") {
			description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			utilities.LogVerbose(component, op, "提取到描述",
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("description_len=%d", len(description)),
			)
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "category:") {
			category = strings.TrimSpace(strings.TrimPrefix(line, "category:"))
			utilities.LogVerbose(component, op, "提取到分类",
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("category=%s", category),
			)
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "keywords:") {
			kwStr := strings.TrimSpace(strings.TrimPrefix(line, "keywords:"))
			for _, kw := range strings.Split(kwStr, ",") {
				kw = strings.TrimSpace(kw)
				if kw != "" {
					keywords = append(keywords, kw)
				}
			}
			utilities.LogVerbose(component, op, "提取到关键词",
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("keywords_count=%d", len(keywords)),
			)
			continue
		}
	}

	if description == "" && len(lines) > 2 {
		for _, line := range lines[2:] {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				description = line
				if len(description) > 100 {
					description = description[:100] + "..."
				}
				break
			}
		}
	}

	utilities.LogVerbose(component, op, "技能定义元数据解析完成",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("display_name=%s", displayName),
		fmt.Sprintf("description_len=%d", len(description)),
		fmt.Sprintf("category=%s", category),
		fmt.Sprintf("keywords_count=%d", len(keywords)),
		fmt.Sprintf("parse_elapsed_ns=%d", time.Since(start).Nanoseconds()),
	)

	return models.SkillDefinition{
		SkillIdentifier:  identifier,
		SkillDisplayName: displayName,
		SkillDescription: description,
		SearchKeywords:   keywords,
		SkillCategory:    category,
		MarkdownBody: models.SkillsMarkdownBody{
			Instructions: string(content),
		},
		SchemaVersion:        1,
		LastUpdatedTimestamp: time.Now(),
	}
}

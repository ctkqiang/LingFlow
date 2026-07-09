package services

import (
	"context"
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
	bucket := os.Getenv("SKILLS_S3_BUCKET")
	if bucket == "" {
		bucket = os.Getenv("AWS_SKILLS_S3_BUCKET")
	}
	// 如果是 ARN 格式 (arn:aws:s3:::bucket-name)，提取纯 bucket 名称
	bucket = extractBucketFromARN(bucket)

	prefix := utilities.GetEnv("SKILLS_S3_PREFIX", defaultSkillsS3Prefix)
	region := utilities.GetEnv("AWS_REGION", "ap-east-1")

	if bucket == "" {
		utilities.LogProgress("S3SkillLoader", "NewS3SkillLoader",
			"SKILLS_S3_BUCKET / AWS_SKILLS_S3_BUCKET 未设置，将使用本地模式（无技能）")
		return nil
	}

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
	if loader.client != nil {
		return loader.client, nil
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(loader.region),
	)
	if err != nil {
		return nil, fmt.Errorf("加载 AWS SDK 配置失败: %w", err)
	}

	loader.client = s3.NewFromConfig(cfg)
	return loader.client, nil
}

func (loader *S3SkillLoader) ListSkills(ctx context.Context) ([]string, error) {
	start := time.Now()
	utilities.LogStart("S3SkillLoader", "ListSkills")

	if loader == nil {
		return []string{}, nil
	}

	client, err := loader.getClient(ctx)
	if err != nil {
		return nil, err
	}

	var skillNames []string
	var continuationToken *string

	for {
		output, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(loader.bucket),
			Prefix:            aws.String(loader.prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("S3 ListObjectsV2 失败 (bucket=%s prefix=%s): %w",
				loader.bucket, loader.prefix, err)
		}

		for _, obj := range output.Contents {
			key := *obj.Key
			if strings.HasSuffix(strings.ToLower(key), skillsFileExt) {
				skillName := extractSkillName(key, loader.prefix)
				if skillName != "" {
					skillNames = append(skillNames, skillName)
				}
			}
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}
		continuationToken = output.NextContinuationToken
	}

	utilities.LogSuccess("S3SkillLoader", "ListSkills", time.Since(start),
		fmt.Sprintf("bucket=%s", loader.bucket),
		fmt.Sprintf("prefix=%s", loader.prefix),
		fmt.Sprintf("count=%d", len(skillNames)),
	)

	return skillNames, nil
}

func (loader *S3SkillLoader) LoadSkill(ctx context.Context, skillIdentifier string) (models.SkillDefinition, error) {
	start := time.Now()
	utilities.LogStart("S3SkillLoader", "LoadSkill")

	if loader == nil {
		return models.SkillDefinition{}, fmt.Errorf("S3 技能加载器未初始化")
	}

	client, err := loader.getClient(ctx)
	if err != nil {
		return models.SkillDefinition{}, err
	}

	skillKey := loader.buildSkillKey(skillIdentifier)

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(loader.bucket),
		Key:    aws.String(skillKey),
	})
	if err != nil {
		return models.SkillDefinition{}, fmt.Errorf("S3 GetObject 失败 (bucket=%s key=%s): %w",
			loader.bucket, skillKey, err)
	}
	defer output.Body.Close()

	content, err := io.ReadAll(output.Body)
	if err != nil {
		return models.SkillDefinition{}, fmt.Errorf("读取技能文件内容失败: %w", err)
	}

	skillDef := parseSkillDefinition(skillIdentifier, content)

	utilities.LogSuccess("S3SkillLoader", "LoadSkill", time.Since(start),
		fmt.Sprintf("skill=%s", skillIdentifier),
		fmt.Sprintf("size=%d", len(content)),
	)

	return skillDef, nil
}

func (loader *S3SkillLoader) LoadAllSkills(ctx context.Context) ([]models.SkillDefinition, error) {
	start := time.Now()
	utilities.LogStart("S3SkillLoader", "LoadAllSkills")

	if loader == nil {
		return []models.SkillDefinition{}, nil
	}

	skillNames, err := loader.ListSkills(ctx)
	if err != nil {
		return nil, err
	}

	var skills []models.SkillDefinition
	for _, skillName := range skillNames {
		skill, err := loader.LoadSkill(ctx, skillName)
		if err != nil {
			utilities.LogError("S3SkillLoader", "LoadAllSkills", err, 0,
				fmt.Sprintf("skill=%s", skillName))
			continue
		}
		skills = append(skills, skill)
	}

	utilities.LogSuccess("S3SkillLoader", "LoadAllSkills", time.Since(start),
		fmt.Sprintf("loaded=%d", len(skills)),
		fmt.Sprintf("total=%d", len(skillNames)),
	)

	return skills, nil
}

func (loader *S3SkillLoader) buildSkillKey(skillIdentifier string) string {
	baseName := strings.TrimPrefix(skillIdentifier, "/")
	return loader.prefix + baseName + skillsFileExt
}

func extractSkillName(key string, prefix string) string {
	relativeKey := strings.TrimPrefix(key, prefix)
	if relativeKey == "" {
		return ""
	}

	if strings.Contains(relativeKey, "/") {
		return ""
	}

	baseName := strings.TrimSuffix(relativeKey, skillsFileExt)
	baseName = strings.TrimSuffix(baseName, strings.ToUpper(skillsFileExt))
	if baseName == "" {
		return ""
	}

	return "/" + baseName
}

func parseSkillDefinition(identifier string, content []byte) models.SkillDefinition {
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
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "description:") {
			description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "category:") {
			category = strings.TrimSpace(strings.TrimPrefix(line, "category:"))
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

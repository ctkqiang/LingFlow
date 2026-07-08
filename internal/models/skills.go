package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	SkillObjectKeyPrefix = "skills/"
	SkillFileExtension   = ".md"
)

var (
	ErrSkillBucketNameEmpty         = errors.New("技能: bucket 名称为空")
	ErrSkillObjectKeyEmpty          = errors.New("技能: 对象键为空")
	ErrSkillObjectKeyWrongPrefix    = fmt.Errorf("技能: 键必须位于 %q 下", SkillObjectKeyPrefix)
	ErrSkillObjectKeyWrongExtension = fmt.Errorf("技能: 键必须以 %q 结尾", SkillFileExtension)
	ErrSkillFileNameMissing         = errors.New("技能: 键没有文件名")
	ErrSkillNestedPathNotAllowed    = errors.New("技能: 不允许在 skills/ 下使用嵌套文件夹")
	ErrSkillPathTraversalDetected   = errors.New("技能: 键包含非法的路径遍历")
)

// SkillReferenceFile 表示技能可以按需拉取的额外参考文件
type SkillReferenceFile struct {
	FilePath string // 参考文件在存储桶中的完整路径
	FileName string // 参考文件的显示名称
}

// SkillStoragePath 表示一个经过校验的 S3 技能文件路径，
// 包含存储桶名称和技能文件名（不含前缀与扩展名）。
type SkillStoragePath struct {
	BucketName    string // S3 存储桶名称
	SkillFileName string // 技能文件名（不含前缀和扩展名），例如 "refund-status"
}

// ObjectKey 返回规范的 S3 对象键，例如 "skills/refund-status.md"
func (skillPath SkillStoragePath) ObjectKey() string {
	return SkillObjectKeyPrefix + skillPath.SkillFileName + SkillFileExtension
}

// StorageURI 返回完整的 S3 URI，例如 "s3://my-bucket/skills/refund-status.md"
func (skillPath SkillStoragePath) StorageURI() string {
	return fmt.Sprintf("s3://%s/%s", skillPath.BucketName, skillPath.ObjectKey())
}

type SkillsMarkdownBody struct {
	Instructions string
	Rules        []string
	References   []SkillReferenceFile
}

// SkillDefinition 表示一个完整的技能定义，包含内容体和所有元数据
type SkillDefinition struct {
	SkillIdentifier      string               // 稳定唯一标识符，同时也是 S3 前缀，例如 "billing/refund-status"
	SkillDisplayName     string               // 简短的人类可读名称
	SkillDescription     string               // 触发文本 -- 用于嵌入和语义搜索的内容
	SearchKeywords       []string             // 可选，用于混合检索（关键词 + 向量）
	SkillCategory        string               // 用于两阶段检索和分组
	MarkdownBody         SkillsMarkdownBody   // 完整的 SKILL.md 内容，注入到提示词中
	ReferenceFiles       []SkillReferenceFile // 技能可以按需拉取的额外参考文件
	SchemaVersion        int                  // 编辑时递增；用于使索引缓存失效
	LastUpdatedTimestamp time.Time            // 最后更新时间
}

// SkillMetadata 表示技能的轻量级元数据，用于列表展示和检索索引
type SkillMetadata struct {
	SkillIdentifier  string   // 稳定唯一标识符
	SkillDisplayName string   // 简短的人类可读名称
	SkillDescription string   // 触发文本 -- 用于嵌入和语义搜索的内容
	SearchKeywords   []string // 可选，用于混合检索（关键词 + 向量）
	SkillCategory    string   // 用于两阶段检索和分组
	SchemaVersion    int      // 编辑时递增；用于使索引缓存失效
}

// NewSkillStoragePath 根据存储桶名称和技能名称构建一个经过校验的存储路径，
// 适用于写入场景（创建/更新技能文件时使用）。
func NewSkillStoragePath(bucketName, skillName string) (SkillStoragePath, error) {
	if bucketName == "" {
		return SkillStoragePath{}, ErrSkillBucketNameEmpty
	}

	// 去除首尾空白和可能误带的文件扩展名
	cleanedSkillName := strings.TrimSpace(
		strings.TrimSuffix(skillName, SkillFileExtension),
	)

	if cleanedSkillName == "" {
		return SkillStoragePath{}, ErrSkillFileNameMissing
	}

	// 技能文件必须是扁平结构，不允许包含路径分隔符
	if strings.ContainsAny(cleanedSkillName, `/\`) {
		return SkillStoragePath{}, ErrSkillNestedPathNotAllowed
	}

	return SkillStoragePath{
		BucketName:    bucketName,
		SkillFileName: cleanedSkillName,
	}, nil
}

// ParseSkillObjectKey 校验一个已存在的 S3 对象键，并解析为 SkillStoragePath。
// 适用于读取场景（处理 S3 事件或上传通知时使用），
// 会拒绝任何不符合规范的键（路径遍历、嵌套目录、错误前缀/扩展名等）。
func ParseSkillObjectKey(bucketName, objectKey string) (SkillStoragePath, error) {
	switch {
	case bucketName == "":
		return SkillStoragePath{}, ErrSkillBucketNameEmpty
	case objectKey == "":
		return SkillStoragePath{}, ErrSkillObjectKeyEmpty
	case strings.Contains(objectKey, ".."):
		return SkillStoragePath{}, fmt.Errorf("%w: %q", ErrSkillPathTraversalDetected, objectKey)
	case strings.Contains(objectKey, `\`):
		return SkillStoragePath{}, fmt.Errorf("%w: %q", ErrSkillNestedPathNotAllowed, objectKey)
	case !strings.HasPrefix(objectKey, SkillObjectKeyPrefix):
		return SkillStoragePath{}, fmt.Errorf("%w: got %q", ErrSkillObjectKeyWrongPrefix, objectKey)
	case !strings.HasSuffix(objectKey, SkillFileExtension):
		return SkillStoragePath{}, fmt.Errorf("%w: got %q", ErrSkillObjectKeyWrongExtension, objectKey)
	}

	// 从对象键中提取纯文件名（去除前缀和扩展名）
	extractedSkillName := strings.TrimSuffix(
		strings.TrimPrefix(objectKey, SkillObjectKeyPrefix),
		SkillFileExtension,
	)

	if extractedSkillName == "" {
		return SkillStoragePath{}, ErrSkillFileNameMissing
	}

	// 确保提取后的名称不包含子目录
	if strings.Contains(extractedSkillName, "/") {
		return SkillStoragePath{}, fmt.Errorf("%w: %q", ErrSkillNestedPathNotAllowed, objectKey)
	}

	return SkillStoragePath{
		BucketName:    bucketName,
		SkillFileName: extractedSkillName,
	}, nil
}

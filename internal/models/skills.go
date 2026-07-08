package models

import "time"

type S3ReferenceFile struct {
	Path    string
	Content string
}

type Skills struct {
	ID          string            // 稳定唯一标识符，同时也是 S3 前缀，例如 "billing/refund-status"
	Name        string            // 简短的人类可读名称
	Description string            // 触发文本 —— 用于嵌入和搜索的内容
	Keywords    []string          // 可选，用于混合（关键词 + 向量）搜索
	Category    string            // 用于两阶段检索 / 分组
	Body        string            // 完整的 SKILL.md 内容，注入到提示词中
	References  []S3ReferenceFile // 技能可以按需拉取的额外文件
	Version     int               // 编辑时递增；用于使索引缓存失效
	UpdatedAt   time.Time         // 最后更新时间
}

type SkillMetadata struct {
	ID          string
	Name        string
	Description string
	Keywords    []string
	Category    string
	Version     int
}

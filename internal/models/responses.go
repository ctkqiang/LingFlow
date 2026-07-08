package models

type RetrievalResult struct {
	Meta  SkillMetadata
	Score float32 // 相似度/相关性分数，用于排序和阈值判断
}

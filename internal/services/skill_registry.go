package services

import (
	"fmt"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	defaultRetrievalScoreThreshold = 0.3
	defaultMaxRetrievalResults     = 5
)

// SkillRegistry 管理所有可用技能的内存索引，并提供技能选择的检索能力。
type SkillRegistry struct {
	registryMutex   sync.RWMutex
	skills          map[string]models.SkillDefinition
	metadataIndex   []models.SkillMetadata
	scoreThreshold  float32
	maxResults      int
}

// NewSkillRegistry 使用默认配置创建一个新的技能注册中心。
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills:         make(map[string]models.SkillDefinition),
		metadataIndex:  make([]models.SkillMetadata, 0),
		scoreThreshold: defaultRetrievalScoreThreshold,
		maxResults:     defaultMaxRetrievalResults,
	}
}

// RegisterSkill 在注册中心中添加或更新一个技能。
func (registry *SkillRegistry) RegisterSkill(skill models.SkillDefinition) error {
	start := time.Now()
	utilities.LogStart("SkillRegistry", "RegisterSkill")

	if skill.SkillIdentifier == "" {
		return fmt.Errorf("技能标识符为必填项")
	}

	if skill.SkillDisplayName == "" {
		return fmt.Errorf("技能 %q 的显示名称为必填项", skill.SkillIdentifier)
	}

	registry.registryMutex.Lock()
	defer registry.registryMutex.Unlock()

	registry.skills[skill.SkillIdentifier] = skill
	registry.rebuildMetadataIndexLocked()

	utilities.LogSuccess("SkillRegistry", "RegisterSkill", time.Since(start),
		fmt.Sprintf("skill=%s", skill.SkillIdentifier),
		fmt.Sprintf("total=%d", len(registry.skills)),
	)
	return nil
}

// UnregisterSkill 从注册中心移除一个技能。
func (registry *SkillRegistry) UnregisterSkill(skillIdentifier string) bool {
	registry.registryMutex.Lock()
	defer registry.registryMutex.Unlock()

	if _, exists := registry.skills[skillIdentifier]; !exists {
		return false
	}

	delete(registry.skills, skillIdentifier)
	registry.rebuildMetadataIndexLocked()
	return true
}

// GetSkill 根据标识符检索一个技能。
func (registry *SkillRegistry) GetSkill(skillIdentifier string) (models.SkillDefinition, bool) {
	registry.registryMutex.RLock()
	defer registry.registryMutex.RUnlock()

	skill, exists := registry.skills[skillIdentifier]
	return skill, exists
}

// ListSkills 返回所有已注册技能的元数据。
func (registry *SkillRegistry) ListSkills() []models.SkillMetadata {
	registry.registryMutex.RLock()
	defer registry.registryMutex.RUnlock()

	result := make([]models.SkillMetadata, len(registry.metadataIndex))
	copy(result, registry.metadataIndex)
	return result
}

// ListSkillIDs 返回所有已注册技能的标识符列表。
func (registry *SkillRegistry) ListSkillIDs() []string {
	registry.registryMutex.RLock()
	defer registry.registryMutex.RUnlock()

	result := make([]string, 0, len(registry.metadataIndex))
	for _, meta := range registry.metadataIndex {
		result = append(result, meta.SkillIdentifier)
	}
	return result
}

// RetrieveSkills 执行基于关键词的检索，查找与用户查询相关的技能。
// 返回按相关性评分排序的结果（最高分在前）。
func (registry *SkillRegistry) RetrieveSkills(userQuery string) []models.RetrievalResult {
	start := time.Now()
	utilities.LogStart("SkillRegistry", "RetrieveSkills")

	registry.registryMutex.RLock()
	defer registry.registryMutex.RUnlock()

	queryTokens := tokenize(userQuery)
	if len(queryTokens) == 0 {
		return nil
	}

	var results []models.RetrievalResult

	for _, meta := range registry.metadataIndex {
		score := computeRelevanceScore(queryTokens, meta)
		if score >= registry.scoreThreshold {
			results = append(results, models.RetrievalResult{
				Meta:  meta,
				Score: score,
			})
		}
	}

	sortRetrievalResults(results)

	if len(results) > registry.maxResults {
		results = results[:registry.maxResults]
	}

	utilities.LogSuccess("SkillRegistry", "RetrieveSkills", time.Since(start),
		fmt.Sprintf("query=%q", truncate(userQuery, 50)),
		fmt.Sprintf("candidates=%d", len(registry.metadataIndex)),
		fmt.Sprintf("matched=%d", len(results)),
	)

	return results
}

// RetrieveBestSkill 返回与查询最相关的单个技能，
// 若没有技能达到评分阈值则返回 nil。
func (registry *SkillRegistry) RetrieveBestSkill(userQuery string) *models.SkillDefinition {
	results := registry.RetrieveSkills(userQuery)
	if len(results) == 0 {
		return nil
	}

	skill, exists := registry.GetSkill(results[0].Meta.SkillIdentifier)
	if !exists {
		return nil
	}

	return &skill
}

// SkillCount 返回已注册技能的数量。
func (registry *SkillRegistry) SkillCount() int {
	registry.registryMutex.RLock()
	defer registry.registryMutex.RUnlock()
	return len(registry.skills)
}

// rebuildMetadataIndexLocked 从当前技能重建元数据索引。
// 调用方必须持有写锁。
func (registry *SkillRegistry) rebuildMetadataIndexLocked() {
	registry.metadataIndex = make([]models.SkillMetadata, 0, len(registry.skills))
	for _, skill := range registry.skills {
		registry.metadataIndex = append(registry.metadataIndex, models.SkillMetadata{
			SkillIdentifier:  skill.SkillIdentifier,
			SkillDisplayName: skill.SkillDisplayName,
			SkillDescription: skill.SkillDescription,
			SearchKeywords:   skill.SearchKeywords,
			SkillCategory:    skill.SkillCategory,
			SchemaVersion:    skill.SchemaVersion,
		})
	}
}

// computeRelevanceScore 计算查询词元与技能可搜索文本字段之间基于 TF 的相关性评分。
func computeRelevanceScore(queryTokens []string, meta models.SkillMetadata) float32 {
	searchableText := strings.ToLower(strings.Join([]string{
		meta.SkillDisplayName,
		meta.SkillDescription,
		meta.SkillCategory,
		strings.Join(meta.SearchKeywords, " "),
	}, " "))

	searchTokens := tokenize(searchableText)
	if len(searchTokens) == 0 {
		return 0
	}

	tokenFrequency := make(map[string]int, len(searchTokens))
	for _, token := range searchTokens {
		tokenFrequency[token]++
	}

	matchedTokens := 0
	totalWeight := float64(0)

	for _, queryToken := range queryTokens {
		if frequency, found := tokenFrequency[queryToken]; found {
			matchedTokens++
			totalWeight += 1.0 + math.Log(float64(frequency))
		} else {
			for searchToken, frequency := range tokenFrequency {
				if strings.Contains(searchToken, queryToken) || strings.Contains(queryToken, searchToken) {
					matchedTokens++
					totalWeight += 0.5 * (1.0 + math.Log(float64(frequency)))
					break
				}
			}
		}
	}

	if matchedTokens == 0 {
		return 0
	}

	coverage := float64(matchedTokens) / float64(len(queryTokens))
	normalizedWeight := totalWeight / float64(len(queryTokens))

	return float32(coverage*0.6 + normalizedWeight*0.4)
}

// tokenize 将文本拆分为小写词元，过滤掉短词元和标点符号。
func tokenize(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	tokens := make([]string, 0, len(words))
	for _, word := range words {
		if len(word) >= 2 {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

// sortRetrievalResults 使用插入排序按评分降序排列结果
// （对技能检索中常见的小切片非常高效）。
func sortRetrievalResults(results []models.RetrievalResult) {
	for outerIndex := 1; outerIndex < len(results); outerIndex++ {
		currentElement := results[outerIndex]
		innerIndex := outerIndex - 1
		for innerIndex >= 0 && results[innerIndex].Score < currentElement.Score {
			results[innerIndex+1] = results[innerIndex]
			innerIndex--
		}
		results[innerIndex+1] = currentElement
	}
}

// truncate 将字符串截断为 maxLen 个字符，截断时追加 "..."。
func truncate(inputString string, maxLen int) string {
	runes := []rune(inputString)
	if len(runes) <= maxLen {
		return inputString
	}
	return string(runes[:maxLen]) + "..."
}

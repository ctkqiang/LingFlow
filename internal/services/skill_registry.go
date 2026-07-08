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
	mu              sync.RWMutex
	skills          map[string]models.SkillDefinition
	metadataIndex   []models.SkillMetadata
	scoreThreshold  float32
	maxResults      int
}

// NewSkillRegistry creates a new skill registry with default configuration.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills:         make(map[string]models.SkillDefinition),
		metadataIndex:  make([]models.SkillMetadata, 0),
		scoreThreshold: defaultRetrievalScoreThreshold,
		maxResults:     defaultMaxRetrievalResults,
	}
}

// RegisterSkill adds or updates a skill in the registry.
func (registry *SkillRegistry) RegisterSkill(skill models.SkillDefinition) error {
	start := time.Now()
	utilities.LogStart("SkillRegistry", "RegisterSkill")

	if skill.SkillIdentifier == "" {
		return fmt.Errorf("skill identifier is required")
	}

	if skill.SkillDisplayName == "" {
		return fmt.Errorf("skill display name is required for skill %q", skill.SkillIdentifier)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.skills[skill.SkillIdentifier] = skill
	registry.rebuildMetadataIndexLocked()

	utilities.LogSuccess("SkillRegistry", "RegisterSkill", time.Since(start),
		fmt.Sprintf("skill=%s", skill.SkillIdentifier),
		fmt.Sprintf("total=%d", len(registry.skills)),
	)
	return nil
}

// UnregisterSkill removes a skill from the registry.
func (registry *SkillRegistry) UnregisterSkill(skillIdentifier string) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.skills[skillIdentifier]; !exists {
		return false
	}

	delete(registry.skills, skillIdentifier)
	registry.rebuildMetadataIndexLocked()
	return true
}

// GetSkill retrieves a skill by its identifier.
func (registry *SkillRegistry) GetSkill(skillIdentifier string) (models.SkillDefinition, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	skill, exists := registry.skills[skillIdentifier]
	return skill, exists
}

// ListSkills returns metadata for all registered skills.
func (registry *SkillRegistry) ListSkills() []models.SkillMetadata {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	result := make([]models.SkillMetadata, len(registry.metadataIndex))
	copy(result, registry.metadataIndex)
	return result
}

// RetrieveSkills performs keyword-based retrieval to find skills
// relevant to the given user query. Returns scored results sorted
// by relevance (highest first).
func (registry *SkillRegistry) RetrieveSkills(userQuery string) []models.RetrievalResult {
	start := time.Now()
	utilities.LogStart("SkillRegistry", "RetrieveSkills")

	registry.mu.RLock()
	defer registry.mu.RUnlock()

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

// RetrieveBestSkill returns the single most relevant skill for the query,
// or nil if no skill meets the score threshold.
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

// SkillCount returns the number of registered skills.
func (registry *SkillRegistry) SkillCount() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return len(registry.skills)
}

// rebuildMetadataIndexLocked rebuilds the metadata index from current skills.
// Caller must hold the write lock.
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

// computeRelevanceScore calculates a TF-based relevance score between
// query tokens and a skill's searchable text fields.
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
		if freq, found := tokenFrequency[queryToken]; found {
			matchedTokens++
			totalWeight += 1.0 + math.Log(float64(freq))
		} else {
			for searchToken, freq := range tokenFrequency {
				if strings.Contains(searchToken, queryToken) || strings.Contains(queryToken, searchToken) {
					matchedTokens++
					totalWeight += 0.5 * (1.0 + math.Log(float64(freq)))
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

// tokenize splits text into lowercase tokens, filtering out short tokens and punctuation.
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

// sortRetrievalResults sorts results by score descending using insertion sort
// (efficient for small slices typical in skill retrieval).
func sortRetrievalResults(results []models.RetrievalResult) {
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Score < key.Score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"time"
)

const defaultSystemPrompt = "You are LingFlow, an intelligent assistant. " +
	"Answer the user's question accurately and helpfully. " +
	"If a skill context is provided, use it to guide your response."

// SkillExecutionError 表示技能执行过程中的结构化错误。
type SkillExecutionError struct {
	SkillID   string
	Phase     string
	Cause     error
	Timestamp time.Time
}

func (e *SkillExecutionError) Error() string {
	return fmt.Sprintf("skill execution failed [skill=%s phase=%s]: %v", e.SkillID, e.Phase, e.Cause)
}

func (e *SkillExecutionError) Unwrap() error {
	return e.Cause
}

// NewSkillExecutionError creates a new SkillExecutionError.
func NewSkillExecutionError(skillID, phase string, cause error) *SkillExecutionError {
	return &SkillExecutionError{
		SkillID:   skillID,
		Phase:     phase,
		Cause:     cause,
		Timestamp: time.Now(),
	}
}

// SkillExecutor orchestrates the full pipeline: skill retrieval,
// context injection, LLM generation, and response formatting.
type SkillExecutor struct {
	registry     *SkillRegistry
	llmService   LLMService
	systemPrompt string
}

// NewSkillExecutor creates a new skill executor with the given dependencies.
func NewSkillExecutor(registry *SkillRegistry, llmService LLMService) *SkillExecutor {
	return &SkillExecutor{
		registry:     registry,
		llmService:   llmService,
		systemPrompt: defaultSystemPrompt,
	}
}

// SetSystemPrompt overrides the default system prompt.
func (executor *SkillExecutor) SetSystemPrompt(prompt string) {
	executor.systemPrompt = prompt
}

// ExecutionResult holds the complete result of a skill-augmented LLM execution.
type ExecutionResult struct {
	Response    LLMResponse
	SkillUsed   *models.SkillMetadata
	WSMessage   models.WSMessage
}

// Execute processes a user message through the full pipeline:
// 1. Retrieve the best matching skill for the user's query
// 2. Build skill-augmented context for the LLM
// 3. Generate a response via the LLM service
// 4. Format the response as a WSMessage
func (executor *SkillExecutor) Execute(ctx context.Context, userMessage models.UserChatData) (ExecutionResult, error) {
	start := time.Now()
	utilities.LogStart("SkillExecutor", "Execute")

	// Phase 1: Skill Retrieval
	skill, skillMeta, err := executor.retrieveSkill(userMessage.Message)
	if err != nil {
		utilities.LogError("SkillExecutor", "Execute", err, time.Since(start), "phase=retrieval")
	}

	// Phase 2: Build LLM Request
	llmRequest := executor.buildLLMRequest(userMessage.Message, skill)

	// Phase 3: LLM Generation
	llmResponse, err := executor.generateResponse(ctx, llmRequest)
	if err != nil {
		return ExecutionResult{}, NewSkillExecutionError(
			llmRequest.SkillID, "generation", err,
		)
	}

	// Phase 4: Format as WSMessage
	wsMessage, err := executor.formatAsWSMessage(llmResponse)
	if err != nil {
		return ExecutionResult{}, NewSkillExecutionError(
			llmRequest.SkillID, "formatting", err,
		)
	}

	utilities.LogSuccess("SkillExecutor", "Execute", time.Since(start),
		fmt.Sprintf("skill_used=%v", skillMeta != nil),
		fmt.Sprintf("tokens=%d", llmResponse.TokensUsed),
		fmt.Sprintf("latency=%s", llmResponse.Latency),
	)

	return ExecutionResult{
		Response:  llmResponse,
		SkillUsed: skillMeta,
		WSMessage: wsMessage,
	}, nil
}

// ExecuteWithSkill processes a user message using a specific skill (by ID),
// bypassing the automatic retrieval step.
func (executor *SkillExecutor) ExecuteWithSkill(
	ctx context.Context,
	userMessage models.UserChatData,
	skillID string,
) (ExecutionResult, error) {
	start := time.Now()
	utilities.LogStart("SkillExecutor", "ExecuteWithSkill")

	skill, exists := executor.registry.GetSkill(skillID)
	if !exists {
		return ExecutionResult{}, NewSkillExecutionError(
			skillID, "lookup", fmt.Errorf("skill %q not found in registry", skillID),
		)
	}

	skillContext := FormatSkillAsContext(skill)
	llmRequest := LLMRequest{
		SystemPrompt: executor.systemPrompt,
		UserMessage:  userMessage.Message,
		SkillContext: skillContext,
		SkillID:      skillID,
	}

	llmResponse, err := executor.generateResponse(ctx, llmRequest)
	if err != nil {
		return ExecutionResult{}, NewSkillExecutionError(skillID, "generation", err)
	}

	wsMessage, err := executor.formatAsWSMessage(llmResponse)
	if err != nil {
		return ExecutionResult{}, NewSkillExecutionError(skillID, "formatting", err)
	}

	meta := models.SkillMetadata{
		SkillIdentifier:  skill.SkillIdentifier,
		SkillDisplayName: skill.SkillDisplayName,
		SkillDescription: skill.SkillDescription,
		SearchKeywords:   skill.SearchKeywords,
		SkillCategory:    skill.SkillCategory,
		SchemaVersion:    skill.SchemaVersion,
	}

	utilities.LogSuccess("SkillExecutor", "ExecuteWithSkill", time.Since(start),
		fmt.Sprintf("skill=%s", skillID),
		fmt.Sprintf("tokens=%d", llmResponse.TokensUsed),
	)

	return ExecutionResult{
		Response:  llmResponse,
		SkillUsed: &meta,
		WSMessage: wsMessage,
	}, nil
}

// retrieveSkill finds the best matching skill for the user's query.
// Returns nil skill and metadata if no skill meets the threshold.
func (executor *SkillExecutor) retrieveSkill(userQuery string) (*models.SkillDefinition, *models.SkillMetadata, error) {
	skill := executor.registry.RetrieveBestSkill(userQuery)
	if skill == nil {
		utilities.LogProgress("SkillExecutor", "retrieveSkill",
			"No matching skill found, proceeding without skill context",
			fmt.Sprintf("query=%q", truncate(userQuery, 50)),
		)
		return nil, nil, nil
	}

	meta := &models.SkillMetadata{
		SkillIdentifier:  skill.SkillIdentifier,
		SkillDisplayName: skill.SkillDisplayName,
		SkillDescription: skill.SkillDescription,
		SearchKeywords:   skill.SearchKeywords,
		SkillCategory:    skill.SkillCategory,
		SchemaVersion:    skill.SchemaVersion,
	}

	utilities.LogProgress("SkillExecutor", "retrieveSkill",
		fmt.Sprintf("Selected skill: %s (%s)", skill.SkillDisplayName, skill.SkillIdentifier),
	)

	return skill, meta, nil
}

// buildLLMRequest constructs the LLM request with optional skill context.
func (executor *SkillExecutor) buildLLMRequest(userMessage string, skill *models.SkillDefinition) LLMRequest {
	request := LLMRequest{
		SystemPrompt: executor.systemPrompt,
		UserMessage:  userMessage,
	}

	if skill != nil {
		request.SkillContext = FormatSkillAsContext(*skill)
		request.SkillID = skill.SkillIdentifier
	}

	return request
}

// generateResponse calls the LLM service with retry logic.
func (executor *SkillExecutor) generateResponse(ctx context.Context, request LLMRequest) (LLMResponse, error) {
	var lastErr error
	var response LLMResponse

	err := utilities.RetryWithBackoff("LLM-Generate", 3, 500*time.Millisecond, func() error {
		var genErr error
		response, genErr = executor.llmService.Generate(ctx, request)
		if genErr != nil {
			lastErr = genErr
			return genErr
		}
		return nil
	})

	if err != nil {
		return LLMResponse{}, fmt.Errorf("LLM generation failed after retries: %w", lastErr)
	}

	return response, nil
}

// formatAsWSMessage converts an LLM response into a validated WSMessage.
func (executor *SkillExecutor) formatAsWSMessage(response LLMResponse) (models.WSMessage, error) {
	chatData := models.SystemChatData{
		Event:   "llm_response",
		Message: response.Content,
	}

	dataBytes, err := json.Marshal(chatData)
	if err != nil {
		return models.WSMessage{}, fmt.Errorf("failed to marshal system chat data: %w", err)
	}

	wsMessage := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		SkillsId:  response.SkillID,
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(wsMessage); err != nil {
		return models.WSMessage{}, fmt.Errorf("generated message failed validation: %w", err)
	}

	return wsMessage, nil
}

// ValidateWSMessage ensures a WSMessage conforms to the required structure.
func ValidateWSMessage(msg models.WSMessage) error {
	if msg.Type == "" {
		return fmt.Errorf("message type is required")
	}

	if msg.Type != models.UserChat && msg.Type != models.SystemChat {
		return fmt.Errorf("invalid message type: %s", msg.Type)
	}

	if len(msg.Data) == 0 {
		return fmt.Errorf("message data is required")
	}

	if !json.Valid(msg.Data) {
		return fmt.Errorf("message data is not valid JSON")
	}

	if msg.Timestamp.IsZero() {
		return fmt.Errorf("message timestamp is required")
	}

	switch msg.Type {
	case models.UserChat:
		var userData models.UserChatData
		if err := json.Unmarshal(msg.Data, &userData); err != nil {
			return fmt.Errorf("user_chat data does not match UserChatData schema: %w", err)
		}
		if userData.Message == "" {
			return fmt.Errorf("user_chat message content is required")
		}
	case models.SystemChat:
		var sysData models.SystemChatData
		if err := json.Unmarshal(msg.Data, &sysData); err != nil {
			return fmt.Errorf("system_chat data does not match SystemChatData schema: %w", err)
		}
		if sysData.Event == "" {
			return fmt.Errorf("system_chat event type is required")
		}
	}

	return nil
}

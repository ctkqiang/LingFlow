package services

import (
	"context"
	"encoding/json"
	"fmt"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"time"
)

const defaultSystemPrompt = "你是 LingFlow，一个智能助手。" +
	"请准确且有帮助地回答用户的问题。" +
	"如果提供了技能上下文，请使用它来指导你的回答。"

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

// NewSkillExecutionError 创建一个新的 SkillExecutionError 实例。
func NewSkillExecutionError(skillID, phase string, cause error) *SkillExecutionError {
	return &SkillExecutionError{
		SkillID:   skillID,
		Phase:     phase,
		Cause:     cause,
		Timestamp: time.Now(),
	}
}

// SkillExecutor 编排完整的处理管道：技能检索、上下文注入、LLM 生成和响应格式化。
type SkillExecutor struct {
	registry     *SkillRegistry
	llmService   LLMService
	systemPrompt string
}

// NewSkillExecutor 使用给定的依赖创建一个新的技能执行器。
func NewSkillExecutor(registry *SkillRegistry, llmService LLMService) *SkillExecutor {
	return &SkillExecutor{
		registry:     registry,
		llmService:   llmService,
		systemPrompt: defaultSystemPrompt,
	}
}

// SetSystemPrompt 覆盖默认的系统提示词。
func (executor *SkillExecutor) SetSystemPrompt(prompt string) {
	executor.systemPrompt = prompt
}

// ExecutionResult 保存技能增强 LLM 执行的完整结果。
type ExecutionResult struct {
	Response    LLMResponse
	SkillUsed   *models.SkillMetadata
	WSMessage   models.WSMessage
}

// Execute 通过完整管道处理用户消息：
// 1. 为用户查询检索最佳匹配技能
// 2. 为 LLM 构建技能增强上下文
// 3. 通过 LLM 服务生成响应
// 4. 将响应格式化为 WSMessage
func (executor *SkillExecutor) Execute(ctx context.Context, userMessage models.UserChatData) (ExecutionResult, error) {
	start := time.Now()
	utilities.LogStart("SkillExecutor", "Execute")

	// 阶段 1: 技能检索
	skill, skillMeta, err := executor.retrieveSkill(userMessage.Message)
	if err != nil {
		utilities.LogError("SkillExecutor", "Execute", err, time.Since(start), "phase=retrieval")
	}

	// 阶段 2: 构建 LLM 请求
	llmRequest := executor.buildLLMRequest(userMessage.Message, skill)

	// 阶段 3: LLM 生成
	llmResponse, err := executor.generateResponse(ctx, llmRequest)
	if err != nil {
		return ExecutionResult{}, NewSkillExecutionError(
			llmRequest.SkillID, "generation", err,
		)
	}

	// 阶段 4: 格式化为 WSMessage
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

// ExecuteWithSkill 使用指定技能（按 ID）处理用户消息，跳过自动检索步骤。
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
			skillID, "lookup", fmt.Errorf("技能 %q 未在注册中心找到", skillID),
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

// retrieveSkill 为用户查询找到最佳匹配的技能。
// 若没有技能达到阈值，返回 nil。
func (executor *SkillExecutor) retrieveSkill(userQuery string) (*models.SkillDefinition, *models.SkillMetadata, error) {
	skill := executor.registry.RetrieveBestSkill(userQuery)
	if skill == nil {
		utilities.LogProgress("SkillExecutor", "retrieveSkill",
			"未找到匹配的技能，将在无技能上下文的情况下继续",
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
		fmt.Sprintf("已选择技能: %s (%s)", skill.SkillDisplayName, skill.SkillIdentifier),
	)

	return skill, meta, nil
}

// buildLLMRequest 构建带有可选技能上下文的 LLM 请求。
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

// generateResponse 调用 LLM 服务，包含重试逻辑。
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
		return LLMResponse{}, fmt.Errorf("LLM 生成在重试后仍然失败: %w", lastErr)
	}

	return response, nil
}

// formatAsWSMessage 将 LLM 响应转换为经过校验的 WSMessage。
func (executor *SkillExecutor) formatAsWSMessage(response LLMResponse) (models.WSMessage, error) {
	chatData := models.SystemChatData{
		Event:   "llm_response",
		Message: response.Content,
	}

	dataBytes, err := json.Marshal(chatData)
	if err != nil {
		return models.WSMessage{}, fmt.Errorf("序列化系统聊天数据失败: %w", err)
	}

	wsMessage := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		SkillsId:  response.SkillID,
		Timestamp: time.Now(),
	}

	if err := ValidateWSMessage(wsMessage); err != nil {
		return models.WSMessage{}, fmt.Errorf("生成的消息未通过校验: %w", err)
	}

	return wsMessage, nil
}

// ValidateWSMessage 确保 WSMessage 符合要求的结构规范。
func ValidateWSMessage(msg models.WSMessage) error {
	if msg.Type == "" {
		return fmt.Errorf("消息类型为必填项")
	}

	if msg.Type != models.UserChat && msg.Type != models.SystemChat && msg.Type != models.HeartbeatChat {
		return fmt.Errorf("无效的消息类型: %s", msg.Type)
	}

	if len(msg.Data) == 0 {
		return fmt.Errorf("消息数据为必填项")
	}

	if !json.Valid(msg.Data) {
		return fmt.Errorf("消息数据不是有效的 JSON")
	}

	if msg.Timestamp.IsZero() {
		return fmt.Errorf("消息时间戳为必填项")
	}

	switch msg.Type {
	case models.UserChat:
		var userData models.UserChatData
		if err := json.Unmarshal(msg.Data, &userData); err != nil {
			return fmt.Errorf("user_chat 数据不符合 UserChatData 结构: %w", err)
		}
		if userData.Message == "" {
			return fmt.Errorf("user_chat 消息内容为必填项")
		}
	case models.SystemChat:
		var sysData models.SystemChatData
		if err := json.Unmarshal(msg.Data, &sysData); err != nil {
			return fmt.Errorf("system_chat 数据不符合 SystemChatData 结构: %w", err)
		}
		if sysData.Event == "" {
			return fmt.Errorf("system_chat 事件类型为必填项")
		}
	case models.HeartbeatChat:
		var heartbeatData models.HeartbeatChatData
		if err := json.Unmarshal(msg.Data, &heartbeatData); err != nil {
			return fmt.Errorf("heartbeat_chat 数据不符合 HeartbeatChatData 结构: %w", err)
		}
		if heartbeatData.Action != "ping" && heartbeatData.Action != "pong" {
			return fmt.Errorf("heartbeat_chat 动作类型必须为 ping 或 pong")
		}
		if heartbeatData.Nonce == "" {
			return fmt.Errorf("heartbeat_chat nonce 为必填项")
		}
	}

	return nil
}

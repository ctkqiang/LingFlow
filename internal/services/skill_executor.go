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

func (executionError *SkillExecutionError) Error() string {
	return fmt.Sprintf("技能执行失败 [skill=%s phase=%s]: %v", executionError.SkillID, executionError.Phase, executionError.Cause)
}

func (executionError *SkillExecutionError) Unwrap() error {
	return executionError.Cause
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
	Response  LLMResponse
	SkillUsed *models.SkillMetadata
	WSMessage models.WSMessage
}

// Execute 通过完整管道处理用户消息：
// 1. 为用户查询检索最佳匹配技能
// 2. 为 LLM 构建技能增强上下文
// 3. 通过 LLM 服务生成响应
// 4. 将响应格式化为 WSMessage
func (executor *SkillExecutor) Execute(ctx context.Context, userMessage models.UserChatData) (ExecutionResult, error) {
	start := time.Now()
	traceID := utilities.NewTraceID()
	utilities.LogStart("SkillExecutor", "Execute")
	utilities.LogVerbose("SkillExecutor", "Execute", "开始完整执行管道",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("message_len=%d", len(userMessage.Message)),
	)

	// ── 阶段1: 技能检索 ──
	retrievalStart := time.Now()
	skill, skillMeta, err := executor.retrieveSkill(userMessage.Message)
	retrievalNs := time.Since(retrievalStart).Nanoseconds()
	if err != nil {
		utilities.LogError("SkillExecutor", "Execute", err, time.Since(retrievalStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=retrieval")
	}
	utilities.LogNano("SkillExecutor", "Execute", utilities.INFO, "RETRIEVAL_DONE",
		time.Since(retrievalStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("retrieval_ns=%d", retrievalNs),
		fmt.Sprintf("skill_found=%v", skill != nil),
	)

	// ── 阶段2: 构建 LLM 请求 ──
	buildStart := time.Now()
	llmRequest := executor.buildLLMRequest(userMessage.Message, skill)
	utilities.LogNano("SkillExecutor", "Execute", utilities.INFO, "REQUEST_BUILT",
		time.Since(buildStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill_id=%s", llmRequest.SkillID),
	)

	// ── 阶段3: LLM 生成 ──
	genStart := time.Now()
	utilities.LogVerbose("SkillExecutor", "Execute", "开始 LLM 生成",
		fmt.Sprintf("trace_id=%s", traceID))
	llmResponse, err := executor.generateResponse(ctx, llmRequest)
	genNs := time.Since(genStart).Nanoseconds()
	if err != nil {
		utilities.LogError("SkillExecutor", "Execute", err, time.Since(genStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=generation",
			fmt.Sprintf("gen_ns=%d", genNs))
		return ExecutionResult{}, NewSkillExecutionError(
			llmRequest.SkillID, "generation", err,
		)
	}
	utilities.LogNano("SkillExecutor", "Execute", utilities.INFO, "GENERATION_DONE",
		time.Since(genStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("gen_ns=%d", genNs),
		fmt.Sprintf("tokens=%d", llmResponse.TokensUsed),
		fmt.Sprintf("latency=%s", llmResponse.Latency),
	)

	// ── 阶段4: 格式化为 WSMessage ──
	fmtStart := time.Now()
	wsMessage, err := executor.formatAsWSMessage(llmResponse)
	fmtNs := time.Since(fmtStart).Nanoseconds()
	if err != nil {
		utilities.LogError("SkillExecutor", "Execute", err, time.Since(fmtStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=formatting",
			fmt.Sprintf("fmt_ns=%d", fmtNs))
		return ExecutionResult{}, NewSkillExecutionError(
			llmRequest.SkillID, "formatting", err,
		)
	}
	utilities.LogNano("SkillExecutor", "Execute", utilities.INFO, "FORMAT_DONE",
		time.Since(fmtStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("fmt_ns=%d", fmtNs),
	)

	totalNs := time.Since(start).Nanoseconds()
	utilities.LogSuccess("SkillExecutor", "Execute", time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("total_ns=%d", totalNs),
		fmt.Sprintf("skill_used=%v", skillMeta != nil),
		fmt.Sprintf("tokens=%d", llmResponse.TokensUsed),
		fmt.Sprintf("latency=%s", llmResponse.Latency),
		fmt.Sprintf("retrieval_ns=%d", retrievalNs),
		fmt.Sprintf("gen_ns=%d", genNs),
		fmt.Sprintf("fmt_ns=%d", fmtNs),
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
	traceID := utilities.NewTraceID()
	utilities.LogStart("SkillExecutor", "ExecuteWithSkill")
	utilities.LogVerbose("SkillExecutor", "ExecuteWithSkill", "开始指定技能执行管道",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("skill_id=%s", skillID),
		fmt.Sprintf("message_len=%d", len(userMessage.Message)),
	)

	// ── 阶段1: 技能查找 ──
	lookupStart := time.Now()
	skill, exists := executor.registry.GetSkill(skillID)
	lookupNs := time.Since(lookupStart).Nanoseconds()
	if !exists {
		err := NewSkillExecutionError(
			skillID, "lookup", fmt.Errorf("技能 %q 未在注册中心找到", skillID),
		)
		utilities.LogError("SkillExecutor", "ExecuteWithSkill", err, time.Since(lookupStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=lookup",
			fmt.Sprintf("lookup_ns=%d", lookupNs))
		return ExecutionResult{}, err
	}
	utilities.LogNano("SkillExecutor", "ExecuteWithSkill", utilities.INFO, "LOOKUP_OK",
		time.Since(lookupStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("lookup_ns=%d", lookupNs),
		fmt.Sprintf("skill_display_name=%s", skill.SkillDisplayName),
		fmt.Sprintf("skill_category=%s", skill.SkillCategory),
	)

	// ── 阶段2: 构建上下文和请求 ──
	ctxStart := time.Now()
	skillContext := FormatSkillAsContext(skill)
	llmRequest := LLMRequest{
		SystemPrompt: executor.systemPrompt,
		UserMessage:  userMessage.Message,
		SkillContext: skillContext,
		SkillID:      skillID,
	}
	utilities.LogNano("SkillExecutor", "ExecuteWithSkill", utilities.INFO, "CONTEXT_FORMATTED",
		time.Since(ctxStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("context_len=%d", len(skillContext)),
	)

	// ── 阶段3: LLM 生成 ──
	genStart := time.Now()
	utilities.LogVerbose("SkillExecutor", "ExecuteWithSkill", "开始 LLM 生成",
		fmt.Sprintf("trace_id=%s", traceID), fmt.Sprintf("skill_id=%s", skillID))
	llmResponse, err := executor.generateResponse(ctx, llmRequest)
	genNs := time.Since(genStart).Nanoseconds()
	if err != nil {
		utilities.LogError("SkillExecutor", "ExecuteWithSkill", err, time.Since(genStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=generation",
			fmt.Sprintf("gen_ns=%d", genNs))
		return ExecutionResult{}, NewSkillExecutionError(skillID, "generation", err)
	}
	utilities.LogNano("SkillExecutor", "ExecuteWithSkill", utilities.INFO, "GENERATION_DONE",
		time.Since(genStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("gen_ns=%d", genNs),
		fmt.Sprintf("tokens=%d", llmResponse.TokensUsed),
	)

	// ── 阶段4: 格式化为 WSMessage ──
	fmtStart := time.Now()
	wsMessage, err := executor.formatAsWSMessage(llmResponse)
	fmtNs := time.Since(fmtStart).Nanoseconds()
	if err != nil {
		utilities.LogError("SkillExecutor", "ExecuteWithSkill", err, time.Since(fmtStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=formatting",
			fmt.Sprintf("fmt_ns=%d", fmtNs))
		return ExecutionResult{}, NewSkillExecutionError(skillID, "formatting", err)
	}
	utilities.LogNano("SkillExecutor", "ExecuteWithSkill", utilities.INFO, "FORMAT_DONE",
		time.Since(fmtStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("fmt_ns=%d", fmtNs),
	)

	skillMetadata := models.SkillMetadata{
		SkillIdentifier:  skill.SkillIdentifier,
		SkillDisplayName: skill.SkillDisplayName,
		SkillDescription: skill.SkillDescription,
		SearchKeywords:   skill.SearchKeywords,
		SkillCategory:    skill.SkillCategory,
		SchemaVersion:    skill.SchemaVersion,
	}

	totalNs := time.Since(start).Nanoseconds()
	utilities.LogSuccess("SkillExecutor", "ExecuteWithSkill", time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("total_ns=%d", totalNs),
		fmt.Sprintf("skill=%s", skillID),
		fmt.Sprintf("tokens=%d", llmResponse.TokensUsed),
		fmt.Sprintf("lookup_ns=%d", lookupNs),
		fmt.Sprintf("gen_ns=%d", genNs),
		fmt.Sprintf("fmt_ns=%d", fmtNs),
	)

	return ExecutionResult{
		Response:  llmResponse,
		SkillUsed: &skillMetadata,
		WSMessage: wsMessage,
	}, nil
}

// retrieveSkill 为用户查询找到最佳匹配的技能。
// 若没有技能达到阈值，返回 nil。
func (executor *SkillExecutor) retrieveSkill(userQuery string) (*models.SkillDefinition, *models.SkillMetadata, error) {
	retrieveStart := time.Now()
	utilities.LogStart("SkillExecutor", "retrieveSkill")
	utilities.LogVerbose("SkillExecutor", "retrieveSkill", "开始技能检索",
		fmt.Sprintf("query=%q", truncate(userQuery, 100)),
		fmt.Sprintf("query_len=%d", len(userQuery)),
	)

	skill := executor.registry.RetrieveBestSkill(userQuery)
	retrieveNs := time.Since(retrieveStart).Nanoseconds()

	if skill == nil {
		utilities.LogNano("SkillExecutor", "retrieveSkill", utilities.INFO, "NO_MATCH",
			time.Since(retrieveStart),
			fmt.Sprintf("retrieve_ns=%d", retrieveNs),
			fmt.Sprintf("query=%q", truncate(userQuery, 50)),
		)
		utilities.LogProgress("SkillExecutor", "retrieveSkill",
			"未找到匹配的技能，将在无技能上下文的情况下继续",
			fmt.Sprintf("query=%q", truncate(userQuery, 50)),
		)
		return nil, nil, nil
	}

	skillMetadata := &models.SkillMetadata{
		SkillIdentifier:  skill.SkillIdentifier,
		SkillDisplayName: skill.SkillDisplayName,
		SkillDescription: skill.SkillDescription,
		SearchKeywords:   skill.SearchKeywords,
		SkillCategory:    skill.SkillCategory,
		SchemaVersion:    skill.SchemaVersion,
	}

	utilities.LogNano("SkillExecutor", "retrieveSkill", utilities.INFO, "MATCH_FOUND",
		time.Since(retrieveStart),
		fmt.Sprintf("retrieve_ns=%d", retrieveNs),
		fmt.Sprintf("skill_id=%s", skill.SkillIdentifier),
		fmt.Sprintf("skill_name=%s", skill.SkillDisplayName),
		fmt.Sprintf("category=%s", skill.SkillCategory),
	)

	utilities.LogProgress("SkillExecutor", "retrieveSkill",
		fmt.Sprintf("已选择技能: %s (%s)", skill.SkillDisplayName, skill.SkillIdentifier),
	)

	return skill, skillMetadata, nil
}

// buildLLMRequest 构建带有可选技能上下文的 LLM 请求。
func (executor *SkillExecutor) buildLLMRequest(userMessage string, skill *models.SkillDefinition) LLMRequest {
	buildStart := time.Now()
	utilities.LogStart("SkillExecutor", "buildLLMRequest")

	request := LLMRequest{
		SystemPrompt: executor.systemPrompt,
		UserMessage:  userMessage,
	}

	if skill != nil {
		request.SkillContext = FormatSkillAsContext(*skill)
		request.SkillID = skill.SkillIdentifier
	}

	utilities.LogNano("SkillExecutor", "buildLLMRequest", utilities.INFO, "REQUEST_BUILT",
		time.Since(buildStart),
		fmt.Sprintf("system_prompt_len=%d", len(executor.systemPrompt)),
		fmt.Sprintf("user_message_len=%d", len(userMessage)),
		fmt.Sprintf("skill_context_len=%d", len(request.SkillContext)),
		fmt.Sprintf("has_skill=%v", skill != nil),
		fmt.Sprintf("skill_id=%s", request.SkillID),
	)

	return request
}

// generateResponse 调用 LLM 服务，包含重试逻辑。
func (executor *SkillExecutor) generateResponse(ctx context.Context, request LLMRequest) (LLMResponse, error) {
	genStart := time.Now()
	utilities.LogStart("SkillExecutor", "generateResponse")
	utilities.LogVerbose("SkillExecutor", "generateResponse", "开始 LLM 生成（含重试）",
		fmt.Sprintf("skill_id=%s", request.SkillID),
		fmt.Sprintf("max_retries=3"),
		fmt.Sprintf("backoff=500ms"),
	)

	var lastErr error
	var response LLMResponse
	attempt := 0

	err := utilities.RetryWithBackoff("LLM-Generate", 3, 500*time.Millisecond, func() error {
		attempt++
		attemptStart := time.Now()
		utilities.LogVerbose("SkillExecutor", "generateResponse",
			fmt.Sprintf("第 %d 次尝试 LLM 生成", attempt),
			fmt.Sprintf("skill_id=%s", request.SkillID),
		)

		var genErr error
		response, genErr = executor.llmService.Generate(ctx, request)
		if genErr != nil {
			lastErr = genErr
			utilities.LogWarn("SkillExecutor", "generateResponse",
				fmt.Sprintf("第 %d 次尝试失败: %v", attempt, genErr),
				time.Since(attemptStart),
				fmt.Sprintf("skill_id=%s", request.SkillID),
			)
			return genErr
		}

		utilities.LogNano("SkillExecutor", "generateResponse", utilities.INFO,
			fmt.Sprintf("ATTEMPT_%d_OK", attempt),
			time.Since(attemptStart),
			fmt.Sprintf("tokens=%d", response.TokensUsed),
			fmt.Sprintf("finish_reason=%s", response.FinishReason),
			fmt.Sprintf("latency=%s", response.Latency),
		)
		return nil
	})

	if err != nil {
		utilities.LogError("SkillExecutor", "generateResponse", lastErr, time.Since(genStart),
			fmt.Sprintf("total_attempts=%d", attempt),
			fmt.Sprintf("skill_id=%s", request.SkillID),
		)
		return LLMResponse{}, fmt.Errorf("LLM 生成在重试后仍然失败: %w", lastErr)
	}

	utilities.LogSuccess("SkillExecutor", "generateResponse", time.Since(genStart),
		fmt.Sprintf("total_attempts=%d", attempt),
		fmt.Sprintf("tokens=%d", response.TokensUsed),
		fmt.Sprintf("finish_reason=%s", response.FinishReason),
		fmt.Sprintf("latency=%s", response.Latency),
	)

	return response, nil
}

// formatAsWSMessage 将 LLM 响应转换为经过校验的 WSMessage。
func (executor *SkillExecutor) formatAsWSMessage(response LLMResponse) (models.WSMessage, error) {
	fmtStart := time.Now()
	utilities.LogStart("SkillExecutor", "formatAsWSMessage")

	chatData := models.SystemChatData{
		Event:   "llm_response",
		Message: response.Content,
	}

	dataBytes, err := json.Marshal(chatData)
	if err != nil {
		utilities.LogError("SkillExecutor", "formatAsWSMessage", err, time.Since(fmtStart), "phase=serialize")
		return models.WSMessage{}, fmt.Errorf("序列化系统聊天数据失败: %w", err)
	}

	utilities.LogVerbose("SkillExecutor", "formatAsWSMessage", "序列化完成",
		fmt.Sprintf("data_size_bytes=%d", len(dataBytes)),
		fmt.Sprintf("content_len=%d", len(response.Content)),
		fmt.Sprintf("skill_id=%s", response.SkillID),
	)

	wsMessage := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		SkillsId:  response.SkillID,
		Timestamp: time.Now(),
	}

	validateStart := time.Now()
	if err := ValidateWSMessage(wsMessage); err != nil {
		utilities.LogError("SkillExecutor", "formatAsWSMessage", err, time.Since(validateStart),
			"phase=validate")
		return models.WSMessage{}, fmt.Errorf("生成的消息未通过校验: %w", err)
	}

	utilities.LogNano("SkillExecutor", "formatAsWSMessage", utilities.INFO, "FORMAT_VALIDATED",
		time.Since(fmtStart),
		fmt.Sprintf("validate_ns=%d", time.Since(validateStart).Nanoseconds()),
		fmt.Sprintf("total_ns=%d", time.Since(fmtStart).Nanoseconds()),
	)

	return wsMessage, nil
}

// ValidateWSMessage 确保 WSMessage 符合要求的结构规范。
func ValidateWSMessage(msg models.WSMessage) error {
	validateStart := time.Now()
	utilities.LogVerbose("SkillExecutor", "ValidateWSMessage", "开始消息校验",
		fmt.Sprintf("type=%s", msg.Type),
		fmt.Sprintf("data_len=%d", len(msg.Data)),
		fmt.Sprintf("skill_id=%s", msg.SkillsId),
	)

	// 校验步骤1: 类型非空
	if msg.Type == "" {
		utilities.LogWarn("SkillExecutor", "ValidateWSMessage", "校验失败: 消息类型为空",
			time.Since(validateStart))
		return fmt.Errorf("消息类型为必填项")
	}

	// 校验步骤2: 类型合法性
	if msg.Type != models.UserChat && msg.Type != models.SystemChat &&
		msg.Type != models.SystemThinking && msg.Type != models.SystemResponse &&
		msg.Type != models.HeartbeatChat {
		utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
			fmt.Sprintf("校验失败: 无效的消息类型 %s", msg.Type), time.Since(validateStart))
		return fmt.Errorf("无效的消息类型: %s", msg.Type)
	}

	// 校验步骤3: 数据非空
	if len(msg.Data) == 0 {
		utilities.LogWarn("SkillExecutor", "ValidateWSMessage", "校验失败: 消息数据为空",
			time.Since(validateStart))
		return fmt.Errorf("消息数据为必填项")
	}

	// 校验步骤4: JSON 有效性
	if !json.Valid(msg.Data) {
		utilities.LogWarn("SkillExecutor", "ValidateWSMessage", "校验失败: 消息数据不是有效的 JSON",
			time.Since(validateStart))
		return fmt.Errorf("消息数据不是有效的 JSON")
	}

	// 校验步骤5: 时间戳非零
	if msg.Timestamp.IsZero() {
		utilities.LogWarn("SkillExecutor", "ValidateWSMessage", "校验失败: 时间戳为零值",
			time.Since(validateStart))
		return fmt.Errorf("消息时间戳为必填项")
	}

	// 校验步骤6: 按类型校验数据结构
	switch msg.Type {
	case models.UserChat:
		var userData models.UserChatData
		if err := json.Unmarshal(msg.Data, &userData); err != nil {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				fmt.Sprintf("校验失败: user_chat 数据结构不匹配: %v", err), time.Since(validateStart))
			return fmt.Errorf("user_chat 数据不符合 UserChatData 结构: %w", err)
		}
		if userData.Message == "" {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				"校验失败: user_chat 消息内容为空", time.Since(validateStart))
			return fmt.Errorf("user_chat 消息内容为必填项")
		}
	case models.SystemChat:
		var sysData models.SystemChatData
		if err := json.Unmarshal(msg.Data, &sysData); err != nil {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				fmt.Sprintf("校验失败: system_chat 数据结构不匹配: %v", err), time.Since(validateStart))
			return fmt.Errorf("system_chat 数据不符合 SystemChatData 结构: %w", err)
		}
		if sysData.Event == "" {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				"校验失败: system_chat 事件类型为空", time.Since(validateStart))
			return fmt.Errorf("system_chat 事件类型为必填项")
		}
	case models.HeartbeatChat:
		var heartbeatData models.HeartbeatChatData
		if err := json.Unmarshal(msg.Data, &heartbeatData); err != nil {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				fmt.Sprintf("校验失败: heartbeat_chat 数据结构不匹配: %v", err), time.Since(validateStart))
			return fmt.Errorf("heartbeat_chat 数据不符合 HeartbeatChatData 结构: %w", err)
		}
		if heartbeatData.Action != "ping" && heartbeatData.Action != "pong" {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				fmt.Sprintf("校验失败: heartbeat_chat 动作类型无效: %s", heartbeatData.Action),
				time.Since(validateStart))
			return fmt.Errorf("heartbeat_chat 动作类型必须为 ping 或 pong")
		}
		if heartbeatData.Nonce == "" {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				"校验失败: heartbeat_chat nonce 为空", time.Since(validateStart))
			return fmt.Errorf("heartbeat_chat nonce 为必填项")
		}
	case models.SystemThinking:
		var thinkingData models.SystemThinkingData
		if err := json.Unmarshal(msg.Data, &thinkingData); err != nil {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				fmt.Sprintf("校验失败: system_thinking 数据结构不匹配: %v", err), time.Since(validateStart))
			return fmt.Errorf("system_thinking 数据不符合 SystemThinkingData 结构: %w", err)
		}
		if thinkingData.Phase == "" {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				"校验失败: system_thinking phase 为空", time.Since(validateStart))
			return fmt.Errorf("system_thinking phase 为必填项")
		}
	case models.SystemResponse:
		var responseData models.SystemResponseData
		if err := json.Unmarshal(msg.Data, &responseData); err != nil {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				fmt.Sprintf("校验失败: system_response 数据结构不匹配: %v", err), time.Since(validateStart))
			return fmt.Errorf("system_response 数据不符合 SystemResponseData 结构: %w", err)
		}
		if responseData.Content == "" {
			utilities.LogWarn("SkillExecutor", "ValidateWSMessage",
				"校验失败: system_response content 为空", time.Since(validateStart))
			return fmt.Errorf("system_response content 为必填项")
		}
	}

	utilities.LogNano("SkillExecutor", "ValidateWSMessage", utilities.INFO, "VALIDATE_PASS",
		time.Since(validateStart),
		fmt.Sprintf("type=%s", msg.Type),
		fmt.Sprintf("validate_ns=%d", time.Since(validateStart).Nanoseconds()),
	)

	return nil
}

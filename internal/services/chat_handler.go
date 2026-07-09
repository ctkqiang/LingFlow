package services

import (
	"context"
	"encoding/json"
	"fmt"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"time"
)

// MessageSender 定义了向连接发送消息的能力。
type MessageSender interface {
	SendMessage(connectionID string, payload []byte) error
}

// ChatHandler 通过技能增强的 LLM 管道处理传入的 WebSocket 消息，并生成响应消息。
type ChatHandler struct {
	executor      *SkillExecutor
	registry      *SkillRegistry
	s3Loader      *S3SkillLoader
	messageSender MessageSender
}

// NewChatHandler 创建一个完整注入依赖的 ChatHandler 实例。
func NewChatHandler(registry *SkillRegistry, llmService LLMService) *ChatHandler {
	return &ChatHandler{
		executor: NewSkillExecutor(registry, llmService),
		registry: registry,
	}
}

// NewChatHandlerWithS3Loader 创建带 S3 技能加载器的 ChatHandler。
func NewChatHandlerWithS3Loader(
	registry *SkillRegistry,
	llmService LLMService,
	s3Loader *S3SkillLoader,
	messageSender MessageSender,
) *ChatHandler {
	return &ChatHandler{
		executor:      NewSkillExecutor(registry, llmService),
		registry:      registry,
		s3Loader:      s3Loader,
		messageSender: messageSender,
	}
}

// NewDefaultChatHandler 使用基于环境变量的 Bedrock 配置创建 ChatHandler。
// 通过 AWS 默认凭证链初始化 Bedrock 客户端。
func NewDefaultChatHandler(ctx context.Context) (*ChatHandler, error) {
	registry := NewSkillRegistry()
	bedrockConfig := NewBedrockConfig()
	llmService, err := NewBedrockLLMService(ctx, bedrockConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化 Bedrock LLM 服务失败: %w", err)
	}
	return NewChatHandler(registry, llmService), nil
}

// HandleIncomingMessage 处理原始 WebSocket 消息载荷。
// 解析 WSMessage，根据类型路由，并返回响应 WSMessage。
func (handler *ChatHandler) HandleIncomingMessage(
	ctx context.Context,
	rawPayload []byte,
) ([]byte, error) {
	start := time.Now()
	utilities.LogStart("ChatHandler", "HandleIncomingMessage")

	incomingMessage, err := handler.parseIncomingMessage(rawPayload)
	if err != nil {
		errorResponse := handler.buildErrorResponse("parse_error", err.Error())
		return json.Marshal(errorResponse)
	}

	if err := ValidateWSMessage(incomingMessage); err != nil {
		errorResponse := handler.buildErrorResponse("validation_error", err.Error())
		return json.Marshal(errorResponse)
	}

	var responseMessage models.WSMessage

	switch incomingMessage.Type {
	case models.UserChat:
		responseMessage, err = handler.handleUserChat(ctx, incomingMessage)
	case models.SystemChat:
		responseMessage, err = handler.handleSystemChat(incomingMessage)
	case models.SystemThinking:
		responseMessage, err = handler.handleSystemMessage(incomingMessage)
	case models.SystemResponse:
		responseMessage, err = handler.handleSystemMessage(incomingMessage)
	case models.HeartbeatChat:
		responseMessage, err = handler.handleHeartbeat(incomingMessage)
	default:
		err = fmt.Errorf("不支持的消息类型: %s", incomingMessage.Type)
	}

	if err != nil {
		utilities.LogError("ChatHandler", "HandleIncomingMessage", err, time.Since(start))
		errorResponse := handler.buildErrorResponse("processing_error", err.Error())
		return json.Marshal(errorResponse)
	}

	responseBytes, err := json.Marshal(responseMessage)
	if err != nil {
		return nil, fmt.Errorf("序列化响应消息失败: %w", err)
	}

	utilities.LogSuccess("ChatHandler", "HandleIncomingMessage", time.Since(start),
		fmt.Sprintf("type=%s", incomingMessage.Type),
		fmt.Sprintf("skill=%s", incomingMessage.SkillsId),
	)

	return responseBytes, nil
}

// handleUserChat 通过 LLM 管道处理 user_chat 类型的消息。
func (handler *ChatHandler) handleUserChat(
	ctx context.Context,
	message models.WSMessage,
) (models.WSMessage, error) {
	// 解析用户聊天数据
	var userData models.UserChatData
	if err := json.Unmarshal(message.Data, &userData); err != nil {
		return models.WSMessage{}, fmt.Errorf("解析用户聊天数据失败: %w", err)
	}

	if userData.Message == "" {
		return models.WSMessage{}, fmt.Errorf("用户消息内容为空")
	}

	// 通过技能管道执行
	var result ExecutionResult
	var err error

	if message.SkillsId != "" {
		// 客户端显式指定了技能
		result, err = handler.executor.ExecuteWithSkill(ctx, userData, message.SkillsId)
	} else {
		// 从用户消息自动检测技能
		result, err = handler.executor.Execute(ctx, userData)
	}

	if err != nil {
		// 若尚未包装为 SkillExecutionError 则进行包装
		if _, ok := err.(*SkillExecutionError); !ok {
			err = NewSkillExecutionError(message.SkillsId, "execute", err)
		}
		return models.WSMessage{}, err
	}

	return result.WSMessage, nil
}

// handleSystemChat 处理 system_chat 类型的消息（透传或确认应答）。
func (handler *ChatHandler) handleSystemChat(
	message models.WSMessage,
) (models.WSMessage, error) {
	var sysData models.SystemChatData
	if err := json.Unmarshal(message.Data, &sysData); err != nil {
		return models.WSMessage{}, fmt.Errorf("解析系统聊天数据失败: %w", err)
	}

	acknowledgementData := models.SystemChatData{
		Event:   "system_ack",
		Message: fmt.Sprintf("已确认系统事件: %s", sysData.Event),
	}

	dataBytes, err := json.Marshal(acknowledgementData)
	if err != nil {
		return models.WSMessage{}, fmt.Errorf("序列化确认应答数据失败: %w", err)
	}

	return models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		SkillsId:  message.SkillsId,
		Timestamp: time.Now(),
	}, nil
}

// handleHeartbeat 处理 heartbeat_chat 类型的消息。
// 收到 ping 时返回 pong，包含往返延迟。
func (handler *ChatHandler) handleHeartbeat(
	message models.WSMessage,
) (models.WSMessage, error) {
	var heartbeatData models.HeartbeatChatData
	if err := json.Unmarshal(message.Data, &heartbeatData); err != nil {
		return models.WSMessage{}, fmt.Errorf("解析心跳数据失败: %w", err)
	}

	if heartbeatData.Action != "ping" {
		return models.WSMessage{}, fmt.Errorf("heartbeat_chat 只支持 ping 动作")
	}

	latency := time.Since(heartbeatData.Timestamp).Milliseconds()
	pongResponseData := models.HeartbeatChatData{
		Action:    "pong",
		Nonce:     heartbeatData.Nonce,
		Timestamp: time.Now(),
		Latency:   latency,
	}

	dataBytes, err := json.Marshal(pongResponseData)
	if err != nil {
		return models.WSMessage{}, fmt.Errorf("序列化 pong 数据失败: %w", err)
	}

	return models.WSMessage{
		Type:      models.HeartbeatChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}, nil
}

// handleSystemMessage 处理 system_thinking 和 system_response 类型的消息，直接透传。
func (handler *ChatHandler) handleSystemMessage(
	message models.WSMessage,
) (models.WSMessage, error) {
	return message, nil
}

// parseIncomingMessage 将原始载荷反序列化为 WSMessage。
func (handler *ChatHandler) parseIncomingMessage(rawPayload []byte) (models.WSMessage, error) {
	var message models.WSMessage
	if err := json.Unmarshal(rawPayload, &message); err != nil {
		return models.WSMessage{}, fmt.Errorf("无效的消息格式: %w", err)
	}
	return message, nil
}

// buildErrorResponse 创建标准化的错误 WSMessage。
func (handler *ChatHandler) buildErrorResponse(event, errorMessage string) models.WSMessage {
	errorData := models.SystemChatData{
		Event:   event,
		Message: errorMessage,
	}

	dataBytes, _ := json.Marshal(errorData)

	return models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}
}

// OnConnectionReady 是 ConnectionReadyNotifier 接口的实现。
// 连接建立后，向客户端推送可用技能列表。
func (handler *ChatHandler) OnConnectionReady(ctx context.Context, connectionID string) {
	if handler.messageSender == nil {
		return
	}

	skills := handler.registry.ListSkills()
	skillItems := make([]models.SkillListItem, 0, len(skills))
	for _, s := range skills {
		skillItems = append(skillItems, models.SkillListItem{
			SkillIdentifier:  s.SkillIdentifier,
			SkillDisplayName: s.SkillDisplayName,
			SkillDescription: s.SkillDescription,
			SkillCategory:    s.SkillCategory,
			SearchKeywords:   s.SearchKeywords,
		})
	}

	source := "local"
	if handler.s3Loader != nil {
		source = "s3"
	}

	listData := models.SystemSkillsListData{
		Skills:    skillItems,
		Total:     len(skillItems),
		Source:    source,
		UpdatedAt: time.Now(),
	}

	dataBytes, err := json.Marshal(listData)
	if err != nil {
		utilities.LogError("ChatHandler", "OnConnectionReady", err, 0)
		return
	}

	listMessage := models.WSMessage{
		Type:      models.SystemSkillsList,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}

	payload, err := json.Marshal(listMessage)
	if err != nil {
		utilities.LogError("ChatHandler", "OnConnectionReady", err, 0)
		return
	}

	if sendErr := handler.messageSender.SendMessage(connectionID, payload); sendErr != nil {
		utilities.LogWarn("ChatHandler", "OnConnectionReady",
			fmt.Sprintf("发送技能列表失败: %v", sendErr), 0)
	}
}

// HandleUserChatWithStreaming 是 ChatStreamer 接口的实现。
// 检查消息是否为 user_chat，是则异步启动流式处理并返回 true。
func (handler *ChatHandler) HandleUserChatWithStreaming(
	ctx context.Context,
	connectionID string,
	messagePayload []byte,
) bool {
	if handler.messageSender == nil {
		return false
	}

	var incomingMessage models.WSMessage
	if err := json.Unmarshal(messagePayload, &incomingMessage); err != nil {
		return false
	}

	if incomingMessage.Type != models.UserChat {
		return false
	}

	if err := ValidateWSMessage(incomingMessage); err != nil {
		handler.sendError(connectionID, "validation_error", err.Error())
		return true
	}

	go handler.processUserChatStream(ctx, connectionID, incomingMessage)
	return true
}

func (handler *ChatHandler) processUserChatStream(
	ctx context.Context,
	connectionID string,
	incomingMessage models.WSMessage,
) {
	var (
		selectedSkill  *models.SkillDefinition
		matchedResults []models.RetrievalResult

		llmStart = time.Now()
	)

	start := time.Now()
	utilities.LogStart("ChatHandler", "processUserChatStream")

	var userData models.UserChatData
	if err := json.Unmarshal(incomingMessage.Data, &userData); err != nil {
		handler.sendError(connectionID, "parse_error", err.Error())
		return
	}

	if userData.Message == "" {
		handler.sendError(connectionID, "validation_error", "用户消息内容为空")
		return
	}

	// 拦截 #create_skill 命令，由独立的处理器完成创建流程
	if handler.tryHandleCreateSkillCommand(ctx, connectionID, incomingMessage, userData) {
		return
	}

	skillSelectionStart := time.Now()

	availableSkillIDs := handler.registry.ListSkillIDs()

	if userData.SelectedSkill != "" {
		if skill, exists := handler.registry.GetSkill(userData.SelectedSkill); exists {
			selectedSkill = &skill
			matchedResults = []models.RetrievalResult{
				{Meta: skill.Metadata(), Score: 1.0},
			}
		}
	} else if incomingMessage.SkillsId != "" {
		if skill, exists := handler.registry.GetSkill(incomingMessage.SkillsId); exists {
			selectedSkill = &skill
			matchedResults = []models.RetrievalResult{
				{Meta: skill.Metadata(), Score: 1.0},
			}
		}
	} else {
		matchedResults = handler.registry.RetrieveSkills(userData.Message)
		if len(matchedResults) > 0 {
			if skill, exists := handler.registry.GetSkill(matchedResults[0].Meta.SkillIdentifier); exists {
				selectedSkill = &skill
			}
		}
	}

	skillMatches := make([]models.SkillMatch, 0, len(matchedResults))
	for _, r := range matchedResults {
		skillMatches = append(skillMatches, models.SkillMatch{
			SkillIdentifier:  r.Meta.SkillIdentifier,
			SkillDisplayName: r.Meta.SkillDisplayName,
			MatchScore:       r.Score,
			SkillCategory:    r.Meta.SkillCategory,
		})
	}

	var selectedSkillMatch *models.SkillMatch
	if selectedSkill != nil {
		selectedSkillMatch = &models.SkillMatch{
			SkillIdentifier:  selectedSkill.SkillIdentifier,
			SkillDisplayName: selectedSkill.SkillDisplayName,
			MatchScore:       1.0,
			SkillCategory:    selectedSkill.SkillCategory,
		}
	}

	thinkingMsg := models.SystemThinkingData{
		Phase:         "skill_selection",
		SkillMatches:  skillMatches,
		SelectedSkill: selectedSkillMatch,
		Thought:       fmt.Sprintf("已完成技能匹配，找到 %d 个候选技能", len(matchedResults)),
		Metadata: map[string]interface{}{
			"latency_ms": time.Since(skillSelectionStart).Milliseconds(),
			"candidates": len(matchedResults),
			"available":  len(availableSkillIDs),
		},
	}
	handler.sendThinking(connectionID, thinkingMsg, incomingMessage.SkillsId)

	thinkingMsg2 := models.SystemThinkingData{
		Phase:         "llm_generation",
		SkillMatches:  skillMatches,
		SelectedSkill: selectedSkillMatch,
		Thought:       "正在生成响应，请稍候...",
		Metadata: map[string]interface{}{
			"started_at": llmStart.Format(time.RFC3339),
		},
	}
	handler.sendThinking(connectionID, thinkingMsg2, incomingMessage.SkillsId)

	var execResult ExecutionResult
	var execErr error

	if selectedSkill != nil {
		execResult, execErr = handler.executor.ExecuteWithSkill(ctx, userData, selectedSkill.SkillIdentifier)
	} else {
		execResult, execErr = handler.executor.Execute(ctx, userData)
	}

	llmLatency := time.Since(llmStart)

	if execErr != nil {
		handler.sendError(connectionID, "generation_error", execErr.Error())
		utilities.LogError("ChatHandler", "processUserChatStream", execErr, time.Since(start))
		return
	}

	finishReason := execResult.Response.FinishReason
	tokensUsed := execResult.Response.TokensUsed
	modelName := ""

	responseData := models.SystemResponseData{
		Content:      execResult.Response.Content,
		SkillUsed:    selectedSkillMatch,
		FinishReason: finishReason,
		TokensUsed:   tokensUsed,
		LatencyMs:    llmLatency.Milliseconds(),
		Metadata: map[string]interface{}{
			"model": modelName,
		},
	}
	handler.sendResponse(connectionID, responseData, incomingMessage.SkillsId)

	utilities.LogSuccess("ChatHandler", "processUserChatStream", time.Since(start),
		fmt.Sprintf("conn=%s", connectionID),
		fmt.Sprintf("skill=%s", func() string {
			if selectedSkill != nil {
				return selectedSkill.SkillIdentifier
			}
			return "none"
		}()),
	)
}

func (handler *ChatHandler) sendThinking(
	connectionID string,
	data models.SystemThinkingData,
	skillID string,
) {
	if handler.messageSender == nil {
		return
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		utilities.LogError("ChatHandler", "sendThinking", err, 0)
		return
	}

	msg := models.WSMessage{
		Type:      models.SystemThinking,
		Data:      json.RawMessage(dataBytes),
		SkillsId:  skillID,
		Timestamp: time.Now(),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		utilities.LogError("ChatHandler", "sendThinking", err, 0)
		return
	}

	if err := handler.messageSender.SendMessage(connectionID, payload); err != nil {
		utilities.LogError("ChatHandler", "sendThinking", err, 0)
	}
}

func (handler *ChatHandler) sendResponse(
	connectionID string,
	data models.SystemResponseData,
	skillID string,
) {
	if handler.messageSender == nil {
		return
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		utilities.LogError("ChatHandler", "sendResponse", err, 0)
		return
	}

	msg := models.WSMessage{
		Type:      models.SystemResponse,
		Data:      json.RawMessage(dataBytes),
		SkillsId:  skillID,
		Timestamp: time.Now(),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		utilities.LogError("ChatHandler", "sendResponse", err, 0)
		return
	}

	if err := handler.messageSender.SendMessage(connectionID, payload); err != nil {
		utilities.LogError("ChatHandler", "sendResponse", err, 0)
	}
}

func (handler *ChatHandler) sendError(
	connectionID string,
	event string,
	errorMessage string,
) {
	if handler.messageSender == nil {
		return
	}

	errorData := models.SystemChatData{
		Event:   event,
		Message: errorMessage,
	}

	dataBytes, _ := json.Marshal(errorData)

	msg := models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}

	payload, _ := json.Marshal(msg)
	_ = handler.messageSender.SendMessage(connectionID, payload)
}

// GetRegistry 返回底层的技能注册中心，供外部注册使用。
func (handler *ChatHandler) GetRegistry() *SkillRegistry {
	return handler.registry
}

// GetExecutor 返回底层的技能执行器。
func (handler *ChatHandler) GetExecutor() *SkillExecutor {
	return handler.executor
}

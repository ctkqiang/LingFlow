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
	traceID := utilities.NewTraceID()
	utilities.LogStart("ChatHandler", "HandleIncomingMessage")
	utilities.LogVerbose("ChatHandler", "HandleIncomingMessage", "收到原始载荷",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("payload_size_bytes=%d", len(rawPayload)),
	)

	// 解析消息
	parseStart := time.Now()
	incomingMessage, err := handler.parseIncomingMessage(rawPayload)
	if err != nil {
		utilities.LogError("ChatHandler", "HandleIncomingMessage", err, time.Since(parseStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=parse")
		errorResponse := handler.buildErrorResponse("parse_error", err.Error())
		return json.Marshal(errorResponse)
	}
	utilities.LogNano("ChatHandler", "HandleIncomingMessage", utilities.INFO, "PARSE_OK",
		time.Since(parseStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("type=%s", incomingMessage.Type),
		fmt.Sprintf("skill_id=%s", incomingMessage.SkillsId),
	)

	// 校验消息
	validateStart := time.Now()
	if err := ValidateWSMessage(incomingMessage); err != nil {
		utilities.LogError("ChatHandler", "HandleIncomingMessage", err, time.Since(validateStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=validate")
		errorResponse := handler.buildErrorResponse("validation_error", err.Error())
		return json.Marshal(errorResponse)
	}
	utilities.LogNano("ChatHandler", "HandleIncomingMessage", utilities.INFO, "VALIDATE_OK",
		time.Since(validateStart), fmt.Sprintf("trace_id=%s", traceID))

	// 路由消息
	routeStart := time.Now()
	var responseMessage models.WSMessage

	utilities.LogVerbose("ChatHandler", "HandleIncomingMessage", "开始消息类型路由",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("message_type=%s", incomingMessage.Type),
	)

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

	utilities.LogNano("ChatHandler", "HandleIncomingMessage", utilities.INFO, "ROUTE_DONE",
		time.Since(routeStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("routed_type=%s", incomingMessage.Type),
		fmt.Sprintf("has_error=%v", err != nil),
	)

	if err != nil {
		utilities.LogError("ChatHandler", "HandleIncomingMessage", err, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID))
		errorResponse := handler.buildErrorResponse("processing_error", err.Error())
		return json.Marshal(errorResponse)
	}

	responseBytes, err := json.Marshal(responseMessage)
	if err != nil {
		utilities.LogError("ChatHandler", "HandleIncomingMessage",
			fmt.Errorf("序列化响应消息失败: %w", err), time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID))
		return nil, fmt.Errorf("序列化响应消息失败: %w", err)
	}

	totalNs := time.Since(start).Nanoseconds()
	utilities.LogSuccess("ChatHandler", "HandleIncomingMessage", time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("type=%s", incomingMessage.Type),
		fmt.Sprintf("skill=%s", incomingMessage.SkillsId),
		fmt.Sprintf("response_size_bytes=%d", len(responseBytes)),
		fmt.Sprintf("total_ns=%d", totalNs),
	)

	return responseBytes, nil
}

// handleUserChat 通过 LLM 管道处理 user_chat 类型的消息。
func (handler *ChatHandler) handleUserChat(
	ctx context.Context,
	message models.WSMessage,
) (models.WSMessage, error) {
	chatStart := time.Now()
	utilities.LogStart("ChatHandler", "handleUserChat")

	// 解析用户聊天数据
	var userData models.UserChatData
	if err := json.Unmarshal(message.Data, &userData); err != nil {
		utilities.LogError("ChatHandler", "handleUserChat", err, time.Since(chatStart), "phase=parse_user_data")
		return models.WSMessage{}, fmt.Errorf("解析用户聊天数据失败: %w", err)
	}

	utilities.LogVerbose("ChatHandler", "handleUserChat", "已提取用户数据",
		fmt.Sprintf("message_len=%d", len(userData.Message)),
		fmt.Sprintf("selected_skill=%s", userData.SelectedSkill),
		fmt.Sprintf("skill_id_from_msg=%s", message.SkillsId),
	)

	if userData.Message == "" {
		utilities.LogWarn("ChatHandler", "handleUserChat", "用户消息内容为空", time.Since(chatStart))
		return models.WSMessage{}, fmt.Errorf("用户消息内容为空")
	}

	// 通过技能管道执行
	var result ExecutionResult
	var err error

	execStart := time.Now()
	if message.SkillsId != "" {
		// 客户端显式指定了技能
		utilities.LogVerbose("ChatHandler", "handleUserChat", "技能选择路径: 客户端显式指定",
			fmt.Sprintf("skill_id=%s", message.SkillsId))
		result, err = handler.executor.ExecuteWithSkill(ctx, userData, message.SkillsId)
	} else {
		// 从用户消息自动检测技能
		utilities.LogVerbose("ChatHandler", "handleUserChat", "技能选择路径: 自动检测")
		result, err = handler.executor.Execute(ctx, userData)
	}

	if err != nil {
		utilities.LogError("ChatHandler", "handleUserChat", err, time.Since(execStart), "phase=execution")
		// 若尚未包装为 SkillExecutionError 则进行包装
		if _, ok := err.(*SkillExecutionError); !ok {
			err = NewSkillExecutionError(message.SkillsId, "execute", err)
		}
		return models.WSMessage{}, err
	}

	utilities.LogSuccess("ChatHandler", "handleUserChat", time.Since(chatStart),
		fmt.Sprintf("execution_ns=%d", time.Since(execStart).Nanoseconds()),
		fmt.Sprintf("skill_used=%v", result.SkillUsed != nil),
	)

	return result.WSMessage, nil
}

// handleSystemChat 处理 system_chat 类型的消息（透传或确认应答）。
func (handler *ChatHandler) handleSystemChat(
	message models.WSMessage,
) (models.WSMessage, error) {
	sysStart := time.Now()
	utilities.LogStart("ChatHandler", "handleSystemChat")

	var sysData models.SystemChatData
	if err := json.Unmarshal(message.Data, &sysData); err != nil {
		utilities.LogError("ChatHandler", "handleSystemChat", err, time.Since(sysStart), "phase=parse")
		return models.WSMessage{}, fmt.Errorf("解析系统聊天数据失败: %w", err)
	}

	utilities.LogVerbose("ChatHandler", "handleSystemChat", "收到系统事件",
		fmt.Sprintf("event_type=%s", sysData.Event),
		fmt.Sprintf("message_len=%d", len(sysData.Message)),
	)

	acknowledgementData := models.SystemChatData{
		Event:   "system_ack",
		Message: fmt.Sprintf("已确认系统事件: %s", sysData.Event),
	}

	dataBytes, err := json.Marshal(acknowledgementData)
	if err != nil {
		utilities.LogError("ChatHandler", "handleSystemChat", err, time.Since(sysStart), "phase=serialize_ack")
		return models.WSMessage{}, fmt.Errorf("序列化确认应答数据失败: %w", err)
	}

	utilities.LogNano("ChatHandler", "handleSystemChat", utilities.INFO, "ACK_CONSTRUCTED",
		time.Since(sysStart),
		fmt.Sprintf("original_event=%s", sysData.Event),
		fmt.Sprintf("ack_size_bytes=%d", len(dataBytes)),
	)

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
	hbStart := time.Now()
	utilities.LogStart("ChatHandler", "handleHeartbeat")

	var heartbeatData models.HeartbeatChatData
	if err := json.Unmarshal(message.Data, &heartbeatData); err != nil {
		utilities.LogError("ChatHandler", "handleHeartbeat", err, time.Since(hbStart), "phase=parse")
		return models.WSMessage{}, fmt.Errorf("解析心跳数据失败: %w", err)
	}

	utilities.LogVerbose("ChatHandler", "handleHeartbeat", "收到心跳",
		fmt.Sprintf("action=%s", heartbeatData.Action),
		fmt.Sprintf("nonce=%s", heartbeatData.Nonce),
		fmt.Sprintf("client_timestamp=%s", heartbeatData.Timestamp.Format(time.RFC3339Nano)),
	)

	if heartbeatData.Action != "ping" {
		utilities.LogWarn("ChatHandler", "handleHeartbeat",
			fmt.Sprintf("不支持的心跳动作: %s", heartbeatData.Action), time.Since(hbStart))
		return models.WSMessage{}, fmt.Errorf("heartbeat_chat 只支持 ping 动作")
	}

	latency := time.Since(heartbeatData.Timestamp).Milliseconds()
	pongResponseData := models.HeartbeatChatData{
		Action:    "pong",
		Nonce:     heartbeatData.Nonce,
		Timestamp: time.Now(),
		Latency:   latency,
	}

	utilities.LogNano("ChatHandler", "handleHeartbeat", utilities.INFO, "PONG_CONSTRUCTED",
		time.Since(hbStart),
		fmt.Sprintf("nonce=%s", heartbeatData.Nonce),
		fmt.Sprintf("latency_ms=%d", latency),
		fmt.Sprintf("processing_ns=%d", time.Since(hbStart).Nanoseconds()),
	)

	dataBytes, err := json.Marshal(pongResponseData)
	if err != nil {
		utilities.LogError("ChatHandler", "handleHeartbeat", err, time.Since(hbStart), "phase=serialize_pong")
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
	connStart := time.Now()
	utilities.LogStart("ChatHandler", "OnConnectionReady")
	utilities.LogVerbose("ChatHandler", "OnConnectionReady", "连接就绪，准备推送技能列表",
		fmt.Sprintf("connection_id=%s", connectionID),
	)

	if handler.messageSender == nil {
		utilities.LogWarn("ChatHandler", "OnConnectionReady", "messageSender 为 nil，跳过技能列表推送",
			time.Since(connStart), fmt.Sprintf("connection_id=%s", connectionID))
		return
	}

	skills := handler.registry.ListSkills()
	skillItems := make([]models.SkillListItem, 0, len(skills))
	skillIDs := make([]string, 0, len(skills))
	for _, s := range skills {
		skillItems = append(skillItems, models.SkillListItem{
			SkillIdentifier:  s.SkillIdentifier,
			SkillDisplayName: s.SkillDisplayName,
			SkillDescription: s.SkillDescription,
			SkillCategory:    s.SkillCategory,
			SearchKeywords:   s.SearchKeywords,
		})
		skillIDs = append(skillIDs, s.SkillIdentifier)
	}

	source := "local"
	if handler.s3Loader != nil {
		source = "s3"
	}

	utilities.LogVerbose("ChatHandler", "OnConnectionReady", "技能列表构建完成",
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("skill_count=%d", len(skillItems)),
		fmt.Sprintf("source=%s", source),
		fmt.Sprintf("skill_ids=%v", skillIDs),
	)

	listData := models.SystemSkillsListData{
		Skills:    skillItems,
		Total:     len(skillItems),
		Source:    source,
		UpdatedAt: time.Now(),
	}

	dataBytes, err := json.Marshal(listData)
	if err != nil {
		utilities.LogError("ChatHandler", "OnConnectionReady", err, time.Since(connStart),
			fmt.Sprintf("connection_id=%s", connectionID), "phase=serialize_list")
		return
	}

	listMessage := models.WSMessage{
		Type:      models.SystemSkillsList,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}

	payload, err := json.Marshal(listMessage)
	if err != nil {
		utilities.LogError("ChatHandler", "OnConnectionReady", err, time.Since(connStart),
			fmt.Sprintf("connection_id=%s", connectionID), "phase=serialize_message")
		return
	}

	utilities.LogVerbose("ChatHandler", "OnConnectionReady", "准备发送技能列表消息",
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("payload_size_bytes=%d", len(payload)),
	)

	if sendErr := handler.messageSender.SendMessage(connectionID, payload); sendErr != nil {
		utilities.LogWarn("ChatHandler", "OnConnectionReady",
			fmt.Sprintf("发送技能列表失败: %v", sendErr), time.Since(connStart),
			fmt.Sprintf("connection_id=%s", connectionID))
	} else {
		utilities.LogSuccess("ChatHandler", "OnConnectionReady", time.Since(connStart),
			fmt.Sprintf("connection_id=%s", connectionID),
			fmt.Sprintf("skill_count=%d", len(skillItems)),
			fmt.Sprintf("payload_size_bytes=%d", len(payload)),
		)
	}
}

// HandleUserChatWithStreaming 是 ChatStreamer 接口的实现。
// 检查消息是否为 user_chat，是则异步启动流式处理并返回 true。
func (handler *ChatHandler) HandleUserChatWithStreaming(
	ctx context.Context,
	connectionID string,
	messagePayload []byte,
) bool {
	streamStart := time.Now()
	utilities.LogStart("ChatHandler", "HandleUserChatWithStreaming")
	utilities.LogVerbose("ChatHandler", "HandleUserChatWithStreaming", "收到流式处理请求",
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("payload_size_bytes=%d", len(messagePayload)),
	)

	if handler.messageSender == nil {
		utilities.LogWarn("ChatHandler", "HandleUserChatWithStreaming",
			"messageSender 为 nil，无法处理流式请求", time.Since(streamStart))
		return false
	}

	var incomingMessage models.WSMessage
	if err := json.Unmarshal(messagePayload, &incomingMessage); err != nil {
		utilities.LogError("ChatHandler", "HandleUserChatWithStreaming", err, time.Since(streamStart),
			fmt.Sprintf("connection_id=%s", connectionID), "phase=parse")
		return false
	}

	utilities.LogVerbose("ChatHandler", "HandleUserChatWithStreaming", "消息解析完成",
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("type=%s", incomingMessage.Type),
	)

	if incomingMessage.Type != models.UserChat {
		utilities.LogVerbose("ChatHandler", "HandleUserChatWithStreaming", "非 user_chat 类型，跳过流式处理",
			fmt.Sprintf("connection_id=%s", connectionID),
			fmt.Sprintf("actual_type=%s", incomingMessage.Type),
		)
		return false
	}

	if err := ValidateWSMessage(incomingMessage); err != nil {
		utilities.LogError("ChatHandler", "HandleUserChatWithStreaming", err, time.Since(streamStart),
			fmt.Sprintf("connection_id=%s", connectionID), "phase=validate")
		handler.sendError(connectionID, "validation_error", err.Error())
		return true
	}

	utilities.LogNano("ChatHandler", "HandleUserChatWithStreaming", utilities.INFO, "GOROUTINE_LAUNCH",
		time.Since(streamStart),
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("skill_id=%s", incomingMessage.SkillsId),
	)

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
	traceID := utilities.NewTraceID()
	utilities.LogStart("ChatHandler", "processUserChatStream")
	utilities.LogVerbose("ChatHandler", "processUserChatStream", "流式处理管道启动",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("skill_id=%s", incomingMessage.SkillsId),
	)

	// ── 步骤1: 解析用户数据 ──
	parseStart := time.Now()
	var userData models.UserChatData
	if err := json.Unmarshal(incomingMessage.Data, &userData); err != nil {
		utilities.LogError("ChatHandler", "processUserChatStream", err, time.Since(parseStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=parse")
		handler.sendError(connectionID, "parse_error", err.Error())
		return
	}
	utilities.LogNano("ChatHandler", "processUserChatStream", utilities.INFO, "PARSE_OK",
		time.Since(parseStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("message_len=%d", len(userData.Message)),
		fmt.Sprintf("selected_skill=%s", userData.SelectedSkill),
	)

	if userData.Message == "" {
		utilities.LogWarn("ChatHandler", "processUserChatStream", "用户消息内容为空",
			time.Since(start), fmt.Sprintf("trace_id=%s", traceID))
		handler.sendError(connectionID, "validation_error", "用户消息内容为空")
		return
	}

	// 拦截 #create_skill 命令，由独立的处理器完成创建流程
	if handler.tryHandleCreateSkillCommand(ctx, connectionID, incomingMessage, userData) {
		utilities.LogVerbose("ChatHandler", "processUserChatStream", "已拦截 #create_skill 命令",
			fmt.Sprintf("trace_id=%s", traceID))
		return
	}

	// ── 步骤2: 技能选择 ──
	skillSelectionStart := time.Now()

	availableSkillIDs := handler.registry.ListSkillIDs()
	utilities.LogVerbose("ChatHandler", "processUserChatStream", "开始技能选择",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("available_skills=%d", len(availableSkillIDs)),
		fmt.Sprintf("available_ids=%v", availableSkillIDs),
	)

	if userData.SelectedSkill != "" {
		utilities.LogVerbose("ChatHandler", "processUserChatStream", "技能选择路径: 用户显式选择",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("selected_skill=%s", userData.SelectedSkill),
		)
		if skill, exists := handler.registry.GetSkill(userData.SelectedSkill); exists {
			selectedSkill = &skill
			matchedResults = []models.RetrievalResult{
				{Meta: skill.Metadata(), Score: 1.0},
			}
		}
	} else if incomingMessage.SkillsId != "" {
		utilities.LogVerbose("ChatHandler", "processUserChatStream", "技能选择路径: 消息头指定",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("skill_id=%s", incomingMessage.SkillsId),
		)
		if skill, exists := handler.registry.GetSkill(incomingMessage.SkillsId); exists {
			selectedSkill = &skill
			matchedResults = []models.RetrievalResult{
				{Meta: skill.Metadata(), Score: 1.0},
			}
		}
	} else {
		utilities.LogVerbose("ChatHandler", "processUserChatStream", "技能选择路径: 自动检索",
			fmt.Sprintf("trace_id=%s", traceID))
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

	// 记录所有候选技能及其分数
	for i, m := range skillMatches {
		utilities.LogVerbose("ChatHandler", "processUserChatStream",
			fmt.Sprintf("候选技能 #%d", i+1),
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("skill_id=%s", m.SkillIdentifier),
			fmt.Sprintf("display_name=%s", m.SkillDisplayName),
			fmt.Sprintf("score=%.4f", m.MatchScore),
			fmt.Sprintf("category=%s", m.SkillCategory),
		)
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

	skillSelectionNs := time.Since(skillSelectionStart).Nanoseconds()
	utilities.LogNano("ChatHandler", "processUserChatStream", utilities.INFO, "SKILL_SELECTION_DONE",
		time.Since(skillSelectionStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("candidates=%d", len(matchedResults)),
		fmt.Sprintf("selected=%v", selectedSkill != nil),
		fmt.Sprintf("selection_ns=%d", skillSelectionNs),
	)

	// ── 步骤3: 发送 thinking 消息 ──
	thinkingSendStart := time.Now()
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
	utilities.LogNano("ChatHandler", "processUserChatStream", utilities.INFO, "THINKING_1_SENT",
		time.Since(thinkingSendStart), fmt.Sprintf("trace_id=%s", traceID))

	thinkingSendStart = time.Now()
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
	utilities.LogNano("ChatHandler", "processUserChatStream", utilities.INFO, "THINKING_2_SENT",
		time.Since(thinkingSendStart), fmt.Sprintf("trace_id=%s", traceID))

	// ── 步骤4: LLM 执行 ──
	llmExecStart := time.Now()
	utilities.LogVerbose("ChatHandler", "processUserChatStream", "开始 LLM 执行",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("has_skill=%v", selectedSkill != nil),
	)

	var execResult ExecutionResult
	var execErr error

	if selectedSkill != nil {
		execResult, execErr = handler.executor.ExecuteWithSkill(ctx, userData, selectedSkill.SkillIdentifier)
	} else {
		execResult, execErr = handler.executor.Execute(ctx, userData)
	}

	llmLatency := time.Since(llmStart)
	llmExecNs := time.Since(llmExecStart).Nanoseconds()
	utilities.LogNano("ChatHandler", "processUserChatStream", utilities.INFO, "LLM_EXEC_DONE",
		time.Since(llmExecStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("exec_ns=%d", llmExecNs),
		fmt.Sprintf("has_error=%v", execErr != nil),
	)

	if execErr != nil {
		handler.sendError(connectionID, "generation_error", execErr.Error())
		utilities.LogError("ChatHandler", "processUserChatStream", execErr, time.Since(start),
			fmt.Sprintf("trace_id=%s", traceID), fmt.Sprintf("connection_id=%s", connectionID))
		return
	}

	// ── 步骤5: 构建并发送响应 ──
	responseStart := time.Now()
	finishReason := execResult.Response.FinishReason
	tokensUsed := execResult.Response.TokensUsed
	modelName := ""

	utilities.LogVerbose("ChatHandler", "processUserChatStream", "构建响应消息",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("finish_reason=%s", finishReason),
		fmt.Sprintf("tokens_used=%d", tokensUsed),
		fmt.Sprintf("content_len=%d", len(execResult.Response.Content)),
	)

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
	utilities.LogNano("ChatHandler", "processUserChatStream", utilities.INFO, "RESPONSE_SENT",
		time.Since(responseStart), fmt.Sprintf("trace_id=%s", traceID))

	// ── 管道完成 ──
	totalNs := time.Since(start).Nanoseconds()
	utilities.LogSuccess("ChatHandler", "processUserChatStream", time.Since(start),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("conn=%s", connectionID),
		fmt.Sprintf("total_pipeline_ns=%d", totalNs),
		fmt.Sprintf("skill=%s", func() string {
			if selectedSkill != nil {
				return selectedSkill.SkillIdentifier
			}
			return "none"
		}()),
		fmt.Sprintf("tokens=%d", tokensUsed),
		fmt.Sprintf("llm_latency=%s", llmLatency),
	)
}

func (handler *ChatHandler) sendThinking(
	connectionID string,
	data models.SystemThinkingData,
	skillID string,
) {
	sendStart := time.Now()
	if handler.messageSender == nil {
		utilities.LogWarn("ChatHandler", "sendThinking", "messageSender 为 nil", 0)
		return
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		utilities.LogError("ChatHandler", "sendThinking", err, time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID))
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
		utilities.LogError("ChatHandler", "sendThinking", err, time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID))
		return
	}

	utilities.LogVerbose("ChatHandler", "sendThinking", "发送 thinking 消息",
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("phase=%s", data.Phase),
		fmt.Sprintf("payload_size_bytes=%d", len(payload)),
	)

	if err := handler.messageSender.SendMessage(connectionID, payload); err != nil {
		utilities.LogError("ChatHandler", "sendThinking", err, time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID))
	} else {
		utilities.LogNano("ChatHandler", "sendThinking", utilities.INFO, "SENT_OK",
			time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID),
			fmt.Sprintf("payload_size_bytes=%d", len(payload)),
		)
	}
}

func (handler *ChatHandler) sendResponse(
	connectionID string,
	data models.SystemResponseData,
	skillID string,
) {
	sendStart := time.Now()
	if handler.messageSender == nil {
		utilities.LogWarn("ChatHandler", "sendResponse", "messageSender 为 nil", 0)
		return
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		utilities.LogError("ChatHandler", "sendResponse", err, time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID))
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
		utilities.LogError("ChatHandler", "sendResponse", err, time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID))
		return
	}

	utilities.LogVerbose("ChatHandler", "sendResponse", "发送响应消息",
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("content_len=%d", len(data.Content)),
		fmt.Sprintf("payload_size_bytes=%d", len(payload)),
	)

	if err := handler.messageSender.SendMessage(connectionID, payload); err != nil {
		utilities.LogError("ChatHandler", "sendResponse", err, time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID))
	} else {
		utilities.LogNano("ChatHandler", "sendResponse", utilities.INFO, "SENT_OK",
			time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID),
			fmt.Sprintf("payload_size_bytes=%d", len(payload)),
		)
	}
}

func (handler *ChatHandler) sendError(
	connectionID string,
	event string,
	errorMessage string,
) {
	sendStart := time.Now()
	utilities.LogVerbose("ChatHandler", "sendError", "发送错误消息",
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("event=%s", event),
		fmt.Sprintf("error_message_len=%d", len(errorMessage)),
	)

	if handler.messageSender == nil {
		utilities.LogWarn("ChatHandler", "sendError", "messageSender 为 nil，无法发送错误", 0,
			fmt.Sprintf("connection_id=%s", connectionID))
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

	if sendErr := handler.messageSender.SendMessage(connectionID, payload); sendErr != nil {
		utilities.LogError("ChatHandler", "sendError", sendErr, time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID), fmt.Sprintf("event=%s", event))
	} else {
		utilities.LogNano("ChatHandler", "sendError", utilities.INFO, "ERROR_SENT",
			time.Since(sendStart),
			fmt.Sprintf("connection_id=%s", connectionID),
			fmt.Sprintf("event=%s", event),
			fmt.Sprintf("payload_size_bytes=%d", len(payload)),
		)
	}
}

// GetRegistry 返回底层的技能注册中心，供外部注册使用。
func (handler *ChatHandler) GetRegistry() *SkillRegistry {
	return handler.registry
}

// GetExecutor 返回底层的技能执行器。
func (handler *ChatHandler) GetExecutor() *SkillExecutor {
	return handler.executor
}

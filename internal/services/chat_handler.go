package services

import (
	"context"
	"encoding/json"
	"fmt"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"time"
)

// ChatHandler 通过技能增强的 LLM 管道处理传入的 WebSocket 消息，并生成响应消息。
type ChatHandler struct {
	executor *SkillExecutor
	registry *SkillRegistry
}

// NewChatHandler 创建一个完整注入依赖的 ChatHandler 实例。
func NewChatHandler(registry *SkillRegistry, llmService LLMService) *ChatHandler {
	return &ChatHandler{
		executor: NewSkillExecutor(registry, llmService),
		registry: registry,
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

// GetRegistry 返回底层的技能注册中心，供外部注册使用。
func (handler *ChatHandler) GetRegistry() *SkillRegistry {
	return handler.registry
}

// GetExecutor 返回底层的技能执行器。
func (handler *ChatHandler) GetExecutor() *SkillExecutor {
	return handler.executor
}

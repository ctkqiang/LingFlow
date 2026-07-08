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

// NewChatHandler creates a fully wired ChatHandler with all dependencies.
func NewChatHandler(registry *SkillRegistry, llmService LLMService) *ChatHandler {
	return &ChatHandler{
		executor: NewSkillExecutor(registry, llmService),
		registry: registry,
	}
}

// NewDefaultChatHandler creates a ChatHandler using environment-based Bedrock config.
// It initializes the AWS Bedrock client via the default credential chain.
func NewDefaultChatHandler(ctx context.Context) (*ChatHandler, error) {
	registry := NewSkillRegistry()
	bedrockConfig := NewBedrockConfig()
	llmService, err := NewBedrockLLMService(ctx, bedrockConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Bedrock LLM service: %w", err)
	}
	return NewChatHandler(registry, llmService), nil
}

// HandleIncomingMessage processes a raw WebSocket message payload.
// It parses the WSMessage, routes based on type, and returns a response WSMessage.
func (handler *ChatHandler) HandleIncomingMessage(
	ctx context.Context,
	rawPayload []byte,
) ([]byte, error) {
	start := time.Now()
	utilities.LogStart("ChatHandler", "HandleIncomingMessage")

	// Step 1: Parse the incoming WSMessage
	incomingMessage, err := handler.parseIncomingMessage(rawPayload)
	if err != nil {
		errorResponse := handler.buildErrorResponse("parse_error", err.Error())
		return json.Marshal(errorResponse)
	}

	// Step 2: Validate the incoming message structure
	if err := ValidateWSMessage(incomingMessage); err != nil {
		errorResponse := handler.buildErrorResponse("validation_error", err.Error())
		return json.Marshal(errorResponse)
	}

	// Step 3: Route based on message type
	var responseMessage models.WSMessage

	switch incomingMessage.Type {
	case models.UserChat:
		responseMessage, err = handler.handleUserChat(ctx, incomingMessage)
	case models.SystemChat:
		responseMessage, err = handler.handleSystemChat(incomingMessage)
	default:
		err = fmt.Errorf("unsupported message type: %s", incomingMessage.Type)
	}

	if err != nil {
		utilities.LogError("ChatHandler", "HandleIncomingMessage", err, time.Since(start))
		errorResponse := handler.buildErrorResponse("processing_error", err.Error())
		return json.Marshal(errorResponse)
	}

	// Step 4: Marshal the response
	responseBytes, err := json.Marshal(responseMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response message: %w", err)
	}

	utilities.LogSuccess("ChatHandler", "HandleIncomingMessage", time.Since(start),
		fmt.Sprintf("type=%s", incomingMessage.Type),
		fmt.Sprintf("skill=%s", incomingMessage.SkillsId),
	)

	return responseBytes, nil
}

// handleUserChat processes a user_chat message through the LLM pipeline.
func (handler *ChatHandler) handleUserChat(
	ctx context.Context,
	message models.WSMessage,
) (models.WSMessage, error) {
	// Parse user chat data
	var userData models.UserChatData
	if err := json.Unmarshal(message.Data, &userData); err != nil {
		return models.WSMessage{}, fmt.Errorf("failed to parse user chat data: %w", err)
	}

	if userData.Message == "" {
		return models.WSMessage{}, fmt.Errorf("user message content is empty")
	}

	// Execute through the skill pipeline
	var result ExecutionResult
	var err error

	if message.SkillsId != "" {
		// Explicit skill specified by the client
		result, err = handler.executor.ExecuteWithSkill(ctx, userData, message.SkillsId)
	} else {
		// Auto-detect skill from user message
		result, err = handler.executor.Execute(ctx, userData)
	}

	if err != nil {
		// Wrap as SkillExecutionError if not already
		if _, ok := err.(*SkillExecutionError); !ok {
			err = NewSkillExecutionError(message.SkillsId, "execute", err)
		}
		return models.WSMessage{}, err
	}

	return result.WSMessage, nil
}

// handleSystemChat processes system_chat messages (pass-through or acknowledgement).
func (handler *ChatHandler) handleSystemChat(
	message models.WSMessage,
) (models.WSMessage, error) {
	var sysData models.SystemChatData
	if err := json.Unmarshal(message.Data, &sysData); err != nil {
		return models.WSMessage{}, fmt.Errorf("failed to parse system chat data: %w", err)
	}

	ackData := models.SystemChatData{
		Event:   "system_ack",
		Message: fmt.Sprintf("Acknowledged system event: %s", sysData.Event),
	}

	dataBytes, err := json.Marshal(ackData)
	if err != nil {
		return models.WSMessage{}, fmt.Errorf("failed to marshal ack data: %w", err)
	}

	return models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		SkillsId:  message.SkillsId,
		Timestamp: time.Now(),
	}, nil
}

// parseIncomingMessage deserializes a raw payload into a WSMessage.
func (handler *ChatHandler) parseIncomingMessage(rawPayload []byte) (models.WSMessage, error) {
	var message models.WSMessage
	if err := json.Unmarshal(rawPayload, &message); err != nil {
		return models.WSMessage{}, fmt.Errorf("invalid message format: %w", err)
	}
	return message, nil
}

// buildErrorResponse creates a standardized error WSMessage.
func (handler *ChatHandler) buildErrorResponse(event, errorMessage string) models.WSMessage {
	errData := models.SystemChatData{
		Event:   event,
		Message: errorMessage,
	}

	dataBytes, _ := json.Marshal(errData)

	return models.WSMessage{
		Type:      models.SystemChat,
		Data:      json.RawMessage(dataBytes),
		Timestamp: time.Now(),
	}
}

// GetRegistry returns the underlying skill registry for external registration.
func (handler *ChatHandler) GetRegistry() *SkillRegistry {
	return handler.registry
}

// GetExecutor returns the underlying skill executor.
func (handler *ChatHandler) GetExecutor() *SkillExecutor {
	return handler.executor
}

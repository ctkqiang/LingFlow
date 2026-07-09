package events

import (
	"context"
	"fmt"
)

// ChatCommandHandler 将命令路由到聚合根，协调事件持久化和发布。
type ChatCommandHandler struct {
	eventStore EventStore
	eventBus   EventBus
}

// NewChatCommandHandler 创建一个命令处理器。
func NewChatCommandHandler(eventStore EventStore, eventBus EventBus) *ChatCommandHandler {
	return &ChatCommandHandler{
		eventStore: eventStore,
		eventBus:   eventBus,
	}
}

// Handle 处理命令的完整流程：
// 1. 从 EventStore 加载聚合根的事件流
// 2. 重建聚合根状态
// 3. 将命令交给聚合根处理，得到待持久化事件
// 4. 逐个追加事件到 EventStore
// 5. 通过 EventBus 发布所有已持久化事件
func (handler *ChatCommandHandler) Handle(
	ctx context.Context,
	cmd Command,
) ([]DomainEvent, error) {
	streamID := ChatStreamID(cmd.AggregateID)
	eventHistory, loadError := handler.eventStore.LoadStream(ctx, streamID)
	if loadError != nil {
		return nil, fmt.Errorf("加载事件流失败 [%s]: %w", streamID, loadError)
	}

	aggregate, rebuildError := RebuildChatSessionAggregate(eventHistory)
	if rebuildError != nil {
		return nil, fmt.Errorf("重建聚合根失败 [%s]: %w", cmd.AggregateID, rebuildError)
	}

	var pendingEvents []DomainEvent
	var handleError error

	switch cmd.CommandType {
	case CommandTypeConnectSession:
		pendingEvents, handleError = aggregate.HandleConnectSession(cmd)
	case CommandTypeDisconnectSession:
		pendingEvents, handleError = aggregate.HandleDisconnectSession(cmd)
	case CommandTypeSendMessage:
		pendingEvents, handleError = aggregate.HandleSendMessage(cmd)
	default:
		return nil, fmt.Errorf("未知的命令类型: %s", cmd.CommandType)
	}

	if handleError != nil {
		return nil, fmt.Errorf("处理命令失败 [%s]: %w", cmd.CommandType, handleError)
	}

	var persistedEvents []DomainEvent
	for _, pendingEvent := range pendingEvents {
		persistedEvent, appendError := handler.eventStore.Append(ctx, pendingEvent)
		if appendError != nil {
			return nil, fmt.Errorf("追加事件失败 [%s]: %w", pendingEvent.EventType, appendError)
		}
		persistedEvents = append(persistedEvents, persistedEvent)
	}

	if handler.eventBus != nil && len(persistedEvents) > 0 {
		if publishError := handler.eventBus.Publish(ctx, persistedEvents...); publishError != nil {
			return nil, fmt.Errorf("发布事件失败: %w", publishError)
		}
	}

	return persistedEvents, nil
}

package events

import (
	"encoding/json"
	"fmt"
)

// ChatSessionAggregate 是 chat session 的聚合根，封装业务逻辑和状态。
type ChatSessionAggregate struct {
	AggregateID           string
	Connected             bool
	ReceivedMessageCount  int
	ProcessedMessageCount int
	FailedMessageCount    int
	BroadcastMessageCount int
	CurrentVersion        int64
}

// RebuildChatSessionAggregate 从事件流重建聚合根状态。
func RebuildChatSessionAggregate(eventHistory []DomainEvent) (*ChatSessionAggregate, error) {
	aggregate := &ChatSessionAggregate{}
	for _, domainEvent := range eventHistory {
		aggregate.ApplyEvent(domainEvent)
		if aggregate.AggregateID == "" {
			aggregate.AggregateID = domainEvent.AggregateID
		}
	}
	aggregate.CurrentVersion = int64(len(eventHistory))
	return aggregate, nil
}

// ApplyEvent 将领域事件应用到聚合根状态。
func (aggregate *ChatSessionAggregate) ApplyEvent(domainEvent DomainEvent) {
	switch domainEvent.EventType {
	case EventTypeChatSessionConnected:
		var eventData ChatSessionEventData
		if err := json.Unmarshal(domainEvent.Data, &eventData); err == nil {
			aggregate.AggregateID = eventData.ConnectionIdentifier
		}
		aggregate.Connected = true
	case EventTypeChatSessionDisconnected:
		aggregate.Connected = false
	case EventTypeChatMessageReceived:
		aggregate.ReceivedMessageCount++
	case EventTypeChatMessageProcessed:
		aggregate.ProcessedMessageCount++
	case EventTypeChatMessageProcessingFailed:
		aggregate.FailedMessageCount++
	case EventTypeChatMessageBroadcasted:
		aggregate.BroadcastMessageCount++
	}
}

// HandleConnectSession 处理连接会话命令。
// 业务不变量：已连接的会话不能重复连接。
func (aggregate *ChatSessionAggregate) HandleConnectSession(cmd Command) ([]DomainEvent, error) {
	if aggregate.Connected {
		return nil, fmt.Errorf("会话 [%s] 已经处于连接状态，不能重复连接", aggregate.AggregateID)
	}

	var commandData ConnectSessionCommandData
	if err := json.Unmarshal(cmd.Data, &commandData); err != nil {
		return nil, fmt.Errorf("解析连接会话命令数据失败: %w", err)
	}

	event, err := NewDomainEvent(
		ChatStreamID(commandData.ConnectionIdentifier),
		commandData.ConnectionIdentifier,
		EventTypeChatSessionConnected,
		ChatSessionEventData{
			ConnectionIdentifier: commandData.ConnectionIdentifier,
			RemoteAddress:        commandData.RemoteAddress,
		},
		cmd.Metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("构建连接会话事件失败: %w", err)
	}

	return []DomainEvent{event}, nil
}

// HandleDisconnectSession 处理断开会话命令。
// 业务不变量：未连接的会话不能断开。
func (aggregate *ChatSessionAggregate) HandleDisconnectSession(cmd Command) ([]DomainEvent, error) {
	if !aggregate.Connected {
		return nil, fmt.Errorf("会话 [%s] 未处于连接状态，无法断开", aggregate.AggregateID)
	}

	var commandData DisconnectSessionCommandData
	if err := json.Unmarshal(cmd.Data, &commandData); err != nil {
		return nil, fmt.Errorf("解析断开会话命令数据失败: %w", err)
	}

	event, err := NewDomainEvent(
		ChatStreamID(commandData.ConnectionIdentifier),
		commandData.ConnectionIdentifier,
		EventTypeChatSessionDisconnected,
		ChatSessionEventData{
			ConnectionIdentifier: commandData.ConnectionIdentifier,
		},
		cmd.Metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("构建断开会话事件失败: %w", err)
	}

	return []DomainEvent{event}, nil
}

// HandleSendMessage 处理发送消息命令。
// 业务不变量：只有已连接的会话才能发送消息。
func (aggregate *ChatSessionAggregate) HandleSendMessage(cmd Command) ([]DomainEvent, error) {
	if !aggregate.Connected {
		return nil, fmt.Errorf("会话 [%s] 未连接，无法发送消息", aggregate.AggregateID)
	}

	var commandData SendMessageCommandData
	if err := json.Unmarshal(cmd.Data, &commandData); err != nil {
		return nil, fmt.Errorf("解析发送消息命令数据失败: %w", err)
	}

	event, err := NewDomainEvent(
		ChatStreamID(commandData.ConnectionIdentifier),
		commandData.ConnectionIdentifier,
		EventTypeChatMessageReceived,
		ChatMessageEventData{
			ConnectionIdentifier: commandData.ConnectionIdentifier,
			Payload:              string(commandData.MessagePayload),
		},
		cmd.Metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("构建接收消息事件失败: %w", err)
	}

	return []DomainEvent{event}, nil
}

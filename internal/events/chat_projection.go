package events

import (
	"encoding/json"
	"fmt"
	"time"
)

// ChatSessionProjection 是从事件流重放得到的 chat session 当前状态。
type ChatSessionProjection struct {
	ConnectionIdentifier string    `json:"connection_identifier"`
	Connected            bool      `json:"connected"`
	ReceivedMessageCount int       `json:"received_message_count"`
	ProcessedMessageCount int      `json:"processed_message_count"`
	FailedMessageCount    int      `json:"failed_message_count"`
	BroadcastMessageCount int      `json:"broadcast_message_count"`
	LastEventAt           time.Time `json:"last_event_at"`
}

// NewChatSessionProjection 创建一个初始的 chat session 投影。
func NewChatSessionProjection(aggregateID string) *ChatSessionProjection {
	return &ChatSessionProjection{
		ConnectionIdentifier: aggregateID,
	}
}

// ApplyEvent 将单个事件增量应用到投影状态。
func (projection *ChatSessionProjection) ApplyEvent(domainEvent DomainEvent) error {
	if domainEvent.OccurredAt.After(projection.LastEventAt) {
		projection.LastEventAt = domainEvent.OccurredAt
	}

	switch domainEvent.EventType {
	case EventTypeChatSessionConnected:
		eventData, decodeError := decodeChatSessionEventData(domainEvent)
		if decodeError != nil {
			return decodeError
		}
		projection.ConnectionIdentifier = eventData.ConnectionIdentifier
		projection.Connected = true
	case EventTypeChatSessionDisconnected:
		eventData, decodeError := decodeChatSessionEventData(domainEvent)
		if decodeError != nil {
			return decodeError
		}
		projection.ConnectionIdentifier = eventData.ConnectionIdentifier
		projection.Connected = false
	case EventTypeChatMessageReceived:
		projection.ReceivedMessageCount++
	case EventTypeChatMessageProcessed:
		projection.ProcessedMessageCount++
	case EventTypeChatMessageProcessingFailed:
		projection.FailedMessageCount++
	case EventTypeChatMessageBroadcasted:
		projection.BroadcastMessageCount++
	}

	return nil
}

// RebuildChatSessionProjection 从事件历史重建 chat session 状态。
func RebuildChatSessionProjection(eventHistory []DomainEvent) (ChatSessionProjection, error) {
	var projection ChatSessionProjection

	for _, domainEvent := range eventHistory {
		if domainEvent.OccurredAt.After(projection.LastEventAt) {
			projection.LastEventAt = domainEvent.OccurredAt
		}

		switch domainEvent.EventType {
		case EventTypeChatSessionConnected:
			eventData, decodeError := decodeChatSessionEventData(domainEvent)
			if decodeError != nil {
				return ChatSessionProjection{}, decodeError
			}
			projection.ConnectionIdentifier = eventData.ConnectionIdentifier
			projection.Connected = true
		case EventTypeChatSessionDisconnected:
			eventData, decodeError := decodeChatSessionEventData(domainEvent)
			if decodeError != nil {
				return ChatSessionProjection{}, decodeError
			}
			projection.ConnectionIdentifier = eventData.ConnectionIdentifier
			projection.Connected = false
		case EventTypeChatMessageReceived:
			projection.ReceivedMessageCount++
		case EventTypeChatMessageProcessed:
			projection.ProcessedMessageCount++
		case EventTypeChatMessageProcessingFailed:
			projection.FailedMessageCount++
		case EventTypeChatMessageBroadcasted:
			projection.BroadcastMessageCount++
		}
	}

	return projection, nil
}

func decodeChatSessionEventData(domainEvent DomainEvent) (ChatSessionEventData, error) {
	var eventData ChatSessionEventData
	if decodeError := json.Unmarshal(domainEvent.Data, &eventData); decodeError != nil {
		return ChatSessionEventData{}, fmt.Errorf(
			"解析 chat session 事件数据失败 [%s]: %w",
			domainEvent.EventType,
			decodeError,
		)
	}
	return eventData, nil
}

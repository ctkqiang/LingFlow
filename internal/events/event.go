package events

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

var eventIdentifierSequence uint64

// EventType 表示系统中已经发生的事实类型。
type EventType string

const (
	EventTypeChatSessionConnected       EventType = "chat_session_connected"
	EventTypeChatSessionDisconnected    EventType = "chat_session_disconnected"
	EventTypeChatMessageReceived         EventType = "chat_message_received"
	EventTypeChatMessageProcessed        EventType = "chat_message_processed"
	EventTypeChatMessageProcessingFailed EventType = "chat_message_processing_failed"
	EventTypeChatMessageBroadcasted      EventType = "chat_message_broadcasted"
	EventTypeHeartbeatPingReceived      EventType = "heartbeat_ping_received"
	EventTypeHeartbeatPongSent          EventType = "heartbeat_pong_sent"
	EventTypeHeartbeatPingSent          EventType = "heartbeat_ping_sent"
	EventTypeHeartbeatPongReceived      EventType = "heartbeat_pong_received"
	EventTypeHeartbeatTimeout           EventType = "heartbeat_timeout"
	EventTypeSkillExecutionStarted   EventType = "skill_execution_started"
	EventTypeSkillExecutionCompleted EventType = "skill_execution_completed"
	EventTypeSkillExecutionFailed    EventType = "skill_execution_failed"
	EventTypeLLMGenerationStarted    EventType = "llm_generation_started"
	EventTypeLLMGenerationCompleted  EventType = "llm_generation_completed"
	EventTypeLLMGenerationFailed     EventType = "llm_generation_failed"
)

// DomainEvent 是事件溯源中唯一持久化的事实记录。
//
// StreamID 用于定位一条事件流，例如 "chat:<uuid>"。
// AggregateID 用于标识业务对象，例如 chat uuid。
// Version 是同一 StreamID 内的递增版本号。
type DomainEvent struct {
	EventID     string            `json:"event_id"`
	StreamID    string            `json:"stream_id"`
	AggregateID string            `json:"aggregate_id"`
	EventType   EventType         `json:"event_type"`
	Version     int64             `json:"version"`
	OccurredAt  time.Time         `json:"occurred_at"`
	Data        json.RawMessage   `json:"data"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// NewDomainEvent 根据事件类型和数据构建领域事件。
func NewDomainEvent(
	streamID string,
	aggregateID string,
	eventType EventType,
	eventData interface{},
	metadata map[string]string,
) (DomainEvent, error) {
	payload, marshalError := json.Marshal(eventData)
	if marshalError != nil {
		return DomainEvent{}, fmt.Errorf("序列化事件数据失败: %w", marshalError)
	}

	return DomainEvent{
		EventID:     newEventIdentifier(),
		StreamID:    streamID,
		AggregateID: aggregateID,
		EventType:   eventType,
		OccurredAt:  time.Now(),
		Data:        json.RawMessage(payload),
		Metadata:    metadata,
	}, nil
}

func newEventIdentifier() string {
	nextSequence := atomic.AddUint64(&eventIdentifierSequence, 1)
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), nextSequence)
}

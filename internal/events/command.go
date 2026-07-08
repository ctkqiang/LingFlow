package events

import (
	"encoding/json"
	"fmt"
	"time"
)

// CommandType 表示命令的类型。
type CommandType string

const (
	CommandTypeConnectSession    CommandType = "connect_session"
	CommandTypeDisconnectSession CommandType = "disconnect_session"
	CommandTypeSendMessage       CommandType = "send_message"
)

// Command 是 CQRS 中表达意图的命令对象。
//
// 与 DomainEvent 不同，Command 描述的是"期望发生的事情"，
// 而非"已经发生的事实"。Command 经 CommandBus 分发后
// 由对应 Handler 处理，最终产生一个或多个 DomainEvent。
type Command struct {
	CommandID   string            `json:"command_id"`
	CommandType CommandType       `json:"command_type"`
	AggregateID string            `json:"aggregate_id"`
	IssuedAt    time.Time         `json:"issued_at"`
	Data        json.RawMessage   `json:"data"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ConnectSessionCommandData 表示连接会话命令的数据。
type ConnectSessionCommandData struct {
	ConnectionIdentifier string `json:"connection_identifier"`
	RemoteAddress        string `json:"remote_address,omitempty"`
}

// DisconnectSessionCommandData 表示断开会话命令的数据。
type DisconnectSessionCommandData struct {
	ConnectionIdentifier string `json:"connection_identifier"`
}

// SendMessageCommandData 表示发送消息命令的数据。
type SendMessageCommandData struct {
	ConnectionIdentifier string          `json:"connection_identifier"`
	MessagePayload       json.RawMessage `json:"message_payload"`
	SkillsID             string          `json:"skills_id,omitempty"`
}

// NewCommand 根据命令类型和数据构建命令对象。
func NewCommand(
	commandType CommandType,
	aggregateID string,
	commandData interface{},
	metadata map[string]string,
) (Command, error) {
	payload, marshalError := json.Marshal(commandData)
	if marshalError != nil {
		return Command{}, fmt.Errorf("序列化命令数据失败: %w", marshalError)
	}

	return Command{
		CommandID:   newEventIdentifier(),
		CommandType: commandType,
		AggregateID: aggregateID,
		IssuedAt:    time.Now(),
		Data:        json.RawMessage(payload),
		Metadata:    metadata,
	}, nil
}

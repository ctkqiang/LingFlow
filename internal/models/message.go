package models

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	UserChat     MessageType = "user_chat"     // 用户发送的普通聊天消息
	SystemChat   MessageType = "system_chat"   // 系统发送的通知类消息
	HeartbeatChat MessageType = "heartbeat_chat" // 心跳消息（ping/pong）
)

type WSMessage struct {
	Type      MessageType     `json:"type"`      // 消息类型，决定如何解析 Data
	Data      json.RawMessage `json:"data"`      // 动态数据部分，根据不同类型解析为不同的结构体
	SkillsId  string          `json:"skills_id"` // 关联的技能ID
	Timestamp time.Time       `json:"timestamp"` // 消息发送的时间戳
}

// UserChatData 定义了 Type 为 "user_chat" 时，Data 字段的结构
type UserChatData struct {
	ID      int64  `json:"id"`      // 消息在数据库中的唯一ID (对应你提供的 UserMessage 模型)
	UserID  string `json:"user_id"` // 发送消息的用户ID
	Message string `json:"message"` // 用户发送的文本内容
}

// SystemChatData 定义了 Type 为 "system_chat" 时，Data 字段的结构
type SystemChatData struct {
	Event   string `json:"event"`   // 系统事件类型，如 "user_joined", "user_left", "server_maintenance"
	Message string `json:"message"` // 系统通知的文本内容
}

// HeartbeatChatData 定义了 Type 为 "heartbeat_chat" 时，Data 字段的结构
type HeartbeatChatData struct {
	Action    string    `json:"action"`    // 动作类型："ping" 或 "pong"
	Nonce     string    `json:"nonce"`     // 随机标识，用于匹配 ping/pong
	Timestamp time.Time `json:"timestamp"` // 发送时间戳
	Latency   int64     `json:"latency,omitempty"` // 往返延迟（毫秒），仅 pong 时返回
}

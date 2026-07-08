package events

// ChatSessionEventData 表示 chat session 生命周期事件的数据。
type ChatSessionEventData struct {
	ConnectionIdentifier string `json:"connection_identifier"`
	RemoteAddress        string `json:"remote_address,omitempty"`
}

// ChatMessageEventData 表示 WebSocket chat 消息事件的数据。
type ChatMessageEventData struct {
	ConnectionIdentifier string `json:"connection_identifier"`
	MessageType          int    `json:"message_type"`
	PayloadSizeBytes     int    `json:"payload_size_bytes"`
	Payload              string `json:"payload,omitempty"`
	ErrorMessage         string `json:"error_message,omitempty"`
}

// ChatBroadcastEventData 表示消息广播事件的数据。
type ChatBroadcastEventData struct {
	ConnectionIdentifier string `json:"connection_identifier,omitempty"`
	RecipientCount       int    `json:"recipient_count"`
	PayloadSizeBytes     int    `json:"payload_size_bytes"`
	FailedCount          int    `json:"failed_count"`
}

// SkillExecutionEventData 表示技能执行事件的数据。
type SkillExecutionEventData struct {
	SkillIdentifier string  `json:"skill_identifier"`
	UserQuery       string  `json:"user_query"`
	MatchScore      float32 `json:"match_score,omitempty"`
	ErrorMessage    string  `json:"error_message,omitempty"`
}

// LLMGenerationEventData 表示 LLM 生成事件的数据。
type LLMGenerationEventData struct {
	SkillIdentifier string        `json:"skill_identifier,omitempty"`
	TokensUsed      int           `json:"tokens_used"`
	LatencyMs       int64         `json:"latency_ms"`
	FinishReason    string        `json:"finish_reason"`
	ErrorMessage    string        `json:"error_message,omitempty"`
}

// ChatStreamID 返回 chat aggregate 对应的事件流标识。
func ChatStreamID(connectionIdentifier string) string {
	return "chat:" + connectionIdentifier
}

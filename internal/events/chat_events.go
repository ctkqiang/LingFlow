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

// ChatStreamID 返回 chat aggregate 对应的事件流标识。
func ChatStreamID(connectionIdentifier string) string {
	return "chat:" + connectionIdentifier
}

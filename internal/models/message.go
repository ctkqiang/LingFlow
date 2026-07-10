package models

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	UserChat         MessageType = "user_chat"          // 用户发送的普通聊天消息
	SystemChat       MessageType = "system_chat"        // 系统发送的通知类消息
	SystemThinking   MessageType = "system_thinking"    // 系统思考过程（技能匹配、推理）
	SystemResponse   MessageType = "system_response"    // 系统最终响应消息
	SystemSkillsList MessageType = "system_skills_list" // 服务端推送的可用技能列表
	HeartbeatChat    MessageType = "heartbeat_chat"     // 心跳消息（ping/pong）
)

type WSMessage struct {
	Type      MessageType     `json:"type"`      // 消息类型，决定如何解析 Data
	Data      json.RawMessage `json:"data"`      // 动态数据部分，根据不同类型解析为不同的结构体
	SkillsId  string          `json:"skills_id"` // 关联的技能ID
	Timestamp time.Time       `json:"timestamp"` // 消息发送的时间戳
}

// UserChatData 定义了 Type 为 "user_chat" 时，Data 字段的结构
type UserChatData struct {
	ID            int64  `json:"id"`             // 消息在数据库中的唯一ID
	UserID        string `json:"user_id"`        // 发送消息的用户ID
	Message       string `json:"message"`        // 用户发送的文本内容
	SelectedSkill string `json:"selected_skill"` // 用户手动选中的技能（可选）
}

// SystemChatData 定义了 Type 为 "system_chat" 时，Data 字段的结构
type SystemChatData struct {
	Event   string `json:"event"`   // 系统事件类型，如 "user_joined", "user_left", "server_maintenance"
	Message string `json:"message"` // 系统通知的文本内容
}

// SystemThinkingData 定义了 Type 为 "system_thinking" 时，Data 字段的结构
type SystemThinkingData struct {
	Phase         string                 `json:"phase"`              // 当前处理阶段：skill_selection, llm_generation, formatting
	SkillMatches  []SkillMatch           `json:"skill_matches"`      // 技能匹配结果列表
	SelectedSkill *SkillMatch            `json:"selected_skill"`     // 最终选中的技能
	Thought       string                 `json:"thought"`            // 思考描述文本
	Metadata      map[string]interface{} `json:"metadata,omitempty"` // 额外元数据（如延迟、token数）
}

// SkillMatch 表示技能匹配结果
type SkillMatch struct {
	SkillIdentifier  string  `json:"skill_identifier"`   // 技能唯一标识符（S3 路径）
	SkillDisplayName string  `json:"skill_display_name"` // 技能显示名称
	MatchScore       float32 `json:"match_score"`        // 匹配分数
	SkillCategory    string  `json:"skill_category"`     // 技能分类
}

// SystemResponseData 定义了 Type 为 "system_response" 时，Data 字段的结构
type SystemResponseData struct {
	Content      string                 `json:"content"`            // LLM 生成的响应内容
	SkillUsed    *SkillMatch            `json:"skill_used"`         // 使用的技能（可选）
	FinishReason string                 `json:"finish_reason"`      // 响应结束原因：end_turn, max_tokens, error
	TokensUsed   int                    `json:"tokens_used"`        // 消耗的 token 总数
	LatencyMs    int64                  `json:"latency_ms"`         // 总延迟（毫秒）
	Metadata     map[string]interface{} `json:"metadata,omitempty"` // 额外元数据
}

// SystemSkillsListData 定义了 Type 为 "system_skills_list" 时，Data 字段的结构。
// 连接建立后，服务端主动推送可用技能列表，客户端据此展示技能菜单。
type SystemSkillsListData struct {
	Skills    []SkillListItem `json:"skills"`     // 可用技能列表
	Total     int             `json:"total"`      // 技能总数
	Source    string          `json:"source"`     // 技能来源：s3, local, hybrid
	UpdatedAt time.Time       `json:"updated_at"` // 列表更新时间
}

// SkillListItem 技能列表项（轻量级，不含完整内容）
type SkillListItem struct {
	SkillIdentifier  string   `json:"skill_identifier"`   // 技能唯一标识符，如 "/trade_analysis"
	SkillDisplayName string   `json:"skill_display_name"` // 技能显示名称
	SkillDescription string   `json:"skill_description"`  // 技能描述
	SkillCategory    string   `json:"skill_category"`     // 技能分类
	SearchKeywords   []string `json:"search_keywords"`    // 搜索关键词
}

// HeartbeatChatData 定义了 Type 为 "heartbeat_chat" 时，Data 字段的结构
type HeartbeatChatData struct {
	Action    string    `json:"action"`            // 动作类型："ping" 或 "pong"
	Nonce     string    `json:"nonce"`             // 随机标识，用于匹配 ping/pong
	Timestamp time.Time `json:"timestamp"`         // 发送时间戳
	Latency   int64     `json:"latency,omitempty"` // 往返延迟（毫秒），仅 pong 时返回
}

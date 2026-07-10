export type MessageType = 
  | 'user_chat'
  | 'system_chat'
  | 'system_thinking'
  | 'system_response'
  | 'system_skills_list'
  | 'heartbeat_chat'

export interface WSMessage {
  type: MessageType
  data: Record<string, unknown>
  skills_id: string
  timestamp: string
}

export interface UserChatData {
  id: number
  user_id: string
  message: string
  selected_skill?: string
}

export interface SystemChatData {
  event: string
  message: string
}

export interface SkillMatch {
  skill_identifier: string
  skill_display_name: string
  match_score: number
  skill_category: string
}

export interface SystemThinkingData {
  phase: string
  skill_matches?: SkillMatch[]
  selected_skill?: SkillMatch
  thought?: string
  metadata?: Record<string, unknown>
}

export interface SystemResponseData {
  content: string
  skill_used?: SkillMatch
  finish_reason?: string
  tokens_used?: number
  latency_ms?: number
  metadata?: Record<string, unknown>
}

export interface SkillListItem {
  skill_identifier: string
  skill_display_name: string
  skill_description: string
  skill_category: string
  search_keywords: string[]
}

export interface SystemSkillsListData {
  skills: SkillListItem[]
  total: number
  source: string
  updated_at: string
}

export interface HeartbeatChatData {
  action: 'ping' | 'pong'
  nonce: string
  timestamp: string
  latency?: number
}

export interface AuthResponse {
  token: string
  expires_at: number
  user_id: string
  ttl: string
}

export interface ChatMessage {
  id: string
  type: MessageType
  content: string
  timestamp: string
  isSystem: boolean
  skillUsed?: SkillMatch
  phase?: string
  metadata?: Record<string, unknown>
}

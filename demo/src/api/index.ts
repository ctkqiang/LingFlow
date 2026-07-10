import type { AuthResponse, WSMessage, SkillListItem } from '@/types'

const API_BASE = '/api'
const WS_BASE = '/chat'

export async function fetchAuthToken(userId: string): Promise<AuthResponse> {
  const response = await fetch(`${API_BASE}/auth/token`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ user_id: userId })
  })
  
  if (!response.ok) {
    throw new Error('认证失败')
  }
  
  return response.json()
}

export function createWebSocket(token: string, sessionId: string): WebSocket {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}${WS_BASE}/${sessionId}?token=${encodeURIComponent(token)}`
  return new WebSocket(wsUrl)
}

export function sendMessage(
  ws: WebSocket,
  type: string,
  data: Record<string, unknown>,
  skillsId: string = ''
): void {
  const message: WSMessage = {
    type: type as WSMessage['type'],
    data,
    skills_id: skillsId,
    timestamp: new Date().toISOString()
  }
  ws.send(JSON.stringify(message))
}

export function generateSessionId(): string {
  return Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15)
}

export function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp)
  const hours = date.getHours().toString().padStart(2, '0')
  const minutes = date.getMinutes().toString().padStart(2, '0')
  return `${hours}:${minutes}`
}

export function groupSkillsByCategory(skills: SkillListItem[]): Record<string, SkillListItem[]> {
  return skills.reduce((acc, skill) => {
    const category = skill.skill_category || '其他'
    if (!acc[category]) {
      acc[category] = []
    }
    acc[category].push(skill)
    return acc
  }, {} as Record<string, SkillListItem[]>)
}

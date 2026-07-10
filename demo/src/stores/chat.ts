import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { ChatMessage, SkillListItem, SkillMatch } from '@/types'
import { fetchAuthToken, createWebSocket, sendMessage, generateSessionId, formatTimestamp } from '@/api'

export const useChatStore = defineStore('chat', () => {
  const messages = ref<ChatMessage[]>([])
  const skills = ref<SkillListItem[]>([])
  const selectedSkill = ref<string>('')
  const userId = ref('')
  const token = ref('')
  const isConnected = ref(false)
  const isAuthenticating = ref(false)
  const isSending = ref(false)
  const connectionError = ref('')
  const ws = ref<WebSocket | null>(null)

  const groupedSkills = computed(() => {
    const groups: Record<string, SkillListItem[]> = {}
    skills.value.forEach(skill => {
      const category = skill.skill_category || '其他'
      if (!groups[category]) {
        groups[category] = []
      }
      groups[category].push(skill)
    })
    return groups
  })

  const selectedSkillInfo = computed(() => {
    if (!selectedSkill.value) return null
    return skills.value.find(s => s.skill_identifier === selectedSkill.value) || null
  })

  async function authenticate(userIdInput: string) {
    isAuthenticating.value = true
    connectionError.value = ''
    
    try {
      const response = await fetchAuthToken(userIdInput)
      userId.value = response.user_id
      token.value = response.token
      return response
    } catch (error) {
      connectionError.value = '认证失败，请重试'
      throw error
    } finally {
      isAuthenticating.value = false
    }
  }

  function connect() {
    if (!token.value) {
      connectionError.value = '请先认证'
      return
    }

    const sessionId = generateSessionId()
    ws.value = createWebSocket(token.value, sessionId)

    ws.value.onopen = () => {
      isConnected.value = true
      connectionError.value = ''
      addSystemMessage('连接已建立，正在加载技能列表...')
    }

    ws.value.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        handleWebSocketMessage(data)
      } catch (error) {
        console.error('消息解析失败:', error)
      }
    }

    ws.value.onerror = () => {
      connectionError.value = '连接出错，请检查服务是否正常运行'
      isConnected.value = false
    }

    ws.value.onclose = () => {
      isConnected.value = false
      if (!connectionError.value) {
        connectionError.value = '连接已断开'
      }
    }
  }

  function handleWebSocketMessage(message: unknown) {
    const msg = message as Record<string, unknown>
    const type = msg.type as string
    const data = msg.data as Record<string, unknown>
    const timestamp = msg.timestamp as string

    switch (type) {
      case 'system_skills_list':
        handleSkillsList(data)
        break
      case 'system_thinking':
        handleThinking(data, timestamp)
        break
      case 'system_response':
        handleResponse(data, timestamp)
        break
      case 'system_chat':
        handleSystemChat(data)
        break
      case 'heartbeat_chat':
        handleHeartbeat(data)
        break
    }
  }

  function handleSkillsList(data: Record<string, unknown>) {
    skills.value = (data.skills as SkillListItem[]) || []
    addSystemMessage(`已加载 ${skills.value.length} 个技能`)
  }

  function handleThinking(data: Record<string, unknown>, timestamp: string) {
    const phase = data.phase as string
    const thought = data.thought as string || ''
    const selectedSkillData = data.selected_skill as SkillMatch | undefined

    let content = ''
    if (phase === 'skill_selection') {
      content = `🔍 正在匹配技能... ${thought}`
    } else if (phase === 'llm_generation') {
      content = `🤔 AI 正在思考... ${thought}`
    } else {
      content = `📝 ${phase}: ${thought}`
    }

    messages.value.push({
      id: `thinking-${Date.now()}`,
      type: 'system_thinking',
      content,
      timestamp: formatTimestamp(timestamp),
      isSystem: true,
      phase,
      skillUsed: selectedSkillData
    })
  }

  function handleResponse(data: Record<string, unknown>, timestamp: string) {
    const content = data.content as string
    const skillUsed = data.skill_used as SkillMatch | undefined
    const tokensUsed = data.tokens_used as number

    let responseContent = content
    if (tokensUsed) {
      responseContent += `\n\n💡 消耗 Token: ${tokensUsed}`
    }

    messages.value.push({
      id: `response-${Date.now()}`,
      type: 'system_response',
      content: responseContent,
      timestamp: formatTimestamp(timestamp),
      isSystem: false,
      skillUsed
    })
  }

  function handleSystemChat(data: Record<string, unknown>) {
    const event = data.event as string
    const message = data.message as string
    
    let content = message
    if (event === 'invalid_skill_name') {
      content = `❌ ${message}`
    } else if (event === 'prompt_injection_detected') {
      content = `⚠️ ${message}`
    } else {
      content = `📢 ${message}`
    }

    addSystemMessage(content)
  }

  function handleHeartbeat(data: Record<string, unknown>) {
    const action = data.action as string
    const latency = data.latency as number
    
    if (action === 'pong' && latency) {
      console.log(`心跳延迟: ${latency}ms`)
    }
  }

  async function sendChatMessage(message: string) {
    if (!ws.value || !isConnected.value) {
      connectionError.value = '连接未建立'
      return
    }

    if (!message.trim()) return

    isSending.value = true
    
    messages.value.push({
      id: `user-${Date.now()}`,
      type: 'user_chat',
      content: message,
      timestamp: formatTimestamp(new Date().toISOString()),
      isSystem: false
    })

    const userData = {
      id: Date.now(),
      user_id: userId.value,
      message: message.trim(),
      selected_skill: selectedSkill.value || undefined
    }

    sendMessage(ws.value, 'user_chat', userData, selectedSkill.value)
    isSending.value = false
  }

  function addSystemMessage(content: string) {
    messages.value.push({
      id: `system-${Date.now()}`,
      type: 'system_chat',
      content,
      timestamp: formatTimestamp(new Date().toISOString()),
      isSystem: true
    })
  }

  function selectSkill(skillIdentifier: string) {
    selectedSkill.value = selectedSkill.value === skillIdentifier ? '' : skillIdentifier
  }

  function disconnect() {
    if (ws.value) {
      ws.value.close()
      ws.value = null
    }
    isConnected.value = false
  }

  function clearMessages() {
    messages.value = []
  }

  return {
    messages,
    skills,
    selectedSkill,
    userId,
    token,
    isConnected,
    isAuthenticating,
    isSending,
    connectionError,
    groupedSkills,
    selectedSkillInfo,
    authenticate,
    connect,
    sendChatMessage,
    selectSkill,
    disconnect,
    clearMessages
  }
})

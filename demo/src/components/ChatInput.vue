<script setup lang="ts">
import { ref, computed } from 'vue'
import { useChatStore } from '@/stores/chat'

const chatStore = useChatStore()
const messageInput = ref('')
const isFocused = ref(false)

const hasContent = computed(() => messageInput.value.trim().length > 0)

function handleSend() {
  if (!messageInput.value.trim() || !chatStore.isConnected) return
  chatStore.sendChatMessage(messageInput.value.trim())
  messageInput.value = ''
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    handleSend()
  }
}
</script>

<template>
  <div class="chat-input-wrapper">
    <div class="input-floating" :class="{ focused: isFocused, hasContent }">
      <div class="input-inner">
        <textarea
          v-model="messageInput"
          class="message-textarea"
          placeholder="输入消息..."
          rows="1"
          :disabled="!chatStore.isConnected"
          @focus="isFocused = true"
          @blur="isFocused = false"
          @keydown="handleKeydown"
        ></textarea>
        
        <button
          class="send-button"
          :class="{ active: hasContent && chatStore.isConnected }"
          :disabled="!chatStore.isConnected || !hasContent || chatStore.isSending"
          @click="handleSend"
        >
          <span v-if="chatStore.isSending" class="loading-ring">
            <span></span><span></span><span></span>
          </span>
          <svg v-else class="send-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 2L11 13" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M22 2L15 22L11 13L2 9L22 2Z" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
      </div>
      
      <div class="input-footer">
        <span v-if="chatStore.selectedSkillInfo" class="selected-tag">
          <span class="tag-glow">✨</span>
          <span class="tag-text">{{ chatStore.selectedSkillInfo.skill_display_name }}</span>
          <button class="tag-close" @click="chatStore.selectSkill('')">×</button>
        </span>
        <span v-else class="hint-text">
          <span class="hint-icon">💡</span>
          按 Enter 发送，Shift+Enter 换行
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-input-wrapper {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 320px;
  padding: 16px 24px 24px;
  background: linear-gradient(to top, var(--bg-primary) 60%, transparent);
  z-index: 100;
}

.chat-sidebar.collapsed ~ .chat-main .chat-input-wrapper {
  right: 44px;
}

.input-floating {
  max-width: 800px;
  margin: 0 auto;
  background: var(--bg-secondary);
  border: 2px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 12px 16px;
  box-shadow: var(--shadow-sm);
  transition: all var(--transition-normal);
}

.input-floating.focused {
  border-color: var(--primary-color);
  box-shadow: 0 0 0 4px var(--primary-glow), var(--shadow-md);
  transform: translateY(-2px);
}

.input-floating.hasContent {
  border-color: var(--primary-light);
}

.input-inner {
  display: flex;
  align-items: flex-end;
  gap: 12px;
}

.message-textarea {
  flex: 1;
  padding: 10px 0;
  border: none;
  background: transparent;
  font-size: 15px;
  color: var(--text-primary);
  resize: none;
  outline: none;
  font-family: inherit;
  line-height: 1.5;
  max-height: 120px;
  min-height: 24px;
}

.message-textarea::placeholder {
  color: var(--text-muted);
}

.message-textarea:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.send-button {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: none;
  background: var(--border-color);
  color: var(--text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--transition-fast);
  flex-shrink: 0;
}

.send-button.active {
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-dark) 100%);
  color: white;
  box-shadow: 0 4px 12px var(--primary-glow);
}

.send-button.active:hover {
  transform: scale(1.1) rotate(-5deg);
  box-shadow: 0 6px 20px rgba(255, 123, 163, 0.4);
}

.send-button:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.send-icon {
  width: 18px;
  height: 18px;
}

.loading-ring {
  display: flex;
  gap: 3px;
  align-items: center;
}

.loading-ring span {
  width: 5px;
  height: 5px;
  background: white;
  border-radius: 50%;
  animation: bounce 1.4s ease-in-out infinite both;
}

.loading-ring span:nth-child(1) { animation-delay: -0.32s; }
.loading-ring span:nth-child(2) { animation-delay: -0.16s; }

@keyframes bounce {
  0%, 80%, 100% { transform: scale(0); }
  40% { transform: scale(1); }
}

.input-footer {
  display: flex;
  align-items: center;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border-light);
}

.selected-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-light) 100%);
  color: white;
  border-radius: var(--radius-full);
  font-size: 12px;
  font-weight: 500;
  animation: tagIn 0.3s ease;
}

@keyframes tagIn {
  from {
    opacity: 0;
    transform: scale(0.8) translateY(4px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.tag-glow {
  animation: sparkle 2s ease-in-out infinite;
}

@keyframes sparkle {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.6; transform: scale(1.2); }
}

.tag-close {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: none;
  background: rgba(255,255,255,0.3);
  color: white;
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--transition-fast);
}

.tag-close:hover {
  background: rgba(255,255,255,0.5);
  transform: scale(1.1);
}

.hint-text {
  font-size: 12px;
  color: var(--text-muted);
  display: flex;
  align-items: center;
  gap: 4px;
}

.hint-icon {
  opacity: 0.6;
}
</style>

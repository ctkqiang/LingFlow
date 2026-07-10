<script setup lang="ts">
import { marked } from 'marked'
import type { ChatMessage } from '@/types'

defineProps<{
  message: ChatMessage
}>()

function renderMarkdown(text: string): string {
  return marked.parse(text) as string
}
</script>

<template>
  <div 
    class="message-row" 
    :class="{ 
      'user-message': message.type === 'user_chat',
      'bot-message': message.type === 'system_response',
      'system-message': message.isSystem
    }"
  >
    <div v-if="message.type === 'user_chat'" class="avatar user-avatar">
      <span>😊</span>
    </div>
    <div v-else-if="message.type === 'system_response'" class="avatar bot-avatar">
      <span>🤖</span>
    </div>
    
    <div class="message-body">
      <div v-if="message.skillUsed && message.type === 'system_response'" class="skill-tag">
        <span class="tag-star">✨</span>
        <span>{{ message.skillUsed.skill_display_name }}</span>
      </div>
      
      <div 
        class="bubble" 
        :class="{ 
          'user-bubble': message.type === 'user_chat',
          'bot-bubble': message.type === 'system_response',
          'thinking-bubble': message.type === 'system_thinking',
          'system-bubble': message.type === 'system_chat'
        }"
      >
        <p v-if="message.type === 'user_chat'" class="message-text">{{ message.content }}</p>
        <div v-else class="markdown-content" v-html="renderMarkdown(message.content)"></div>
      </div>
      
      <div class="message-meta">
        <span class="timestamp">{{ message.timestamp }}</span>
        <span v-if="message.phase" class="phase-badge">{{ message.phase }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.message-row {
  display: flex;
  gap: 10px;
  padding: 8px 20px;
  animation: slideIn 0.3s ease;
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.user-message {
  flex-direction: row-reverse;
}

.avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
  margin-top: 4px;
}

.user-avatar {
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-light) 100%);
  box-shadow: 0 2px 8px var(--primary-glow);
}

.bot-avatar {
  background: linear-gradient(135deg, var(--accent-color) 0%, var(--accent-light) 100%);
  box-shadow: 0 2px 8px rgba(255, 162, 192, 0.3);
}

.message-body {
  display: flex;
  flex-direction: column;
  max-width: 70%;
}

.user-message .message-body {
  align-items: flex-end;
}

.skill-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 10px;
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-light) 100%);
  color: white;
  font-size: 11px;
  font-weight: 500;
  border-radius: var(--radius-full);
  margin-bottom: 6px;
  align-self: flex-start;
  box-shadow: 0 2px 6px var(--primary-glow);
  animation: tagPop 0.3s ease;
}

@keyframes tagPop {
  from { transform: scale(0.8); opacity: 0; }
  to { transform: scale(1); opacity: 1; }
}

.tag-star {
  animation: twinkle 2s ease-in-out infinite;
}

@keyframes twinkle {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.bubble {
  padding: 12px 16px;
  border-radius: var(--radius-lg);
  word-break: break-word;
  line-height: 1.7;
  font-size: 15px;
}

.user-bubble {
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-dark) 100%);
  color: white;
  border-radius: var(--radius-lg) var(--radius-lg) 4px var(--radius-lg);
  box-shadow: 0 3px 12px rgba(255, 123, 163, 0.3);
}

.bot-bubble {
  background: var(--bg-secondary);
  color: var(--text-primary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg) var(--radius-lg) var(--radius-lg) 4px;
  box-shadow: var(--shadow-sm);
}

.thinking-bubble {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  border: 1px dashed var(--border-color);
  border-radius: var(--radius-md);
  font-size: 13px;
}

.system-bubble {
  background: transparent;
  color: var(--text-secondary);
  font-size: 13px;
  padding: 6px 12px;
  text-align: center;
}

.message-text {
  line-height: 1.7;
}

.markdown-content :deep(h1) {
  font-size: 1.5em;
  font-weight: 700;
  margin-bottom: 0.5em;
  color: inherit;
}

.markdown-content :deep(h2) {
  font-size: 1.3em;
  font-weight: 600;
  margin-bottom: 0.5em;
  color: inherit;
  padding-bottom: 4px;
  border-bottom: 2px solid var(--primary-light);
}

.markdown-content :deep(h3) {
  font-size: 1.1em;
  font-weight: 600;
  margin-bottom: 0.4em;
  color: inherit;
}

.markdown-content :deep(p) {
  margin-bottom: 0.8em;
  line-height: 1.7;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  padding-left: 1.5em;
  margin-bottom: 0.8em;
}

.markdown-content :deep(li) {
  margin-bottom: 0.4em;
}

.markdown-content :deep(a) {
  color: var(--primary-color);
  text-decoration: none;
  border-bottom: 1px solid transparent;
  transition: all var(--transition-fast);
}

.markdown-content :deep(a:hover) {
  border-bottom-color: var(--primary-color);
}

.markdown-content :deep(code) {
  padding: 2px 6px;
  background: var(--bg-tertiary);
  border-radius: 4px;
  font-size: 0.9em;
  font-family: monospace;
  color: var(--primary-color);
}

.markdown-content :deep(pre) {
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
  padding: 12px;
  overflow-x: auto;
  margin-bottom: 0.8em;
}

.markdown-content :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}

.markdown-content :deep(blockquote) {
  border-left: 3px solid var(--primary-color);
  padding-left: 12px;
  margin-left: 0;
  margin-bottom: 0.8em;
  color: var(--text-secondary);
  font-style: italic;
}

.markdown-content :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 0.8em;
}

.markdown-content :deep(th),
.markdown-content :deep(td) {
  border: 1px solid var(--border-color);
  padding: 8px 12px;
  text-align: left;
}

.markdown-content :deep(th) {
  background: var(--bg-tertiary);
  font-weight: 600;
}

.markdown-content :deep(strong) {
  font-weight: 600;
  color: inherit;
}

.markdown-content :deep(em) {
  font-style: italic;
}

.user-bubble :deep(a) {
  color: white;
  border-bottom-color: rgba(255, 255, 255, 0.5);
}

.user-bubble :deep(a:hover) {
  border-bottom-color: white;
}

.user-bubble :deep(code) {
  background: rgba(255, 255, 255, 0.2);
}

.message-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  padding: 0 4px;
}

.user-message .message-meta {
  justify-content: flex-end;
}

.timestamp {
  font-size: 11px;
  color: var(--text-muted);
}

.phase-badge {
  font-size: 10px;
  color: var(--primary-color);
  padding: 2px 6px;
  background: var(--primary-glow);
  border-radius: 4px;
}

.system-message {
  justify-content: center;
}
</style>

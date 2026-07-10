<script setup lang="ts">
import { useChatStore } from '@/stores/chat'

const chatStore = useChatStore()

function handleDisconnect() {
  chatStore.disconnect()
  chatStore.clearMessages()
}
</script>

<template>
  <header class="chat-header">
    <div class="header-left">
      <div class="brand">
        <div class="brand-icon">
          <svg viewBox="0 0 40 40" class="brand-svg">
            <defs>
              <linearGradient id="brandGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#ff9ec4" />
                <stop offset="100%" stop-color="#ff7ba3" />
              </linearGradient>
            </defs>
            <circle cx="20" cy="20" r="16" fill="none" stroke="url(#brandGrad)" stroke-width="2.5" />
            <path d="M14 20 Q20 14 26 20 Q20 26 14 20" fill="url(#brandGrad)" opacity="0.8" />
          </svg>
        </div>
        <div class="brand-text">
          <h1 class="brand-name">LingFlow</h1>
          <div class="status-line">
            <span class="status-pulse" :class="{ online: chatStore.isConnected }"></span>
            <span class="status-text">{{ chatStore.isConnected ? '在线' : '离线' }}</span>
          </div>
        </div>
      </div>
    </div>
    
    <div class="header-right">
      <div class="user-chip">
        <span class="chip-icon">😊</span>
        <span class="chip-name">{{ chatStore.userId || '访客' }}</span>
      </div>
      <button class="action-btn disconnect" @click="handleDisconnect" title="断开连接">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
          <polyline points="16 17 21 12 16 7" />
          <line x1="21" y1="12" x2="9" y2="12" />
        </svg>
      </button>
    </div>
  </header>
</template>

<style scoped>
.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 24px;
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--border-color);
  position: relative;
  z-index: 50;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.brand-icon {
  width: 40px;
  height: 40px;
}

.brand-svg {
  width: 100%;
  height: 100%;
  animation: gentlePulse 3s ease-in-out infinite;
}

@keyframes gentlePulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.8; }
}

.brand-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.brand-name {
  font-size: 18px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-dark) 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.status-line {
  display: flex;
  align-items: center;
  gap: 6px;
}

.status-pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
  position: relative;
}

.status-pulse.online {
  background: var(--success-color);
}

.status-pulse.online::after {
  content: '';
  position: absolute;
  inset: -3px;
  border-radius: 50%;
  border: 2px solid var(--success-color);
  animation: ripple 2s ease-out infinite;
}

@keyframes ripple {
  0% { transform: scale(1); opacity: 1; }
  100% { transform: scale(2); opacity: 0; }
}

.status-text {
  font-size: 12px;
  color: var(--text-secondary);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
  border: 1px solid var(--border-color);
}

.chip-icon {
  font-size: 16px;
}

.chip-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.action-btn {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: none;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--transition-fast);
}

.action-btn:hover {
  background: rgba(255, 123, 123, 0.1);
  color: var(--error-color);
}

.action-btn svg {
  width: 18px;
  height: 18px;
}
</style>

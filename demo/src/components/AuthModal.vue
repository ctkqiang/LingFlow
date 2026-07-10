<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '@/stores/chat'

const emit = defineEmits<{
  authenticated: []
}>()

const chatStore = useChatStore()
const userIdInput = ref('')

async function handleAuthenticate() {
  if (!userIdInput.value.trim()) return
  
  await chatStore.authenticate(userIdInput.value.trim())
  await chatStore.connect()
  emit('authenticated')
}
</script>

<template>
  <div class="auth-overlay">
    <div class="auth-card">
      <div class="auth-visual">
        <div class="visual-ring ring-1"></div>
        <div class="visual-ring ring-2"></div>
        <div class="visual-ring ring-3"></div>
        <div class="visual-icon">🌸</div>
      </div>
      
      <div class="auth-content">
        <h1 class="auth-title">LingFlow</h1>
        <p class="auth-subtitle">AI 智能聊天助手演示</p>
        
        <div class="auth-form">
          <div class="input-group">
            <label class="input-label">用户名</label>
            <div class="input-wrap">
              <span class="input-prefix">@</span>
              <input
                v-model="userIdInput"
                type="text"
                class="input-field"
                placeholder="输入用户名开始体验"
                @keyup.enter="handleAuthenticate"
              />
            </div>
          </div>
          
          <button
            class="auth-btn"
            :disabled="chatStore.isAuthenticating || !userIdInput.trim()"
            @click="handleAuthenticate"
          >
            <span v-if="chatStore.isAuthenticating" class="btn-loader">
              <span></span><span></span><span></span>
            </span>
            <span v-else class="btn-text">
              开始体验
              <svg class="btn-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M5 12h14M12 5l7 7-7 7" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </span>
          </button>
        </div>
        
        <p class="auth-hint">
          <span class="hint-dot"></span>
          开发模式下无需真实凭证
        </p>
        
        <p v-if="chatStore.connectionError" class="auth-error">
          {{ chatStore.connectionError }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.auth-overlay {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #fff5f8 0%, #ffeaf0 50%, #fff0f5 100%);
  z-index: 1000;
}

.auth-card {
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(20px);
  border-radius: 24px;
  padding: 48px;
  width: 100%;
  max-width: 420px;
  box-shadow: 0 20px 60px rgba(255, 158, 196, 0.2), 0 0 0 1px rgba(255, 255, 255, 0.5);
  animation: cardEnter 0.5s cubic-bezier(0.34, 1.56, 0.64, 1);
}

@keyframes cardEnter {
  from {
    opacity: 0;
    transform: scale(0.9) translateY(20px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.auth-visual {
  position: relative;
  width: 100px;
  height: 100px;
  margin: 0 auto 24px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.visual-ring {
  position: absolute;
  border-radius: 50%;
  border: 2px solid var(--primary-light);
}

.ring-1 {
  width: 60px;
  height: 60px;
  animation: ringPulse 3s ease-in-out infinite;
}

.ring-2 {
  width: 80px;
  height: 80px;
  animation: ringPulse 3s ease-in-out infinite 0.5s;
}

.ring-3 {
  width: 100px;
  height: 100px;
  animation: ringPulse 3s ease-in-out infinite 1s;
}

@keyframes ringPulse {
  0%, 100% {
    transform: scale(1);
    opacity: 0.3;
  }
  50% {
    transform: scale(1.1);
    opacity: 0.1;
  }
}

.visual-icon {
  font-size: 40px;
  animation: iconFloat 3s ease-in-out infinite;
  z-index: 1;
}

@keyframes iconFloat {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-6px); }
}

.auth-content {
  text-align: center;
}

.auth-title {
  font-size: 28px;
  font-weight: 800;
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-dark) 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  margin-bottom: 8px;
}

.auth-subtitle {
  font-size: 14px;
  color: var(--text-secondary);
  margin-bottom: 32px;
}

.auth-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.input-group {
  text-align: left;
}

.input-label {
  display: block;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 8px;
}

.input-wrap {
  display: flex;
  align-items: center;
  background: var(--bg-tertiary);
  border: 2px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 0 16px;
  transition: all var(--transition-normal);
}

.input-wrap:focus-within {
  border-color: var(--primary-color);
  box-shadow: 0 0 0 4px var(--primary-glow);
}

.input-prefix {
  font-size: 16px;
  color: var(--primary-color);
  font-weight: 600;
  margin-right: 8px;
}

.input-field {
  flex: 1;
  padding: 14px 0;
  border: none;
  background: transparent;
  font-size: 16px;
  color: var(--text-primary);
  outline: none;
  font-family: inherit;
}

.input-field::placeholder {
  color: var(--text-muted);
}

.auth-btn {
  padding: 14px 24px;
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-dark) 100%);
  color: white;
  border: none;
  border-radius: var(--radius-lg);
  font-size: 16px;
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-normal);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  box-shadow: 0 4px 16px rgba(255, 123, 163, 0.3);
}

.auth-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 8px 24px rgba(255, 123, 163, 0.4);
}

.auth-btn:active:not(:disabled) {
  transform: translateY(0);
}

.auth-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-text {
  display: flex;
  align-items: center;
  gap: 8px;
}

.btn-arrow {
  width: 18px;
  height: 18px;
  transition: transform var(--transition-fast);
}

.auth-btn:hover .btn-arrow {
  transform: translateX(4px);
}

.btn-loader {
  display: flex;
  gap: 4px;
}

.btn-loader span {
  width: 8px;
  height: 8px;
  background: white;
  border-radius: 50%;
  animation: loaderBounce 1.4s ease-in-out infinite both;
}

.btn-loader span:nth-child(1) { animation-delay: -0.32s; }
.btn-loader span:nth-child(2) { animation-delay: -0.16s; }

@keyframes loaderBounce {
  0%, 80%, 100% { transform: scale(0); }
  40% { transform: scale(1); }
}

.auth-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  margin-top: 20px;
  font-size: 13px;
  color: var(--text-secondary);
}

.hint-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--success-color);
  animation: dotPulse 2s ease-in-out infinite;
}

@keyframes dotPulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.auth-error {
  margin-top: 16px;
  padding: 12px;
  background: rgba(255, 123, 123, 0.1);
  color: var(--error-color);
  border-radius: var(--radius-sm);
  font-size: 14px;
  animation: shake 0.4s ease;
}

@keyframes shake {
  0%, 100% { transform: translateX(0); }
  20% { transform: translateX(-8px); }
  40% { transform: translateX(8px); }
  60% { transform: translateX(-4px); }
  80% { transform: translateX(4px); }
}
</style>
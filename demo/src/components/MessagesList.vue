<script setup lang="ts">
import { ref, watch, nextTick, computed } from 'vue'
import { useChatStore } from '@/stores/chat'
import MessageBubble from './MessageBubble.vue'

const chatStore = useChatStore()
const messagesContainer = ref<HTMLElement | null>(null)
const showScrollButton = ref(false)

const isLoading = computed(() => chatStore.messages.length === 0 && chatStore.isConnected)

watch(
  () => chatStore.messages.length,
  async () => {
    await nextTick()
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
      showScrollButton.value = false
    }
  }
)

function scrollToBottom() {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    showScrollButton.value = false
  }
}

function handleScroll() {
  if (!messagesContainer.value) return
  const { scrollTop, scrollHeight, clientHeight } = messagesContainer.value
  showScrollButton.value = scrollHeight - scrollTop - clientHeight > 100
}
</script>

<template>
  <div ref="messagesContainer" class="messages-area" @scroll="handleScroll">
    <div class="messages-content">
      <div v-if="isLoading" class="loading-state">
        <div class="loading-spinner">
          <span></span><span></span><span></span>
        </div>
        <p class="loading-text">正在连接服务器...</p>
      </div>
      
      <div v-else-if="chatStore.messages.length === 0" class="welcome-screen">
        <div class="welcome-hero">
          <div class="hero-icon">🌸</div>
          <h2 class="hero-title">LingFlow</h2>
          <p class="hero-subtitle">AI 智能聊天助手</p>
        </div>
        
        <div class="feature-cards">
          <div class="feature-card">
            <div class="card-icon">🤖</div>
            <h3>AI 驱动</h3>
            <p>基于 AWS Bedrock 大语言模型</p>
          </div>
          <div class="feature-card">
            <div class="card-icon">📚</div>
            <h3>技能系统</h3>
            <p>动态加载领域知识技能</p>
          </div>
          <div class="feature-card">
            <div class="card-icon">⚡</div>
            <h3>实时响应</h3>
            <p>WebSocket 流式消息传输</p>
          </div>
        </div>
        
        <div class="quick-tips">
          <p class="tips-title">💡 快速开始</p>
          <div class="tip-items">
            <span class="tip-item">选择右侧技能开始对话</span>
            <span class="tip-item">输入 #create_skill 创建新技能</span>
          </div>
        </div>
      </div>
      
      <template v-else>
        <MessageBubble
          v-for="(message, index) in chatStore.messages"
          :key="message.id"
          :message="message"
          :class="{ 'message-enter': index >= chatStore.messages.length - 3 }"
        />
      </template>
    </div>
    
    <button 
      v-if="showScrollButton" 
      class="scroll-to-bottom"
      @click="scrollToBottom"
    >
      <span>↓</span>
    </button>
  </div>
</template>

<style scoped>
.messages-area {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding-bottom: 160px;
  position: relative;
}

.messages-content {
  min-height: 100%;
  display: flex;
  flex-direction: column;
}

.loading-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  min-height: 400px;
}

.loading-spinner {
  display: flex;
  gap: 6px;
}

.loading-spinner span {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--primary-color);
  animation: bounce 1.4s ease-in-out infinite both;
}

.loading-spinner span:nth-child(1) { animation-delay: -0.32s; }
.loading-spinner span:nth-child(2) { animation-delay: -0.16s; }

.loading-text {
  color: var(--text-secondary);
  font-size: 14px;
}

.welcome-screen {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 32px;
}

.welcome-hero {
  text-align: center;
  animation: fadeInUp 0.6s ease;
}

.hero-icon {
  font-size: 64px;
  margin-bottom: 16px;
  animation: float 3s ease-in-out infinite;
}

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10px); }
}

.hero-title {
  font-size: 32px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--primary-color) 0%, var(--primary-dark) 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.hero-subtitle {
  font-size: 16px;
  color: var(--text-secondary);
  margin-top: 8px;
}

.feature-cards {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  justify-content: center;
  max-width: 600px;
}

.feature-card {
  background: var(--bg-secondary);
  border-radius: var(--radius-lg);
  padding: 24px;
  text-align: center;
  min-width: 160px;
  box-shadow: var(--shadow-sm);
  border: 1px solid var(--border-color);
  transition: all var(--transition-normal);
  animation: fadeInUp 0.6s ease backwards;
}

.feature-card:nth-child(1) { animation-delay: 0.1s; }
.feature-card:nth-child(2) { animation-delay: 0.2s; }
.feature-card:nth-child(3) { animation-delay: 0.3s; }

.feature-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--shadow-md);
  border-color: var(--primary-light);
}

.card-icon {
  font-size: 32px;
  margin-bottom: 12px;
}

.feature-card h3 {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 6px;
}

.feature-card p {
  font-size: 13px;
  color: var(--text-secondary);
}

.quick-tips {
  background: var(--bg-secondary);
  border-radius: var(--radius-lg);
  padding: 20px 24px;
  border: 1px solid var(--border-color);
  max-width: 400px;
  width: 100%;
  animation: fadeInUp 0.6s 0.4s ease backwards;
}

.tips-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 12px;
}

.tip-items {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tip-item {
  font-size: 13px;
  color: var(--text-secondary);
  padding: 8px 12px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  transition: all var(--transition-fast);
}

.tip-item:hover {
  background: var(--secondary-light);
  color: var(--primary-color);
}

.message-enter {
  animation: messageSlideIn 0.3s ease;
}

@keyframes messageSlideIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.scroll-to-bottom {
  position: absolute;
  bottom: 20px;
  right: 24px;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--primary-color);
  color: white;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  box-shadow: var(--shadow-md);
  animation: fadeIn 0.2s ease;
  transition: all var(--transition-fast);
}

.scroll-to-bottom:hover {
  transform: scale(1.1);
  box-shadow: var(--shadow-lg);
}

@keyframes fadeIn {
  from { opacity: 0; transform: scale(0.8); }
  to { opacity: 1; transform: scale(1); }
}
</style>

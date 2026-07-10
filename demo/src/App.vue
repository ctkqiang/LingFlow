<script setup lang="ts">
import { ref } from 'vue'
import AuthModal from '@/components/AuthModal.vue'
import ChatHeader from '@/components/ChatHeader.vue'
import MessagesList from '@/components/MessagesList.vue'
import ChatInput from '@/components/ChatInput.vue'
import SkillsPanel from '@/components/SkillsPanel.vue'

const isAuthenticated = ref(false)
const showSidebar = ref(true)

function handleAuthenticated() {
  isAuthenticated.value = true
}
</script>

<template>
  <div class="app-container">
    <AuthModal v-if="!isAuthenticated" @authenticated="handleAuthenticated" />
    
    <div v-else class="chat-layout">
      <main class="chat-main">
        <ChatHeader />
        <MessagesList />
        <ChatInput />
      </main>
      
      <aside class="chat-sidebar" :class="{ collapsed: !showSidebar }">
        <button class="sidebar-toggle" @click="showSidebar = !showSidebar">
          <span class="toggle-arrow">{{ showSidebar ? '◀' : '▶' }}</span>
        </button>
        <div v-show="showSidebar" class="sidebar-content">
          <SkillsPanel />
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.app-container {
  height: 100vh;
  overflow: hidden;
}

.chat-layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
  background: linear-gradient(180deg, var(--bg-primary) 0%, var(--secondary-light) 100%);
}

.chat-sidebar {
  width: 320px;
  flex-shrink: 0;
  background: var(--bg-secondary);
  border-left: 1px solid var(--border-color);
  display: flex;
  position: relative;
  transition: width var(--transition-normal);
  box-shadow: -4px 0 20px rgba(255, 158, 196, 0.08);
}

.chat-sidebar.collapsed {
  width: 44px;
}

.sidebar-toggle {
  position: absolute;
  left: -16px;
  top: 50%;
  transform: translateY(-50%);
  width: 32px;
  height: 64px;
  background: var(--bg-secondary);
  border: 2px solid var(--border-color);
  border-radius: 16px 0 0 16px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--primary-color);
  font-size: 12px;
  transition: all var(--transition-fast);
  box-shadow: -4px 0 12px rgba(255, 158, 196, 0.15);
  z-index: 10;
}

.sidebar-toggle:hover {
  background: var(--primary-color);
  color: white;
  border-color: var(--primary-color);
}

.toggle-arrow {
  transition: transform var(--transition-fast);
}

.sidebar-toggle:hover .toggle-arrow {
  transform: scale(1.2);
}

.sidebar-content {
  flex: 1;
  overflow: hidden;
  width: 100%;
}
</style>

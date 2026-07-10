<script setup lang="ts">
import { computed } from 'vue'
import { useChatStore } from '@/stores/chat'

const chatStore = useChatStore()

const categories = computed(() => Object.keys(chatStore.groupedSkills))
</script>

<template>
  <div class="skills-panel">
    <div class="panel-header">
      <div class="header-title">
        <span class="title-icon">🎯</span>
        <h3>技能列表</h3>
      </div>
      <span class="skill-badge">{{ chatStore.skills.length }}</span>
    </div>

    <div v-if="chatStore.skills.length === 0" class="loading-area">
      <div class="loading-dots">
        <span></span><span></span><span></span>
      </div>
      <p>正在加载技能...</p>
    </div>

    <div v-else class="skills-scroll">
      <div class="skills-content">
        <div v-for="category in categories" :key="category" class="category-section">
          <div class="category-header">
            <div class="category-line"></div>
            <span class="category-name">{{ category }}</span>
            <div class="category-line"></div>
          </div>

          <div class="skill-grid">
            <button
              v-for="skill in chatStore.groupedSkills[category]"
              :key="skill.skill_identifier"
              class="skill-card"
              :class="{ active: chatStore.selectedSkill === skill.skill_identifier }"
              @click="chatStore.selectSkill(skill.skill_identifier)"
            >
              <div class="card-glow" v-if="chatStore.selectedSkill === skill.skill_identifier"></div>
              <div class="card-content">
                <span class="card-emoji">✨</span>
                <div class="card-text">
                  <span class="card-name">{{ skill.skill_display_name }}</span>
                  <span class="card-desc">{{ skill.skill_description }}</span>
                </div>
              </div>
              <div class="card-indicator" v-if="chatStore.selectedSkill === skill.skill_identifier">
                <span class="check-mark">✓</span>
              </div>
            </button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="chatStore.selectedSkillInfo" class="selected-bar">
      <div class="selected-content">
        <span class="selected-emoji">🌟</span>
        <div class="selected-text">
          <span class="selected-name">{{ chatStore.selectedSkillInfo.skill_display_name }}</span>
          <span class="selected-desc">{{ chatStore.selectedSkillInfo.skill_description }}</span>
        </div>
      </div>
      <button class="clear-btn" @click="chatStore.selectSkill('')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M18 6L6 18M6 6l12 12" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </button>
    </div>

    <div class="tip-bar">
      <span class="tip-emoji">💡</span>
      <span class="tip-text">
        输入 <code>#create_skill 名称 - 描述</code> 创建技能
      </span>
    </div>
  </div>
</template>

<style scoped>
.skills-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-secondary);
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title-icon {
  font-size: 18px;
}

.header-title h3 {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
}

.skill-badge {
  font-size: 12px;
  font-weight: 600;
  color: var(--primary-color);
  padding: 4px 10px;
  background: var(--primary-glow);
  border-radius: var(--radius-full);
}

.loading-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  color: var(--text-secondary);
}

.loading-dots {
  display: flex;
  gap: 6px;
}

.loading-dots span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--primary-color);
  animation: dotBounce 1.4s ease-in-out infinite both;
}

.loading-dots span:nth-child(1) { animation-delay: -0.32s; }
.loading-dots span:nth-child(2) { animation-delay: -0.16s; }

@keyframes dotBounce {
  0%, 80%, 100% { transform: scale(0); }
  40% { transform: scale(1); }
}

.skills-scroll {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}

.skills-content {
  padding: 16px;
}

.category-section {
  margin-bottom: 24px;
}

.category-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.category-line {
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, transparent, var(--border-color), transparent);
}

.category-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  white-space: nowrap;
}

.skill-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.skill-card {
  position: relative;
  padding: 14px;
  background: var(--bg-tertiary);
  border: 2px solid transparent;
  border-radius: var(--radius-md);
  cursor: pointer;
  text-align: left;
  transition: all var(--transition-fast);
  overflow: hidden;
}

.skill-card:hover {
  border-color: var(--primary-light);
  transform: translateX(4px);
}

.skill-card.active {
  background: var(--primary-glow);
  border-color: var(--primary-color);
}

.card-glow {
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, rgba(255, 158, 196, 0.1) 0%, transparent 100%);
  animation: glowPulse 2s ease-in-out infinite;
}

@keyframes glowPulse {
  0%, 100% { opacity: 0.5; }
  50% { opacity: 1; }
}

.card-content {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  position: relative;
  z-index: 1;
}

.card-emoji {
  font-size: 20px;
  flex-shrink: 0;
}

.card-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.card-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.card-desc {
  font-size: 12px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-indicator {
  position: absolute;
  top: 12px;
  right: 12px;
  width: 20px;
  height: 20px;
  background: var(--primary-color);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: indicatorPop 0.3s ease;
}

.check-mark {
  color: white;
  font-size: 12px;
  font-weight: 700;
}

@keyframes indicatorPop {
  from { transform: scale(0); }
  to { transform: scale(1); }
}

.selected-bar {
  padding: 12px 16px;
  background: var(--secondary-light);
  border-top: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  animation: slideUp 0.3s ease;
}

@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.selected-content {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.selected-emoji {
  font-size: 24px;
  flex-shrink: 0;
}

.selected-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.selected-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.selected-desc {
  font-size: 12px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.clear-btn {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--transition-fast);
  flex-shrink: 0;
}

.clear-btn:hover {
  background: rgba(255, 123, 123, 0.1);
  color: var(--error-color);
}

.clear-btn svg {
  width: 16px;
  height: 16px;
}

.tip-bar {
  padding: 12px 16px;
  background: var(--bg-primary);
  border-top: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.tip-emoji {
  font-size: 14px;
  flex-shrink: 0;
}

.tip-text {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.4;
}

.tip-text code {
  padding: 2px 6px;
  background: var(--bg-secondary);
  border-radius: 4px;
  font-size: 11px;
  color: var(--primary-color);
  font-family: monospace;
}
</style>

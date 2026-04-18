<template>
  <div class="settings-page">
    <div class="settings-container">
      <!-- 左侧导航 -->
            <div class="settings-sidebar">
              <div class="sidebar-header">
                <h2 class="sidebar-title">{{ $t('general.settings') }}</h2>
              </div>
              <div class="settings-nav">
                <template v-for="(group, groupIndex) in navGroups" :key="group.section">
                  <!-- 分组标题 -->
                  <div v-if="groupIndex > 0" class="nav-divider"></div>
                  <div class="nav-section-label">{{ group.label }}</div>

                  <template v-for="(item, index) in group.items" :key="item.key">
                    <div
                      :class="['nav-item', {
                        'active': currentSection === item.key,
                        'has-submenu': item.children && item.children.length > 0,
                        'expanded': expandedMenus.includes(item.key)
                      }]"
                      @click="handleNavClick(item)"
                    >
                      <!-- 网络搜索使用自定义 SVG 图标 -->
                      <svg
                        v-if="item.key === 'websearch' || item.key === 'platform-websearch'"
                        width="18"
                        height="18"
                        viewBox="0 0 18 18"
                        fill="none"
                        xmlns="http://www.w3.org/2000/svg"
                        class="nav-icon"
                      >
                        <circle cx="9" cy="9" r="7" stroke="currentColor" stroke-width="1.2" fill="none"/>
                        <path d="M 9 2 A 3.5 7 0 0 0 9 16" stroke="currentColor" stroke-width="1.2" fill="none"/>
                        <path d="M 9 2 A 3.5 7 0 0 1 9 16" stroke="currentColor" stroke-width="1.2" fill="none"/>
                        <line x1="2.94" y1="5.5" x2="15.06" y2="5.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
                        <line x1="2.94" y1="12.5" x2="15.06" y2="12.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
                      </svg>
                      <t-icon v-else :name="item.icon" class="nav-icon" />
                      <span class="nav-label">{{ item.label }}</span>
                      <t-icon
                        v-if="item.children && item.children.length > 0"
                        :name="expandedMenus.includes(item.key) ? 'chevron-down' : 'chevron-right'"
                        class="expand-icon"
                      />
                    </div>

                    <!-- 子菜单 -->
                    <Transition name="submenu">
                      <div
                        v-if="item.children && expandedMenus.includes(item.key)"
                        class="submenu"
                      >
                        <div
                          v-for="(child, childIndex) in item.children"
                          :key="childIndex"
                          :class="['submenu-item', { 'active': currentSubSection === child.key }]"
                          @click.stop="handleSubMenuClick(item.key, child.key)"
                        >
                          <span class="submenu-label">{{ child.label }}</span>
                        </div>
                      </div>
                    </Transition>
                  </template>
                </template>
              </div>
            </div>

            <!-- 右侧内容区域 -->
            <div class="settings-content">
              <div class="content-wrapper">
                <!-- 常规设置 -->
                <div v-if="currentSection === 'general'" class="section">
                  <GeneralSettings />
                </div>

                <!-- 租户模型配置 -->
                <div v-if="currentSection === 'models'" class="section">
                  <ModelSettings mode="tenant" />
                </div>

                <!-- 租户网络搜索配置 -->
                <div v-if="currentSection === 'websearch'" class="section">
                  <WebSearchSettings mode="tenant" />
                </div>

                <!-- 消息管理 -->
                <div v-if="currentSection === 'chathistory'" class="section">
                  <ChatHistorySettings />
                </div>

                <!-- 租户信息 -->
                <div v-if="currentSection === 'tenant'" class="section">
                  <TenantInfo />
                </div>

                <!-- API 信息 -->
                <div v-if="currentSection === 'api'" class="section">
                  <ApiInfo />
                </div>

                <!-- 租户 MCP 服务 -->
                <div v-if="currentSection === 'mcp'" class="section">
                  <McpSettings mode="tenant" />
                </div>

                <!-- 平台模型管理 -->
                <div v-if="currentSection === 'platform-models'" class="section">
                  <ModelSettings mode="platform" />
                </div>

                <!-- 平台网络搜索 -->
                <div v-if="currentSection === 'platform-websearch'" class="section">
                  <WebSearchSettings mode="platform" />
                </div>

                <!-- 平台 MCP 服务 -->
                <div v-if="currentSection === 'platform-mcp'" class="section">
                  <McpSettings mode="platform" />
                </div>

                <!-- Ollama 设置 -->
                <div v-if="currentSection === 'ollama'" class="section">
                  <OllamaSettings />
                </div>

                <!-- 解析引擎 -->
                <div v-if="currentSection === 'parser'" class="section">
                  <ParserEngineSettings />
                </div>

                <!-- 存储引擎 -->
                <div v-if="currentSection === 'storage'" class="section">
                  <StorageEngineSettings />
                </div>

                <!-- 系统信息 -->
                <div v-if="currentSection === 'system'" class="section">
                  <SystemInfo />
                </div>
              </div>
            </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'
import SystemInfo from './SystemInfo.vue'
import TenantInfo from './TenantInfo.vue'
import ApiInfo from './ApiInfo.vue'
import GeneralSettings from './GeneralSettings.vue'
import ModelSettings from './ModelSettings.vue'
import OllamaSettings from './OllamaSettings.vue'
import McpSettings from './McpSettings.vue'
import WebSearchSettings from './WebSearchSettings.vue'
import ChatHistorySettings from './ChatHistorySettings.vue'
import ParserEngineSettings from './ParserEngineSettings.vue'
import StorageEngineSettings from './StorageEngineSettings.vue'

const uiStore = useUIStore()
const authStore = useAuthStore()
const { t } = useI18n()

const currentSection = ref<string>('general')
const currentSubSection = ref<string>('')
const expandedMenus = ref<string[]>([])

// 角色判断
const isSuperAdmin = computed(() => authStore.canAccessAllTenants)
const isTenantOwner = computed(() => {
  const tenantOwnerId = authStore.tenant?.owner_id
  const userId = authStore.user?.id
  return !!tenantOwnerId && !!userId && tenantOwnerId === userId
})
const isAdmin = computed(() => isSuperAdmin.value || isTenantOwner.value)

// 分组导航项
interface NavItem {
  key: string
  icon: string
  label: string
  children?: { key: string; label: string }[]
}

interface NavGroup {
  section: string
  label: string
  items: NavItem[]
}

const navGroups = computed<NavGroup[]>(() => {
  const groups: NavGroup[] = []

  // 个人设置 — 所有用户可见
  groups.push({
    section: 'personal',
    label: t('settings.sectionPersonal'),
    items: [
      { key: 'general', icon: 'setting', label: t('general.title') },
      { key: 'chathistory', icon: 'chat', label: t('chatHistorySettings.title') },
    ]
  })

  // 租户配置 — 仅租户管理员（Owner）或超管可见
  if (isAdmin.value) {
    groups.push({
      section: 'tenant',
      label: t('settings.sectionTenant'),
      items: [
        {
          key: 'models',
          icon: 'control-platform',
          label: t('settings.modelManagement'),
          children: [
            { key: 'chat', label: t('model.llmModel') },
            { key: 'embedding', label: t('model.embeddingModel') },
            { key: 'rerank', label: t('model.rerankModel') },
            { key: 'vllm', label: t('model.vlmModel') }
          ]
        },
        { key: 'websearch', icon: 'search', label: t('settings.webSearchConfig') },
        { key: 'mcp', icon: 'tools', label: t('settings.mcpService') },
        { key: 'tenant', icon: 'user-circle', label: t('settings.tenantInfo') },
        { key: 'api', icon: 'secured', label: t('settings.apiInfo') },
      ]
    })
  }

  // 平台管理 — 仅超级管理员可见
  if (isSuperAdmin.value) {
    groups.push({
      section: 'platform',
      label: t('settings.sectionPlatform'),
      items: [
        { key: 'system', icon: 'info-circle', label: t('settings.systemSettings') },
        {
          key: 'parser',
          icon: 'file-search',
          label: t('settings.parserEngine'),
          children: [
            { key: 'builtin', label: 'Builtin (DocReader)' },
            { key: 'simple', label: 'Simple' },
            { key: 'markitdown', label: 'Markitdown' },
            { key: 'mineru', label: 'MinerU' },
            { key: 'mineru_cloud', label: 'MinerU Cloud' },
          ]
        },
        {
          key: 'storage',
          icon: 'cloud',
          label: t('settings.storageEngine'),
          children: [
            { key: 'local', label: 'Local' },
            { key: 'minio', label: 'MinIO' },
            { key: 'cos', label: t('settings.storage.cos') },
            { key: 'tos', label: t('settings.storage.tos') },
            { key: 's3', label: 'AWS S3' },
          ]
        },
        {
          key: 'platform-models',
          icon: 'control-platform',
          label: t('settings.platformModelManagement'),
          children: [
            { key: 'chat', label: t('model.llmModel') },
            { key: 'embedding', label: t('model.embeddingModel') },
            { key: 'rerank', label: t('model.rerankModel') },
            { key: 'vllm', label: t('model.vlmModel') }
          ]
        },
        { key: 'platform-websearch', icon: 'search', label: t('settings.platformWebSearch') },
        { key: 'platform-mcp', icon: 'tools', label: t('settings.platformMcp') },
        { key: 'ollama', icon: 'server', label: 'Ollama' },
      ]
    })
  }

  return groups
})

// 扁平化的导航项（用于查找和事件处理）
const navItems = computed(() => {
  return navGroups.value.flatMap(g => g.items)
})

// 导航项点击处理
const handleNavClick = (item: any) => {
  if (item.children && item.children.length > 0) {
    // 有子菜单，切换展开状态
    const index = expandedMenus.value.indexOf(item.key)
    if (index > -1) {
      expandedMenus.value.splice(index, 1)
    } else {
      expandedMenus.value.push(item.key)
    }
    currentSubSection.value = item.children[0].key
  } else {
    currentSubSection.value = ''
  }
  
  // 切换到对应页面
  currentSection.value = item.key
}

// 子菜单点击处理
const handleSubMenuClick = (parentKey: string, childKey: string) => {
  currentSection.value = parentKey
  currentSubSection.value = childKey
  
  // 滚动到对应的模型类型区域
  setTimeout(() => {
    const element = document.querySelector(`[data-model-type="${childKey}"]`)
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, 100)
}

// 应用 uiStore 中预设的 section / subsection（由 openSettings 触发）
const applyInitialSection = (section: string | null, sub: string | null) => {
  if (!section) return
  currentSection.value = section
  const navItem = (navItems.value as any[]).find((item) => item.key === section)
  if (navItem && navItem.children && navItem.children.length > 0) {
    if (!expandedMenus.value.includes(section)) {
      expandedMenus.value.push(section)
    }
    currentSubSection.value = sub || navItem.children[0].key
    if (sub) {
      setTimeout(() => {
        const element = document.querySelector(`[data-model-type="${sub}"]`)
        if (element) {
          element.scrollIntoView({ behavior: 'smooth', block: 'start' })
        }
      }, 300)
    }
  } else {
    currentSubSection.value = ''
  }
}

watch(() => uiStore.settingsInitialSection, (section) => {
  applyInitialSection(section, uiStore.settingsInitialSubSection)
}, { immediate: true })

// 旧版跨组件跳转事件（settings-nav CustomEvent），保留兼容
const handleSettingsNav = (e: CustomEvent) => {
  const { section, subsection } = e.detail || {}
  applyInitialSection(section ?? null, subsection ?? null)
}

onMounted(() => {
  // 保持旧 watchers 语义：进入设置页 = 打开
  uiStore.showSettingsModal = true
  window.addEventListener('settings-nav', handleSettingsNav as EventListener)
})

onUnmounted(() => {
  uiStore.showSettingsModal = false
  window.removeEventListener('settings-nav', handleSettingsNav as EventListener)
})
</script>

<style lang="less" scoped>
/* 路由页容器 */
.settings-page {
  flex: 1;
  min-width: 0;
  height: 100%;
  background: var(--td-bg-color-container);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.settings-container {
  display: flex;
  height: 100%;
  width: 100%;
  overflow: hidden;
}

/* 左侧导航栏 */
.settings-sidebar {
  width: 220px;
  background-color: var(--td-bg-color-settings-modal);
  border-right: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.sidebar-header {
  padding: 24px 16px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.sidebar-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0;
}

.settings-nav {
  padding: 16px 8px;
  flex: 1;
}

.nav-section-label {
  padding: 4px 16px 6px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--td-text-color-placeholder);
  user-select: none;
}

.nav-divider {
  height: 1px;
  background: var(--td-component-stroke);
  margin: 8px 12px;
}

.nav-item {
  display: flex;
  align-items: center;
  padding: 10px 16px;
  margin-bottom: 4px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--td-text-color-secondary);
  font-size: 14px;
  transition: all 0.2s ease;
  user-select: none;

  &:hover {
    background-color: var(--td-bg-color-secondarycontainer-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background-color: rgba(7, 192, 95, 0.1);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.nav-icon {
  margin-right: 12px;
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: inherit;
}

.nav-label {
  flex: 1;
}

.expand-icon {
  margin-left: 4px;
  font-size: 14px;
  transition: transform 0.2s ease;
}

/* 子菜单 */
.submenu {
  margin-left: 32px;
  margin-bottom: 4px;
  overflow: hidden;
}

.submenu-item {
  padding: 8px 16px;
  margin-bottom: 2px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  transition: all 0.2s ease;
  user-select: none;

  &:hover {
    background-color: var(--td-bg-color-secondarycontainer-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background-color: rgba(7, 192, 95, 0.08);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.submenu-label {
  display: block;
}

/* 子菜单动画 */
.submenu-enter-active,
.submenu-leave-active {
  transition: all 0.2s ease;
}

.submenu-enter-from {
  opacity: 0;
  max-height: 0;
}

.submenu-enter-to {
  opacity: 1;
  max-height: 300px;
}

.submenu-leave-from {
  opacity: 1;
  max-height: 300px;
}

.submenu-leave-to {
  opacity: 0;
  max-height: 0;
}

/* 右侧内容区域 */
.settings-content {
  flex: 1;
  overflow-y: auto;
  background-color: var(--td-bg-color-container);
}

.content-wrapper {
  max-width: 600px;
  padding: 40px 48px;
}

.section {
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* 滚动条样式 */
.settings-sidebar::-webkit-scrollbar,
.settings-content::-webkit-scrollbar {
  width: 6px;
}

.settings-sidebar::-webkit-scrollbar-track {
  background: var(--td-bg-color-secondarycontainer);
}

.settings-sidebar::-webkit-scrollbar-thumb {
  background: var(--td-gray-color-5);
  border-radius: 3px;
}

.settings-sidebar::-webkit-scrollbar-thumb:hover {
  background: var(--td-gray-color-6);
}

.settings-content::-webkit-scrollbar-track {
  background: var(--td-bg-color-container);
}

.settings-content::-webkit-scrollbar-thumb {
  background: var(--td-gray-color-5);
  border-radius: 3px;
}

.settings-content::-webkit-scrollbar-thumb:hover {
  background: var(--td-gray-color-6);
}
</style>

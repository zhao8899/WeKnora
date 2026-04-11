<template>
  <Teleport to="body" :disabled="!isModalMode">
    <Transition :name="isModalMode ? 'modal' : ''">
      <div v-if="visible" :class="isModalMode ? 'settings-overlay' : 'settings-page'">
        <div :class="isModalMode ? 'settings-modal' : 'settings-page-shell'">
          <button v-if="isModalMode" class="close-btn" @click="handleClose" :aria-label="$t('general.close')">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
              <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            </svg>
          </button>

          <div class="settings-container">
            <div class="settings-sidebar">
              <div class="sidebar-header">
                <h2 class="sidebar-title">{{ $t('general.settings') }}</h2>
              </div>
              <div class="settings-nav">
                <template v-for="item in navItems" :key="item.key">
                  <div
                    :class="[
                      'nav-item',
                      {
                        active: currentSection === item.key,
                        'has-submenu': item.children && item.children.length > 0,
                        expanded: expandedMenus.includes(item.key)
                      }
                    ]"
                    @click="handleNavClick(item)"
                  >
                    <t-icon :name="item.icon" class="nav-icon" />
                    <span class="nav-label">{{ item.label }}</span>
                    <t-icon
                      v-if="item.children && item.children.length > 0"
                      :name="expandedMenus.includes(item.key) ? 'chevron-down' : 'chevron-right'"
                      class="expand-icon"
                    />
                  </div>

                  <Transition name="submenu">
                    <div v-if="item.children && expandedMenus.includes(item.key)" class="submenu">
                      <div
                        v-for="child in item.children"
                        :key="child.key"
                        :class="['submenu-item', { active: currentSubSection === child.key }]"
                        @click.stop="handleSubMenuClick(item.key, child.key)"
                      >
                        <span class="submenu-label">{{ child.label }}</span>
                      </div>
                    </div>
                  </Transition>
                </template>
              </div>
            </div>

            <div class="settings-content">
              <div class="content-wrapper">
                <div v-if="currentSection === 'general'" class="section">
                  <GeneralSettings />
                </div>

                <div v-if="currentSection === 'models'" class="section">
                  <ModelSettings />
                </div>

                <div v-if="currentSection === 'parser'" class="section">
                  <ParserEngineSettings />
                </div>

                <div v-if="currentSection === 'storage'" class="section">
                  <StorageEngineSettings />
                </div>

                <div v-if="currentSection === 'system'" class="section">
                  <SystemInfo />
                </div>

                <div v-if="currentSection === 'tenant'" class="section">
                  <TenantInfo />
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useUIStore } from '@/stores/ui'
import GeneralSettings from './GeneralSettings.vue'
import ModelSettings from './ModelSettings.vue'
import ParserEngineSettings from './ParserEngineSettings.vue'
import StorageEngineSettings from './StorageEngineSettings.vue'
import SystemInfo from './SystemInfo.vue'
import TenantInfo from './TenantInfo.vue'
import { getSettingsNavItems, type SettingsNavItem } from './nav'

const route = useRoute()
const router = useRouter()
const uiStore = useUIStore()
const { t } = useI18n()

const currentSection = ref<string>('general')
const currentSubSection = ref<string>('')
const expandedMenus = ref<string[]>([])

const navItems = computed<SettingsNavItem[]>(() => getSettingsNavItems(t))

const isRouteMode = computed(() => route.path === '/platform/settings')
const isModalMode = computed(() => !isRouteMode.value && uiStore.showSettingsModal)
const visible = computed(() => isRouteMode.value || isModalMode.value)

const scrollToSubSection = (subSection: string) => {
  setTimeout(() => {
    const element = document.querySelector(`[data-model-type="${subSection}"]`)
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, 100)
}

const applySection = (section: string, subSection?: string) => {
  const navItem = navItems.value.find(item => item.key === section)
  if (!navItem) {
    currentSection.value = 'general'
    currentSubSection.value = ''
    return
  }

  currentSection.value = navItem.key
  if (navItem.children && navItem.children.length > 0) {
    if (!expandedMenus.value.includes(navItem.key)) {
      expandedMenus.value.push(navItem.key)
    }
    currentSubSection.value = subSection || navItem.children[0].key
    if (currentSubSection.value) {
      scrollToSubSection(currentSubSection.value)
    }
    return
  }

  currentSubSection.value = ''
}

const handleNavClick = (item: SettingsNavItem) => {
  if (item.children && item.children.length > 0) {
    const index = expandedMenus.value.indexOf(item.key)
    if (index > -1) {
      expandedMenus.value.splice(index, 1)
    } else {
      expandedMenus.value.push(item.key)
    }
  }

  applySection(item.key)
}

const handleSubMenuClick = (parentKey: string, childKey: string) => {
  currentSection.value = parentKey
  currentSubSection.value = childKey
  scrollToSubSection(childKey)
}

const handleClose = () => {
  uiStore.closeSettings()
  if (isRouteMode.value) {
    router.back()
  }
}

watch(
  () => uiStore.settingsInitialSection,
  section => {
    if (section && visible.value) {
      applySection(section, uiStore.settingsInitialSubSection || '')
    }
  },
  { immediate: true }
)

watch(
  [currentSection, currentSubSection],
  ([section, subSection]) => {
    if (!visible.value) {
      return
    }
    uiStore.setSettingsTarget(section, subSection || undefined)
  }
)

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && isModalMode.value) {
    handleClose()
  }
}

const handleSettingsNav = (event: Event) => {
  const customEvent = event as CustomEvent<{ section?: string; subsection?: string }>
  const { section, subsection } = customEvent.detail || {}
  if (section) {
    applySection(section, subsection)
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleEscape)
  window.addEventListener('settings-nav', handleSettingsNav)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleEscape)
  window.removeEventListener('settings-nav', handleSettingsNav)
})
</script>

<style lang="less" scoped>
.settings-page {
  min-height: 100%;
  padding: 24px 32px;
  background:
    radial-gradient(circle at top right, rgba(7, 192, 95, 0.08), transparent 24%),
    linear-gradient(180deg, #f7fbf7 0%, #ffffff 260px);
}

.settings-overlay {
  position: fixed;
  inset: 0;
  z-index: 1100;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  backdrop-filter: blur(4px);
}

.settings-page-shell {
  width: 100%;
  min-height: calc(100vh - 120px);
  background: var(--td-bg-color-container);
  border-radius: 12px;
  border: 1px solid var(--td-component-stroke);
  box-shadow: 0 6px 28px rgba(15, 23, 42, 0.08);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.settings-modal {
  position: relative;
  width: 100%;
  max-width: 900px;
  height: 700px;
  background: var(--td-bg-color-container);
  border-radius: 12px;
  box-shadow: 0 6px 28px rgba(15, 23, 42, 0.08);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.close-btn {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 32px;
  height: 32px;
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  z-index: 10;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }
}

.settings-container {
  display: flex;
  height: 100%;
  width: 100%;
  overflow: hidden;
}

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

.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .settings-modal,
.modal-leave-active .settings-modal {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .settings-modal,
.modal-leave-to .settings-modal {
  transform: scale(0.95);
  opacity: 0;
}

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

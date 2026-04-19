<template>
  <div class="user-menu" :class="{ 'user-menu--collapsed': uiStore.sidebarCollapsed }" ref="menuRef">
    <!-- 用户按钮 -->
    <div class="user-button" @click="toggleMenu">
      <div class="user-avatar">
        <img v-if="userAvatar" :src="userAvatar" :alt="$t('common.avatar')" />
        <span v-else class="avatar-placeholder">{{ userInitial }}</span>
      </div>
      <template v-if="!uiStore.sidebarCollapsed">
        <div class="user-info">
          <div class="user-name">{{ userName }}</div>
          <div class="user-email">{{ userEmail }}</div>
        </div>
        <t-icon :name="menuVisible ? 'chevron-up' : 'chevron-down'" class="dropdown-icon" />
      </template>
    </div>

    <!-- 下拉菜单 -->
    <Transition name="dropdown">
      <div v-if="menuVisible" class="user-dropdown" @click.stop>
        <div
          class="menu-item"
          @click="handleOpenAllSettings"
        >
          <t-icon name="setting" class="menu-icon" />
          <span>{{ t('general.settings') }}</span>
        </div>
        <div class="menu-divider"></div>
        <div class="menu-item danger" @click="handleLogout">
          <t-icon name="logout" class="menu-icon" />
          <span>{{ $t('auth.logout') }}</span>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import { MessagePlugin } from 'tdesign-vue-next'
import { getCurrentUser, logout as logoutApi } from '@/api/auth'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const route = useRoute()
const router = useRouter()
const uiStore = useUIStore()
const authStore = useAuthStore()

const menuRef = ref<HTMLElement>()
const menuVisible = ref(false)

// 用户信息
const userInfo = ref({
  username: t('common.defaultUser'),
  email: 'user@example.com',
  avatar: ''
})

const userName = computed(() => userInfo.value.username)
const userEmail = computed(() => userInfo.value.email)
const userAvatar = computed(() => userInfo.value.avatar)

// 用户名首字母（用于无头像时显示）
const userInitial = computed(() => {
  return userName.value.charAt(0).toUpperCase()
})

// 切换菜单显示
const toggleMenu = () => {
  menuVisible.value = !menuVisible.value
}

const handleOpenAllSettings = () => {
  menuVisible.value = false
  uiStore.setSettingsTarget(
    uiStore.settingsInitialSection || 'general',
    uiStore.settingsInitialSubSection || undefined
  )

  if (route.path !== '/platform/settings') {
    router.push('/platform/settings')
  }
}

// 注销
const handleLogout = async () => {
  menuVisible.value = false
  
  try {
    // 调用后端API注销
    await logoutApi()
  } catch (error) {
    // 即使API调用失败，也继续执行本地清理
    console.error('注销API调用失败:', error)
  }
  
  // 清理所有状态和本地存储
  authStore.logout()
  
  MessagePlugin.success(t('auth.logout'))
  
  // 跳转到登录页
  router.push('/login')
}

// 加载用户信息
const loadUserInfo = async () => {
  try {
    const response = await getCurrentUser()
    if (response.success && response.data && response.data.user) {
      const user = response.data.user
      userInfo.value = {
        username: user.username || t('common.info'),
        email: user.email || 'user@example.com',
        avatar: user.avatar || ''
      }
      // 同时更新 authStore 中的用户信息，确保包含 can_access_all_tenants 字段
      authStore.setUser({
        id: user.id,
        username: user.username,
        email: user.email,
        avatar: user.avatar,
        tenant_id: user.tenant_id,
        can_access_all_tenants: user.can_access_all_tenants || false,
        created_at: user.created_at,
        updated_at: user.updated_at
      })
      // 如果返回了租户信息，也更新租户信息
      if (response.data.tenant) {
        authStore.setTenant({
          id: String(response.data.tenant.id),
          name: response.data.tenant.name,
          api_key: response.data.tenant.api_key || '',
          owner_id: user.id,
          created_at: response.data.tenant.created_at,
          updated_at: response.data.tenant.updated_at
        })
      }
    }
  } catch (error) {
    console.error('Failed to load user info:', error)
  }
}

// 点击外部关闭菜单
const handleClickOutside = (e: MouseEvent) => {
  if (menuRef.value && !menuRef.value.contains(e.target as Node)) {
    menuVisible.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  loadUserInfo()
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style lang="less" scoped>
.user-menu {
  position: relative;
  width: 100%;

  &--collapsed {
    .user-button {
      justify-content: center;
      padding: 8px;
      gap: 0;
    }

    .user-avatar {
      width: 32px;
      height: 32px;

      .avatar-placeholder {
        font-size: 13px;
      }
    }

    .user-dropdown {
      left: calc(100% + 8px);
      bottom: 0;
      right: auto;
      min-width: 200px;
    }
  }
}

.user-button {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  background: transparent;

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  &:active {
    transform: scale(0.98);
  }
}

.user-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  overflow: hidden;
  flex-shrink: 0;
  background: linear-gradient(135deg, var(--td-brand-color) 0%, var(--td-brand-color-active) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: width 0.2s ease, height 0.2s ease;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .avatar-placeholder {
    color: var(--td-text-color-anti);
    font-size: 16px;
    font-weight: 600;
  }
}

.user-info {
  flex: 1;
  min-width: 0;
  text-align: left;

  .user-name {
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .user-email {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

.dropdown-icon {
  font-size: 16px;
  color: var(--td-text-color-secondary);
  flex-shrink: 0;
  transition: transform 0.2s;
}

.user-dropdown {
  position: absolute;
  bottom: 100%;
  left: 8px;
  right: 8px;
  margin-bottom: 8px;
  background: var(--td-bg-color-container);
  border-radius: 8px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.12);
  border: 1px solid var(--td-component-stroke);
  overflow: hidden;
  max-height: min(70vh, 560px);
  overflow-y: auto;
  z-index: 1000;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  cursor: pointer;
  transition: all 0.2s;
  font-size: 14px;
  color: var(--td-text-color-primary);

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  &.danger {
    color: var(--td-error-color);

    &:hover {
      background: var(--td-error-color-light);
    }

    .menu-icon {
      color: var(--td-error-color);
    }
  }

  .menu-icon {
    font-size: 16px;
    color: var(--td-text-color-secondary);
    
    &.svg-icon {
      width: 16px;
      height: 16px;
      flex-shrink: 0;
    }
  }
}

.menu-divider {
  height: 1px;
  background: var(--td-component-stroke);
  margin: 4px 0;
}

// 下拉动画
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

.dropdown-enter-to,
.dropdown-leave-from {
  opacity: 1;
  transform: translateY(0);
}
</style>


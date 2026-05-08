import { reactive, ref, watch } from 'vue'
import { defineStore } from 'pinia'
import i18n from '@/i18n'
import { useAuthStore } from '@/stores/auth'

type MenuChild = Record<string, any>

interface MenuItem {
  title: string
  titleKey?: string
  icon: string
  path: string
  childrenPath?: string
  children?: MenuChild[]
}

const createMenuChildren = () => reactive<MenuChild[]>([])

const createMenuItems = (isPlatformAdmin: boolean): MenuItem[] => {
  if (isPlatformAdmin) {
    return [
      { title: '控制台', icon: 'home', path: 'home' },
      { title: '', titleKey: 'menu.usageAudit', icon: 'chart-bar', path: 'usage-audit' },
      { title: '', titleKey: 'menu.settings', icon: 'setting', path: 'settings' },
      { title: '', titleKey: 'menu.logout', icon: 'logout', path: 'logout' }
    ]
  }

  return [
    { title: '首页', icon: 'home', path: 'home' },
    { title: '', titleKey: 'menu.knowledgeBase', icon: 'zhishiku', path: 'knowledge-bases' },
    { title: 'FAQ', icon: 'faq', path: 'faq' },
    { title: '', titleKey: 'menu.knowledgeSearch', icon: 'search', path: 'knowledge-search' },
    { title: '', titleKey: 'menu.agents', icon: 'agent', path: 'agents' },
    { title: '', titleKey: 'menu.organizations', icon: 'organization', path: 'organizations' },
    {
      title: '',
      titleKey: 'menu.chat',
      icon: 'prefixIcon',
      path: 'creatChat',
      childrenPath: 'chat',
      children: createMenuChildren()
    },
    { title: '', titleKey: 'menu.settings', icon: 'setting', path: 'settings' },
    { title: '', titleKey: 'menu.logout', icon: 'logout', path: 'logout' }
  ]
}

export const useMenuStore = defineStore('menuStore', () => {
  const authStore = useAuthStore()
  const menuArr = ref<MenuItem[]>(createMenuItems(authStore.canAccessAllTenants))
  const isFirstSession = ref(false)
  const firstQuery = ref('')
  const firstMentionedItems = ref<any[]>([])
  const firstModelId = ref('')
  const firstImageFiles = ref<any[]>([])
  const prefillQuery = ref('')

  const applyMenuTranslations = () => {
    menuArr.value.forEach(item => {
      if (item.titleKey) {
        item.title = i18n.global.t(item.titleKey)
      }
    })
  }

  const rebuildMenu = (isPlatformAdmin: boolean) => {
    const existingChatChildren = menuArr.value.find(item => item.path === 'creatChat')?.children
    const nextItems = createMenuItems(isPlatformAdmin)
    const nextChatMenu = nextItems.find(item => item.path === 'creatChat')
    if (nextChatMenu && existingChatChildren) {
      nextChatMenu.children = existingChatChildren
    }
    menuArr.value = nextItems
    applyMenuTranslations()
  }

  applyMenuTranslations()

  watch(
    () => i18n.global.locale.value,
    () => {
      applyMenuTranslations()
    }
  )

  watch(
    () => authStore.canAccessAllTenants,
    isPlatformAdmin => {
      rebuildMenu(isPlatformAdmin)
    }
  )

  const getChatMenu = () => menuArr.value.find(item => item.path === 'creatChat')

  const clearMenuArr = () => {
    const chatMenu = getChatMenu()
    if (chatMenu && chatMenu.children) {
      chatMenu.children = createMenuChildren()
    }
  }

  const updatemenuArr = (obj: any) => {
    const chatMenu = getChatMenu()
    if (!chatMenu) {
      return
    }
    if (!chatMenu.children) {
      chatMenu.children = createMenuChildren()
    }
    const exists = chatMenu.children.some((item: MenuChild) => item.id === obj.id)
    if (!exists) {
      chatMenu.children.push(obj)
    }
  }

  const updataMenuChildren = (item: MenuChild) => {
    const chatMenu = getChatMenu()
    if (!chatMenu) {
      return
    }
    if (!chatMenu.children) {
      chatMenu.children = createMenuChildren()
    }
    chatMenu.children.unshift(item)
  }

  const updatasessionTitle = (sessionId: string, title: string) => {
    const chatMenu = getChatMenu()
    chatMenu?.children?.forEach((item: MenuChild) => {
      if (item.id === sessionId) {
        item.title = title
        item.isNoTitle = false
      }
    })
  }

  const changeIsFirstSession = (payload: boolean) => {
    isFirstSession.value = payload
  }

  const changeFirstQuery = (payload: string, mentionedItems: any[] = [], modelId: string = '', imageFiles: any[] = []) => {
    firstQuery.value = payload
    firstMentionedItems.value = mentionedItems
    firstModelId.value = modelId
    firstImageFiles.value = imageFiles
  }

  const setPrefillQuery = (q: string) => {
    prefillQuery.value = q
  }

  const consumePrefillQuery = () => {
    const q = prefillQuery.value
    prefillQuery.value = ''
    return q
  }

  return {
    menuArr,
    isFirstSession,
    firstQuery,
    firstMentionedItems,
    firstModelId,
    firstImageFiles,
    prefillQuery,
    clearMenuArr,
    updatemenuArr,
    updataMenuChildren,
    updatasessionTitle,
    changeIsFirstSession,
    changeFirstQuery,
    setPrefillQuery,
    consumePrefillQuery
  }
})

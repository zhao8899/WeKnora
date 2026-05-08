import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.ts'

export const supportedLocales = ['zh-CN', 'en-US', 'ru-RU', 'ko-KR'] as const
export type SupportedLocale = (typeof supportedLocales)[number]

const defaultLocale: SupportedLocale = 'zh-CN'
const loadedLocales = new Set<SupportedLocale>([defaultLocale])

const localeLoaders: Record<SupportedLocale, () => Promise<Record<string, unknown>>> = {
  'zh-CN': async () => zhCN,
  'en-US': () => import('./locales/en-US.ts').then(module => module.default),
  'ru-RU': () => import('./locales/ru-RU.ts').then(module => module.default),
  'ko-KR': () => import('./locales/ko-KR.ts').then(module => module.default)
}

export const normalizeLocale = (locale: string | null | undefined): SupportedLocale => {
  return supportedLocales.includes(locale as SupportedLocale) ? locale as SupportedLocale : defaultLocale
}

const savedLocale = normalizeLocale(localStorage.getItem('locale'))

const i18n = createI18n({
  legacy: false,
  locale: defaultLocale,
  fallbackLocale: defaultLocale,
  globalInjection: true,
  messages: {
    [defaultLocale]: zhCN
  }
})

export const loadLocaleMessages = async (locale: string): Promise<SupportedLocale> => {
  const normalizedLocale = normalizeLocale(locale)
  if (!loadedLocales.has(normalizedLocale)) {
    const messages = await localeLoaders[normalizedLocale]()
    ;(i18n.global.setLocaleMessage as (locale: string, message: Record<string, unknown>) => void)(normalizedLocale, messages)
    loadedLocales.add(normalizedLocale)
  }
  return normalizedLocale
}

export const setI18nLocale = async (locale: string): Promise<SupportedLocale> => {
  const normalizedLocale = await loadLocaleMessages(locale)
  ;(i18n.global.locale as unknown as { value: string }).value = normalizedLocale
  localStorage.setItem('locale', normalizedLocale)
  return normalizedLocale
}

export const initI18nLocale = async () => {
  await setI18nLocale(savedLocale)
}

export default i18n

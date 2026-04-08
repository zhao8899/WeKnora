import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.ts'
import enUS from './locales/en-US.ts'

const messages = {
  'zh-CN': zhCN,
  'en-US': enUS
}

const defaultLocale = 'zh-CN'
const supportedLocales = Object.keys(messages)
const savedLocale = localStorage.getItem('locale')
const locale = savedLocale && supportedLocales.includes(savedLocale) ? savedLocale : defaultLocale

const i18n = createI18n({
  legacy: false,
  locale,
  fallbackLocale: defaultLocale,
  globalInjection: true,
  messages
})

export default i18n

type TranslateFn = (key: string) => string

export type SettingsNavItem = {
  key: string
  label: string
  icon: string
  adminOnly?: boolean
  children?: Array<{
    key: string
    label: string
  }>
}

export function getSettingsNavItems(t: TranslateFn, isAdmin = false): SettingsNavItem[] {
  const allItems: SettingsNavItem[] = [
    {
      key: 'general',
      label: t('general.settings'),
      icon: 'setting'
    },
    {
      key: 'models',
      label: t('settings.modelConfig'),
      icon: 'control-platform',
      children: [
        { key: 'chat', label: t('modelSettings.chat.title') },
        { key: 'embedding', label: t('modelSettings.embedding.title') },
        { key: 'rerank', label: t('modelSettings.rerank.title') },
        { key: 'vllm', label: t('modelSettings.vllm.title') }
      ]
    },
    {
      key: 'ollama',
      label: 'Ollama',
      icon: 'logo-github-filled',
      adminOnly: true
    },
    {
      key: 'agent',
      label: t('settings.agentConfig'),
      icon: 'chat'
    },
    {
      key: 'websearch',
      label: t('settings.webSearchConfig'),
      icon: 'internet'
    },
    {
      key: 'parser',
      label: t('settings.parserEngine'),
      icon: 'file-search',
      adminOnly: true,
      children: [
        { key: 'mineru', label: 'MinerU' },
        { key: 'mineru_cloud', label: 'MinerU Cloud' }
      ]
    },
    {
      key: 'storage',
      label: t('settings.storageEngine'),
      icon: 'cloud',
      adminOnly: true,
      children: [
        { key: 'local', label: t('settings.storage.engineLocal') },
        { key: 'minio', label: 'MinIO' },
        { key: 'cos', label: t('settings.storage.engineCos') },
        { key: 'tos', label: t('settings.storage.engineTos') },
        { key: 's3', label: t('settings.storage.engineS3') }
      ]
    },
    {
      key: 'system',
      label: t('settings.systemInfo'),
      icon: 'desktop',
      adminOnly: true
    },
    {
      key: 'tenant',
      label: t('settings.tenantInfo'),
      icon: 'user'
    },
    {
      key: 'api',
      label: t('settings.apiInfo'),
      icon: 'code'
    },
    {
      key: 'knowledge-health',
      label: t('settings.health.title'),
      icon: 'chart-bubble',
      adminOnly: true
    }
  ]

  return isAdmin ? allItems : allItems.filter(item => !item.adminOnly)
}

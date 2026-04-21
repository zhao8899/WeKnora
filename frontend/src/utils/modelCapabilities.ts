import type { ModelConfig } from '@/api/model'

export function getFixedTemperatureForModel(model?: Partial<ModelConfig> | null): number | null {
  if (!model) return null

  const provider = String(model.parameters?.provider || '').toLowerCase()
  const modelName = String(model.name || '').toLowerCase()

  if (provider === 'moonshot' && (modelName.includes('kimi-k2.6') || modelName.includes('kimi-k2-6'))) {
    return 1
  }

  return null
}

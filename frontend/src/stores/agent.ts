import { defineStore } from 'pinia'
import { ref } from 'vue'
import { listAgents, type CustomAgent } from '@/api/agent'

const AGENT_CACHE_TTL_MS = 30_000

export interface AgentListState {
  data: CustomAgent[]
  disabledOwnAgentIds: string[]
}

export const useAgentStore = defineStore('agent', () => {
  const agents = ref<CustomAgent[]>([])
  const disabledOwnAgentIds = ref<string[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  let fetchedAt = 0
  let fetchPromise: Promise<AgentListState> | null = null

  const isFresh = () => fetchedAt > 0 && Date.now() - fetchedAt < AGENT_CACHE_TTL_MS

  function invalidateAgentsCache() {
    fetchedAt = 0
  }

  async function fetchAgents(options: { force?: boolean } = {}): Promise<AgentListState> {
    if (!options.force && isFresh()) {
      return {
        data: agents.value,
        disabledOwnAgentIds: disabledOwnAgentIds.value,
      }
    }

    if (fetchPromise) return fetchPromise

    loading.value = true
    error.value = null
    fetchPromise = (async () => {
      try {
        const response = await listAgents()
        agents.value = response.data || []
        disabledOwnAgentIds.value = response.disabled_own_agent_ids || []
        fetchedAt = Date.now()
        return {
          data: agents.value,
          disabledOwnAgentIds: disabledOwnAgentIds.value,
        }
      } catch (e: any) {
        error.value = e?.message || 'Failed to fetch agents'
        fetchedAt = 0
        return {
          data: agents.value,
          disabledOwnAgentIds: disabledOwnAgentIds.value,
        }
      } finally {
        loading.value = false
        fetchPromise = null
      }
    })()

    return fetchPromise
  }

  function clearState() {
    agents.value = []
    disabledOwnAgentIds.value = []
    error.value = null
    fetchedAt = 0
  }

  return {
    agents,
    disabledOwnAgentIds,
    loading,
    error,
    fetchAgents,
    invalidateAgentsCache,
    clearState,
  }
})

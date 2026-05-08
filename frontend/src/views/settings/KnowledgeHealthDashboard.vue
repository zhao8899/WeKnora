<template>
  <div class="knowledge-health-dashboard">
    <div class="section-header">
      <div>
        <h2>{{ $t('settings.health.title') }}</h2>
        <p class="section-description">{{ $t('settings.health.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <t-button theme="primary" variant="outline" @click="loadAll">
          {{ $t('settings.health.refresh') }}
        </t-button>
      </div>
    </div>

    <div class="filters-panel">
      <div class="filters-grid">
        <label class="field">
          <span>{{ dimensionLabel }}</span>
          <t-select v-model="filters.dimension" :options="dimensionOptions" />
        </label>
        <label class="field">
          <span>{{ sessionLabel }}</span>
          <t-input v-model="filters.sessionId" :placeholder="sessionPlaceholder" clearable />
        </label>
        <label class="field">
          <span>{{ knowledgeLabel }}</span>
          <t-input v-model="filters.knowledgeId" :placeholder="knowledgePlaceholder" clearable />
        </label>
        <label class="field">
          <span>{{ keywordLabel }}</span>
          <t-input v-model="filters.keyword" :placeholder="keywordPlaceholder" clearable />
        </label>
        <label class="field">
          <span>{{ limitLabel }}</span>
          <t-select v-model="filters.limit" :options="limitOptions" />
        </label>
        <label class="field">
          <span>{{ draftKbLabel }}</span>
          <t-select v-model="draftKnowledgeBaseId" :options="draftKnowledgeBaseOptions" clearable />
        </label>
      </div>
      <div class="filters-actions">
        <t-button variant="outline" @click="resetFilters">
          {{ resetText }}
        </t-button>
      </div>
    </div>

    <div class="dashboard-grid">
      <section class="panel">
        <div class="panel-header">
          <h3>{{ pendingKnowledgeTitle }}</h3>
        </div>
        <div v-if="loading" class="panel-loading">
          <t-loading size="small" />
        </div>
        <div v-else-if="pendingKnowledgeQuestions.length === 0" class="panel-empty">
          {{ $t('settings.health.empty') }}
        </div>
        <div v-else-if="filteredPendingKnowledgeQuestions.length === 0" class="panel-empty">
          {{ noFilterResultText }}
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.question') }}</th>
                <th>{{ $t('settings.health.status') }}</th>
                <th>{{ actionText }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in filteredPendingKnowledgeQuestions" :key="item.message_id">
                <td class="question-cell">{{ item.question || item.message_id }}</td>
                <td>
                  <t-tag theme="warning" variant="light">
                    {{ item.reason || pendingStatusText }}
                  </t-tag>
                </td>
                <td class="action-cell">
                  <t-button variant="text" size="small" @click="openChat(item.session_id)">
                    {{ openChatText }}
                  </t-button>
                  <t-button variant="text" size="small" @click="copyQuestion(item.question)">
                    {{ copyQuestionText }}
                  </t-button>
                  <t-button variant="text" size="small" :loading="draftCreatingMap[item.message_id] === true" @click="createDraft(item)">
                    {{ createDraftText }}
                  </t-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="panel">
        <div class="panel-header">
          <h3>{{ $t('settings.health.hotQuestions') }}</h3>
        </div>
        <div v-if="loading" class="panel-loading">
          <t-loading size="small" />
        </div>
        <div v-else-if="hotQuestions.length === 0" class="panel-empty">
          {{ $t('settings.health.empty') }}
        </div>
        <div v-else-if="filteredHotQuestions.length === 0" class="panel-empty">
          {{ noFilterResultText }}
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.question') }}</th>
                <th>{{ $t('settings.health.retrievedCount') }}</th>
                <th>{{ $t('settings.health.rerankedCount') }}</th>
                <th>{{ $t('settings.health.citedCount') }}</th>
                <th>{{ actionText }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in filteredHotQuestions" :key="item.message_id">
                <td class="question-cell">{{ item.question || item.message_id }}</td>
                <td>{{ item.retrieved_count }}</td>
                <td>{{ item.reranked_count }}</td>
                <td>{{ item.cited_count }}</td>
                <td>
                  <t-button variant="text" size="small" @click="openChat(item.session_id)">
                    {{ openChatText }}
                  </t-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="panel">
        <div class="panel-header">
          <h3>{{ $t('settings.health.coverageGaps') }}</h3>
        </div>
        <div v-if="loading" class="panel-loading">
          <t-loading size="small" />
        </div>
        <div v-else-if="coverageGaps.length === 0" class="panel-empty">
          {{ $t('settings.health.empty') }}
        </div>
        <div v-else-if="filteredCoverageGaps.length === 0" class="panel-empty">
          {{ noFilterResultText }}
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.question') }}</th>
                <th>{{ evidenceStrengthTitle }}</th>
                <th>{{ sourceHealthTitle }}</th>
                <th>{{ $t('settings.health.sources') }}</th>
                <th>{{ actionText }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in filteredCoverageGaps" :key="item.message_id">
                <td class="question-cell">{{ item.question || item.message_id }}</td>
                <td>
                  <t-tag
                    :theme="['low', 'insufficient'].includes(item.evidence_strength_label || item.confidence_label) ? 'danger' : 'warning'"
                    variant="light"
                  >
                    {{ formatPercent(item.evidence_strength_score ?? item.confidence_score) }}
                  </t-tag>
                </td>
                <td>
                  <t-tag
                    :theme="['low', 'insufficient'].includes(item.source_health_label) ? 'danger' : 'warning'"
                    variant="light"
                  >
                    {{ formatPercent(item.source_health_score) }}
                  </t-tag>
                </td>
                <td>{{ item.source_count }}</td>
                <td>
                  <t-button variant="text" size="small" @click="openChat(item.session_id)">
                    {{ openChatText }}
                  </t-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="panel">
        <div class="panel-header">
          <h3>{{ $t('settings.health.staleDocuments') }}</h3>
        </div>
        <div v-if="loading" class="panel-loading">
          <t-loading size="small" />
        </div>
        <div v-else-if="staleDocuments.length === 0" class="panel-empty">
          {{ $t('settings.health.empty') }}
        </div>
        <div v-else-if="filteredStaleDocuments.length === 0" class="panel-empty">
          {{ noFilterResultText }}
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.document') }}</th>
                <th>{{ sourceHealthTitle }}</th>
                <th>{{ $t('settings.health.downFeedback') }}</th>
                <th>{{ $t('settings.health.status') }}</th>
                <th>{{ actionText }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in filteredStaleDocuments" :key="item.knowledge_id">
                <td class="question-cell">{{ item.title || item.knowledge_id }}</td>
                <td>{{ formatPercent(item.source_health_score) }}</td>
                <td>{{ item.down_feedback_count + (item.expired_feedback_count || 0) }}</td>
                <td>
                  <t-tag :theme="statusTheme(item.health_status)" variant="light">
                    {{ statusText(item.health_status) }}
                  </t-tag>
                </td>
                <td>
                  <t-button variant="text" size="small" @click="openKnowledgeSearch(item.knowledge_id, item.title)">
                    {{ inspectText }}
                  </t-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="panel">
        <div class="panel-header">
          <h3>{{ $t('settings.health.citationHeatmap') }}</h3>
        </div>
        <div v-if="loading" class="panel-loading">
          <t-loading size="small" />
        </div>
        <div v-else-if="citationHeatmap.length === 0" class="panel-empty">
          {{ $t('settings.health.empty') }}
        </div>
        <div v-else-if="filteredCitationHeatmap.length === 0" class="panel-empty">
          {{ noFilterResultText }}
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.document') }}</th>
                <th>{{ $t('settings.health.retrievedCount') }}</th>
                <th>{{ $t('settings.health.rerankedCount') }}</th>
                <th>{{ $t('settings.health.citedCount') }}</th>
                <th>{{ sourceHealthTitle }}</th>
                <th>{{ actionText }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in filteredCitationHeatmap" :key="item.knowledge_id">
                <td class="question-cell">{{ item.title || item.knowledge_id }}</td>
                <td>{{ item.retrieved_count }}</td>
                <td>{{ item.reranked_count }}</td>
                <td>{{ item.cited_count }}</td>
                <td>
                  <t-tag :theme="statusTheme(item.health_status)" variant="light">
                    {{ formatPercent(item.source_health_score) }}
                  </t-tag>
                </td>
                <td>
                  <t-button variant="text" size="small" @click="openKnowledgeSearch(item.knowledge_id, item.title)">
                    {{ inspectText }}
                  </t-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next/es/message'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { createManualKnowledge, listKnowledgeBases } from '@/api/knowledge-base'
import { useUIStore } from '@/stores/ui'
import {
  getCitationHeatmap,
  getCoverageGaps,
  getHotQuestions,
  getPendingKnowledgeQuestions,
  getStaleDocuments,
  type CitationHeat,
  type CoverageGap,
  type HotQuestion,
  type PendingKnowledgeQuestion,
  type StaleDocument,
} from '@/api/analytics'

const { t } = useI18n()
const router = useRouter()
const uiStore = useUIStore()
const loading = ref(false)
const hotQuestions = ref<HotQuestion[]>([])
const coverageGaps = ref<CoverageGap[]>([])
const staleDocuments = ref<StaleDocument[]>([])
const citationHeatmap = ref<CitationHeat[]>([])
const pendingKnowledgeQuestions = ref<PendingKnowledgeQuestion[]>([])
const draftKnowledgeBaseId = ref('')
const draftKnowledgeBaseOptions = ref<Array<{ label: string; value: string }>>([])
const draftCreatingMap = reactive<Record<string, boolean>>({})
const filters = reactive({
  dimension: 'all',
  sessionId: '',
  knowledgeId: '',
  keyword: '',
  limit: 10,
})

const formatPercent = (value: number) => `${Math.round((value || 0) * 100)}%`
const i18nText = (key: string, fallback: string) => {
  const translated = t(key)
  return translated === key ? fallback : translated
}
const dimensionLabel = i18nText('settings.health.dimension', 'Dimension')
const sessionLabel = i18nText('settings.health.session', 'Session')
const knowledgeLabel = i18nText('settings.health.knowledge', 'Knowledge')
const keywordLabel = i18nText('settings.health.keyword', 'Keyword')
const limitLabel = i18nText('settings.health.limit', 'Rows')
const draftKbLabel = i18nText('settings.health.draftKnowledgeBase', 'Draft KB')
const sessionPlaceholder = i18nText('settings.health.sessionPlaceholder', 'Filter by session ID')
const knowledgePlaceholder = i18nText('settings.health.knowledgePlaceholder', 'Filter by knowledge ID')
const keywordPlaceholder = i18nText('settings.health.keywordPlaceholder', 'Filter by question or title')
const noFilterResultText = i18nText('settings.health.noFilterResult', 'No rows match current filters')
const actionText = i18nText('settings.health.action', 'Action')
const pendingKnowledgeTitle = i18nText('settings.health.pendingKnowledgeQuestions', 'Pending Knowledge Questions')
const pendingStatusText = i18nText('settings.health.pendingKnowledgeStatus', 'Need knowledge update')
const openChatText = i18nText('settings.health.openChat', 'Open Chat')
const copyQuestionText = i18nText('settings.health.copyQuestion', 'Copy Question')
const copySuccessText = i18nText('settings.health.copySuccess', 'Question copied')
const createDraftText = i18nText('settings.health.createDraft', 'Create Draft')
const draftNeedKbText = i18nText('settings.health.selectKnowledgeBaseFirst', 'Select a target knowledge base first')
const draftCreatedText = i18nText('settings.health.draftCreated', 'Draft created')
const draftCreateFailedText = i18nText('settings.health.draftCreateFailed', 'Failed to create draft')
const inspectText = i18nText('settings.health.inspect', 'Inspect')
const resetText = i18nText('common.reset', 'Reset')
const dimensionOptions = [
  { label: i18nText('settings.health.dimensionAll', 'All'), value: 'all' },
  { label: i18nText('settings.health.dimensionSession', 'Session'), value: 'session' },
  { label: i18nText('settings.health.dimensionKnowledge', 'Knowledge'), value: 'knowledge' },
]
const limitOptions = [10, 20, 50, 100].map(item => ({ label: `${item}`, value: item }))
const evidenceStrengthTitle = i18nText('chat.evidenceStrength', 'Evidence strength')
const sourceHealthTitle = i18nText('chat.sourceHealth', 'Source health')
const statusTheme = (status?: string) => {
  if (status === 'stale') return 'warning'
  if (status === 'at_risk') return 'danger'
  return 'success'
}
const statusText = (status?: string) => {
  if (status === 'stale') return i18nText('settings.health.needsReview', 'Needs Review')
  if (status === 'at_risk') return i18nText('settings.health.atRisk', 'At Risk')
  return i18nText('settings.health.healthy', 'Healthy')
}
const normalize = (value?: string) => (value || '').trim().toLowerCase()
const matchesKeyword = (valueA?: string, valueB?: string) => {
  if (!filters.keyword.trim()) return true
  const keyword = normalize(filters.keyword)
  return normalize(valueA).includes(keyword) || normalize(valueB).includes(keyword)
}
const matchesSession = (sessionId?: string) => {
  if (filters.dimension === 'knowledge') return false
  if (!filters.sessionId.trim()) return true
  return normalize(sessionId).includes(normalize(filters.sessionId))
}
const matchesKnowledge = (knowledgeId?: string) => {
  if (filters.dimension === 'session') return false
  if (!filters.knowledgeId.trim()) return true
  return normalize(knowledgeId).includes(normalize(filters.knowledgeId))
}
const pickList = (payload: any): any[] => {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.data)) return payload.data
  if (Array.isArray(payload?.data?.data)) return payload.data.data
  if (Array.isArray(payload?.data?.list)) return payload.data.list
  if (Array.isArray(payload?.data?.items)) return payload.data.items
  if (Array.isArray(payload?.data?.records)) return payload.data.records
  if (Array.isArray(payload?.data?.rows)) return payload.data.rows
  if (Array.isArray(payload?.data?.results)) return payload.data.results
  if (Array.isArray(payload?.data?.knowledge_bases)) return payload.data.knowledge_bases
  if (Array.isArray(payload?.list)) return payload.list
  if (Array.isArray(payload?.items)) return payload.items
  if (Array.isArray(payload?.records)) return payload.records
  if (Array.isArray(payload?.rows)) return payload.rows
  if (Array.isArray(payload?.results)) return payload.results
  if (Array.isArray(payload?.knowledge_bases)) return payload.knowledge_bases
  return []
}
const pickKnowledgeId = (payload: any): string => {
  const id =
    payload?.data?.id ||
    payload?.data?.knowledge_id ||
    payload?.data?.knowledgeId ||
    payload?.data?.knowledge?.id ||
    payload?.data?.item?.id ||
    payload?.id ||
    payload?.knowledge_id ||
    payload?.knowledgeId
  return id ? String(id) : ''
}

const filteredHotQuestions = computed(() =>
  hotQuestions.value.filter(item => matchesSession(item.session_id) && matchesKeyword(item.question, item.message_id)),
)
const filteredPendingKnowledgeQuestions = computed(() =>
  pendingKnowledgeQuestions.value.filter(
    item => matchesSession(item.session_id) && matchesKeyword(item.question, item.message_id),
  ),
)
const filteredCoverageGaps = computed(() =>
  coverageGaps.value.filter(item => matchesSession(item.session_id) && matchesKeyword(item.question, item.message_id)),
)
const filteredStaleDocuments = computed(() =>
  staleDocuments.value.filter(
    item => matchesKnowledge(item.knowledge_id) && matchesKeyword(item.title, item.knowledge_id),
  ),
)
const filteredCitationHeatmap = computed(() =>
  citationHeatmap.value.filter(
    item => matchesKnowledge(item.knowledge_id) && matchesKeyword(item.title, item.knowledge_id),
  ),
)

const resetFilters = () => {
  filters.dimension = 'all'
  filters.sessionId = ''
  filters.knowledgeId = ''
  filters.keyword = ''
  filters.limit = 10
  loadAll()
}

const openChat = (chatId?: string) => {
  if (!chatId) return
  router.push(`/platform/chat/${chatId}`)
}

const copyQuestion = async (question?: string) => {
  const text = (question || '').trim()
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    MessagePlugin.success(copySuccessText)
  } catch (error) {
    console.error('[KnowledgeHealthDashboard] Failed to copy question', error)
    MessagePlugin.error(t('common.operationFailed'))
  }
}

const createDraft = async (item: PendingKnowledgeQuestion) => {
  const kbId = draftKnowledgeBaseId.value
  if (!kbId) {
    MessagePlugin.warning(draftNeedKbText)
    return
  }

  const question = (item.question || '').trim()
  if (!question) return

  const title = question.length > 48 ? `${question.slice(0, 48)}...` : question
  const content = `## Pending Question\n\n${question}\n\n## Suggested Answer\n\n- \n\n## Sources To Add\n\n- `
  draftCreatingMap[item.message_id] = true
  try {
    const res = await createManualKnowledge(kbId, {
      title,
      content,
      status: 'draft',
    })
    const knowledgeId = pickKnowledgeId(res)
    if (knowledgeId) {
      uiStore.openManualEditor({
        mode: 'edit',
        kbId,
        knowledgeId,
      })
    } else {
      MessagePlugin.success(draftCreatedText)
    }
  } catch (error) {
    console.error('[KnowledgeHealthDashboard] Failed to create draft knowledge', error)
    MessagePlugin.error(draftCreateFailedText)
  } finally {
    draftCreatingMap[item.message_id] = false
  }
}

const openKnowledgeSearch = (knowledgeId?: string, title?: string) => {
  const keyword = (knowledgeId || title || '').trim()
  if (!keyword) {
    router.push('/platform/knowledge-search')
    return
  }
  router.push({ path: '/platform/knowledge-search', query: { keyword } })
}

const buildPendingKnowledgeFallback = (hot: HotQuestion[], gaps: CoverageGap[]) => {
  const records = new Map<string, PendingKnowledgeQuestion>()
  gaps.forEach(item => {
    const weakEvidence =
      (item.evidence_strength_score ?? item.confidence_score ?? 0) < 0.6 ||
      ['low', 'insufficient'].includes(item.evidence_strength_label || item.confidence_label || '') ||
      ['low', 'insufficient'].includes(item.source_health_label || '')
    if (!weakEvidence) return
    records.set(item.message_id, {
      message_id: item.message_id,
      session_id: item.session_id,
      question: item.question,
      reason: pendingStatusText,
      priority: 'high',
      created_at: item.answer_created_at,
    })
  })

  hot.forEach(item => {
    const weakCitation = item.cited_count === 0 || item.reranked_count === 0
    if (!weakCitation || records.has(item.message_id)) return
    records.set(item.message_id, {
      message_id: item.message_id,
      session_id: item.session_id,
      question: item.question,
      reason: pendingStatusText,
      priority: 'medium',
      created_at: item.last_access_at,
    })
  })

  return Array.from(records.values()).slice(0, filters.limit)
}

const loadAll = async () => {
  loading.value = true
  try {
    const [hotRes, gapRes, staleRes, citationRes, pendingRes] = await Promise.all([
      getHotQuestions({ limit: filters.limit }),
      getCoverageGaps({ limit: filters.limit }),
      getStaleDocuments({ limit: filters.limit }),
      getCitationHeatmap({ limit: filters.limit }),
      getPendingKnowledgeQuestions({ limit: filters.limit }).catch(() => null),
    ])

    hotQuestions.value = hotRes?.data || []
    coverageGaps.value = gapRes?.data || []
    staleDocuments.value = staleRes?.data || []
    citationHeatmap.value = citationRes?.data || []
    pendingKnowledgeQuestions.value =
      pendingRes?.data?.length > 0
        ? pendingRes.data
        : buildPendingKnowledgeFallback(hotQuestions.value, coverageGaps.value)
  } catch (error) {
    console.error('[KnowledgeHealthDashboard] Failed to load analytics data', error)
    MessagePlugin.error(t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

const loadDraftKnowledgeBaseOptions = async () => {
  try {
    const res = await listKnowledgeBases()
    const list = pickList(res)
    draftKnowledgeBaseOptions.value = list
      .map((kb: any) => ({
        label: kb?.name || kb?.id,
        value: kb?.id,
      }))
      .filter((option) => option.value)
    if (!draftKnowledgeBaseId.value && draftKnowledgeBaseOptions.value.length > 0) {
      draftKnowledgeBaseId.value = draftKnowledgeBaseOptions.value[0].value
    }
  } catch (error) {
    console.error('[KnowledgeHealthDashboard] Failed to load knowledge base options', error)
  }
}

onMounted(() => {
  loadDraftKnowledgeBaseOptions()
  loadAll()
})
</script>

<style lang="less" scoped>
.knowledge-health-dashboard {
  width: 100%;
}

.section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 24px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px;
  }
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.section-description {
  font-size: 14px;
  color: var(--td-text-color-secondary);
  margin: 0;
  line-height: 1.5;
}

.filters-panel {
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
  padding: 14px 16px;
  margin-bottom: 16px;
}

.filters-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;

  span {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }
}

.filters-actions {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.panel {
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(7, 192, 95, 0.04), rgba(7, 192, 95, 0.01));
  overflow: hidden;
}

.panel-header {
  padding: 16px 18px 12px;
  border-bottom: 1px solid var(--td-component-stroke);

  h3 {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.panel-loading,
.panel-empty {
  min-height: 180px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
}

.table-wrap {
  padding: 8px 12px 12px;
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;

  th,
  td {
    text-align: left;
    padding: 12px;
    border-bottom: 1px solid var(--td-component-stroke);
    font-size: 13px;
    color: var(--td-text-color-secondary);
    vertical-align: top;
  }

  th {
    font-size: 12px;
    font-weight: 600;
    color: var(--td-text-color-placeholder);
    white-space: nowrap;
  }

  tbody tr:last-child td {
    border-bottom: none;
  }
}

.question-cell {
  min-width: 180px;
  color: var(--td-text-color-primary);
  line-height: 1.5;
  word-break: break-word;
}

.action-cell {
  white-space: nowrap;
}


@media (max-width: 1024px) {
  .filters-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .dashboard-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .filters-grid {
    grid-template-columns: 1fr;
  }

  .section-header {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>

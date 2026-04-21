<template>
  <div class="knowledge-health-dashboard">
    <div class="section-header">
      <div>
        <h2>{{ $t('settings.health.title') }}</h2>
        <p class="section-description">{{ $t('settings.health.subtitle') }}</p>
      </div>
      <t-button theme="primary" variant="outline" @click="loadAll">
        {{ $t('settings.health.refresh') }}
      </t-button>
    </div>

    <div class="dashboard-grid">
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
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.question') }}</th>
                <th>{{ $t('settings.health.retrievedCount') }}</th>
                <th>{{ $t('settings.health.rerankedCount') }}</th>
                <th>{{ $t('settings.health.citedCount') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in hotQuestions" :key="item.message_id">
                <td class="question-cell">{{ item.question || item.message_id }}</td>
                <td>{{ item.retrieved_count }}</td>
                <td>{{ item.reranked_count }}</td>
                <td>{{ item.cited_count }}</td>
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
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.question') }}</th>
                <th>{{ $t('settings.health.confidence') }}</th>
                <th>{{ $t('settings.health.sources') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in coverageGaps" :key="item.message_id">
                <td class="question-cell">{{ item.question || item.message_id }}</td>
                <td>
                  <t-tag
                    :theme="['low', 'insufficient'].includes(item.confidence_label) ? 'danger' : 'warning'"
                    variant="light"
                  >
                    {{ formatPercent(item.confidence_score) }}
                  </t-tag>
                </td>
                <td>{{ item.source_count }}</td>
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
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.document') }}</th>
                <th>{{ $t('settings.health.sourceWeight') }}</th>
                <th>{{ $t('settings.health.downFeedback') }}</th>
                <th>{{ $t('settings.health.status') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in staleDocuments" :key="item.knowledge_id">
                <td class="question-cell">{{ item.title || item.knowledge_id }}</td>
                <td>{{ formatPercent(item.source_weight) }}</td>
                <td>{{ item.down_feedback_count }}</td>
                <td>
                  <t-tag v-if="item.freshness_flag" theme="warning" variant="light">
                    {{ $t('settings.health.needsReview') }}
                  </t-tag>
                  <t-tag v-else theme="success" variant="light">
                    {{ $t('settings.health.healthy') }}
                  </t-tag>
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
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ $t('settings.health.document') }}</th>
                <th>{{ $t('settings.health.retrievedCount') }}</th>
                <th>{{ $t('settings.health.rerankedCount') }}</th>
                <th>{{ $t('settings.health.citedCount') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in citationHeatmap" :key="item.knowledge_id">
                <td class="question-cell">{{ item.title || item.knowledge_id }}</td>
                <td>{{ item.retrieved_count }}</td>
                <td>{{ item.reranked_count }}</td>
                <td>{{ item.cited_count }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  getCitationHeatmap,
  getCoverageGaps,
  getHotQuestions,
  getStaleDocuments,
  type CitationHeat,
  type CoverageGap,
  type HotQuestion,
  type StaleDocument,
} from '@/api/analytics'

const { t } = useI18n()
const loading = ref(false)
const hotQuestions = ref<HotQuestion[]>([])
const coverageGaps = ref<CoverageGap[]>([])
const staleDocuments = ref<StaleDocument[]>([])
const citationHeatmap = ref<CitationHeat[]>([])

const formatPercent = (value: number) => `${Math.round((value || 0) * 100)}%`

const loadAll = async () => {
  loading.value = true
  try {
    const [hotRes, gapRes, staleRes, citationRes] = await Promise.all([
      getHotQuestions(),
      getCoverageGaps(),
      getStaleDocuments(),
      getCitationHeatmap(),
    ])

    hotQuestions.value = hotRes?.data || []
    coverageGaps.value = gapRes?.data || []
    staleDocuments.value = staleRes?.data || []
    citationHeatmap.value = citationRes?.data || []
  } catch (error) {
    console.error('[KnowledgeHealthDashboard] Failed to load analytics data', error)
    MessagePlugin.error(t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
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

.section-description {
  font-size: 14px;
  color: var(--td-text-color-secondary);
  margin: 0;
  line-height: 1.5;
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


@media (max-width: 1024px) {
  .dashboard-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .section-header {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>

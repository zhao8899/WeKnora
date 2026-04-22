<template>
  <div v-if="shouldRender" class="confidence-panel">
    <div class="confidence-header" @click="expanded = !expanded">
      <div class="confidence-header-left">
        <span class="confidence-badge" :class="`confidence-${confidenceLabel}`">
          {{ confidenceTagText }}
        </span>
        <span class="confidence-title">{{ evidenceStrengthText }}</span>
        <span class="confidence-score">{{ confidencePercent }}</span>
        <span class="confidence-meta">{{ confidenceMetaText }}</span>
      </div>
      <t-icon :name="expanded ? 'chevron-up' : 'chevron-down'" />
    </div>

    <div v-if="expanded" class="confidence-body">
      <div class="confidence-metrics">
        <div class="metric-card">
          <span class="metric-label">{{ evidenceStrengthText }}</span>
          <strong>{{ confidencePercent }}</strong>
          <span class="metric-tag confidence-badge" :class="`confidence-${confidenceLabel}`">{{ confidenceTagText }}</span>
        </div>
        <div class="metric-card">
          <span class="metric-label">{{ sourceHealthText }}</span>
          <strong>{{ sourceHealthPercent }}</strong>
          <span class="metric-tag confidence-badge" :class="`confidence-${sourceHealthLabel}`">{{ sourceHealthTagText }}</span>
        </div>
      </div>

      <div class="confidence-summary">
        <div
          v-for="(count, key) in sourceTypeCounts"
          :key="key"
          class="summary-chip"
        >
          <span>{{ sourceTypeLabel(key) }}</span>
          <strong>{{ count }}</strong>
        </div>
      </div>

      <div v-if="loading" class="confidence-state">
        <t-loading size="small" />
        <span>{{ $t('common.loading') }}</span>
      </div>

      <div v-else-if="items.length === 0" class="confidence-state">
        <t-icon name="info-circle" />
        <span>{{ noEvidenceText }}</span>
      </div>

      <div v-else class="confidence-list">
        <div v-for="item in items" :key="item.id" class="confidence-item">
          <div class="confidence-item-main">
            <div class="confidence-item-title-row">
              <span class="confidence-item-title">{{ item.title || item.knowledge_id }}</span>
              <span class="confidence-item-position">#{{ item.position }}</span>
            </div>
            <div class="confidence-item-meta">
              <span>{{ sourceTypeLabel(item.source_type) }}</span>
              <span>{{ matchTypeLabel(item.match_type) }}</span>
              <span v-if="item.source_channel">{{ channelLabel(item.source_channel) }}</span>
              <span>{{ $t('chat.retrievalScore') }} {{ formatScore(item.retrieval_score) }}</span>
            </div>
          </div>
          <div class="confidence-item-actions">
            <t-button
              size="small"
              variant="outline"
              shape="round"
              :class="{ 'source-feedback-active-up': feedbackMap[item.id] === 'up' }"
              @click="submitFeedback(item, 'up')"
            >
              <t-icon name="thumb-up" />
            </t-button>
            <t-button
              size="small"
              variant="outline"
              shape="round"
              :class="{ 'source-feedback-active-down': feedbackMap[item.id] === 'down' }"
              @click="submitFeedback(item, 'down')"
            >
              <t-icon name="thumb-down" />
            </t-button>
            <t-tooltip :content="$t('chat.feedbackExpiredHint')" placement="top">
              <t-button
                size="small"
                variant="outline"
                shape="round"
                :class="{ 'source-feedback-active-expired': feedbackMap[item.id] === 'expired' }"
                @click="submitFeedback(item, 'expired')"
              >
                <t-icon name="time" />
              </t-button>
            </t-tooltip>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import { getAnswerConfidence, submitSourceFeedback } from '@/api/chat/index';

const props = defineProps({
  messageId: {
    type: String,
    required: true,
  },
  isCompleted: {
    type: Boolean,
    default: false,
  },
  referenceCount: {
    type: Number,
    default: 0,
  },
});

const { t, locale } = useI18n();
const expanded = ref(false);
const loading = ref(false);
const loaded = ref(false);
const confidenceScore = ref(0);
const confidenceLabel = ref('low');
const sourceHealthScore = ref(0);
const sourceHealthLabel = ref('low');
const sourceCount = ref(0);
const referenceCount = ref(0);
const evidenceStatus = ref('missing');
const sourceTypeCounts = ref<Record<string, number>>({});
const items = ref<any[]>([]);
const feedbackMap = reactive<Record<string, string>>({});

const shouldRender = computed(() => props.isCompleted && !!props.messageId && props.referenceCount > 0);
const confidencePercent = computed(() => `${Math.round((confidenceScore.value || 0) * 100)}%`);
const sourceHealthPercent = computed(() => `${Math.round((sourceHealthScore.value || 0) * 100)}%`);
const fallbackText = (key: string, params: Record<string, any>, zh: string, en: string) => {
  const translated = t(key, params);
  if (translated !== key) return translated;
  return locale.value.startsWith('zh') ? zh : en;
};
const confidenceMetaText = computed(() => {
  if (evidenceStatus.value === 'missing') {
    const count = Math.max(referenceCount.value, props.referenceCount);
    return fallbackText('chat.referencesCount', { count }, `${count} 个引用`, `${count} references`);
  }
  if (evidenceStatus.value === 'degraded') {
    const count = sourceCount.value;
    return fallbackText('chat.evidenceDegraded', { count }, `${count} 个临时来源`, `${count} transient sources`);
  }
  if (evidenceStatus.value === 'recovered') {
    const count = sourceCount.value;
    return fallbackText('chat.evidenceRecovered', { count }, `已从引用恢复 ${count} 个来源`, `${count} sources recovered from references`);
  }
  return t('chat.sourcesCount', { count: sourceCount.value });
});
const evidenceStrengthText = computed(() => fallbackText('chat.evidenceStrength', {}, '证据强度', 'Evidence strength'));
const sourceHealthText = computed(() => fallbackText('chat.sourceHealth', {}, '来源健康度', 'Source health'));
const noEvidenceText = computed(() => {
  if (evidenceStatus.value === 'missing') {
    return fallbackText(
      'chat.noConfidenceEvidenceYet',
      {},
      '已生成引用，但证据明细仍在生成中',
      'References are available, but evidence details are still being generated'
    );
  }
  if (evidenceStatus.value === 'degraded') {
    return fallbackText(
      'chat.noConfidenceEvidenceDegraded',
      {},
      '证据明细暂不可用，当前展示为降级视图',
      'Evidence details are temporarily unavailable; showing a degraded confidence view'
    );
  }
  return t('chat.noConfidenceEvidence');
});
const confidenceTagText = computed(() => {
  if (confidenceLabel.value === 'high') return t('chat.confidenceHigh');
  if (confidenceLabel.value === 'medium') return t('chat.confidenceMedium');
  if (confidenceLabel.value === 'insufficient') return t('chat.confidenceInsufficient');
  return t('chat.confidenceLow');
});
const sourceHealthTagText = computed(() => {
  if (sourceHealthLabel.value === 'high') {
    return fallbackText('chat.sourceHealthHigh', {}, '来源健康', 'Healthy sources');
  }
  if (sourceHealthLabel.value === 'medium') {
    return fallbackText('chat.sourceHealthMedium', {}, '来源一般', 'Mixed source health');
  }
  if (sourceHealthLabel.value === 'insufficient') {
    return fallbackText('chat.sourceHealthInsufficient', {}, '来源未知', 'Source health unknown');
  }
  return fallbackText('chat.sourceHealthLow', {}, '来源偏弱', 'Weak source health');
});

const fetchConfidence = async () => {
  if (!shouldRender.value || loaded.value || loading.value) return;
  loading.value = true;
  try {
    const res = await getAnswerConfidence(props.messageId);
    const data = res?.data || {};
    confidenceScore.value = data.evidence_strength_score ?? data.confidence_score ?? 0;
    confidenceLabel.value = data.evidence_strength_label || data.confidence_label || 'low';
    sourceHealthScore.value = data.source_health_score ?? 0;
    sourceHealthLabel.value = data.source_health_label || 'low';
    sourceCount.value = data.source_count || 0;
    referenceCount.value = data.reference_count || 0;
    evidenceStatus.value = data.evidence_status || 'missing';
    sourceTypeCounts.value = data.source_type_counts || {};
    items.value = data.evidences || [];
    items.value.forEach((item) => {
      feedbackMap[item.id] = item.current_feedback || '';
    });
    loaded.value = true;
  } catch (error) {
    console.error('[Confidence] Failed to fetch answer confidence', error);
    MessagePlugin.error(t('chat.confidenceLoadFailed'));
  } finally {
    loading.value = false;
  }
};

const submitFeedback = async (item: any, value: 'up' | 'down' | 'expired') => {
  if (!props.messageId || !item?.id) return;
  const previous = feedbackMap[item.id] || '';
  feedbackMap[item.id] = value;
  try {
    await submitSourceFeedback(props.messageId, item.id, value);
    MessagePlugin.success(t('chat.sourceFeedbackSuccess'));
  } catch (error) {
    feedbackMap[item.id] = previous;
    console.error('[Confidence] Failed to submit source feedback', error);
    MessagePlugin.error(t('chat.sourceFeedbackFailed'));
  }
};

const sourceTypeLabel = (value: string) => {
  if (value === 'faq') return t('chat.sourceTypeFaq');
  if (value === 'web') return t('chat.sourceTypeWeb');
  return t('chat.sourceTypeDocument');
};

const matchTypeLabel = (value: string) => {
  const labels: Record<string, string> = {
    vector: t('chat.matchTypeVector'),
    keyword: t('chat.matchTypeKeyword'),
    nearby: t('chat.matchTypeNearby'),
    history: t('chat.matchTypeHistory'),
    graph: t('chat.matchTypeGraph'),
    web_search: t('chat.matchTypeWebSearch'),
    data_analysis: t('chat.matchTypeDataAnalysis'),
  };
  return labels[value] || value;
};

const channelLabel = (value: string) => {
  if (value === 'api') return t('chat.channelApi');
  if (value === 'im') return t('chat.channelIm');
  return t('chat.channelWeb');
};

const formatScore = (value: number) => `${Math.round((value || 0) * 100)}%`;

watch(expanded, (value) => {
  if (value) {
    fetchConfidence();
  }
});

watch(
  () => props.messageId,
  () => {
    loaded.value = false;
    items.value = [];
    confidenceScore.value = 0;
    confidenceLabel.value = 'low';
    sourceHealthScore.value = 0;
    sourceHealthLabel.value = 'low';
    sourceCount.value = 0;
    referenceCount.value = 0;
    evidenceStatus.value = 'missing';
    sourceTypeCounts.value = {};
    Object.keys(feedbackMap).forEach((key) => {
      delete feedbackMap[key];
    });
  }
);
</script>

<style lang="less" scoped>
.confidence-panel {
  width: 100%;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(7, 192, 95, 0.04), rgba(7, 192, 95, 0.01));
  overflow: hidden;
}

.confidence-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  cursor: pointer;

  &:hover {
    background: rgba(7, 192, 95, 0.03);
  }
}

.confidence-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex-wrap: wrap;
}

.confidence-badge {
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;

  &.confidence-high {
    color: var(--td-success-color);
    background: rgba(0, 168, 112, 0.1);
  }

  &.confidence-medium {
    color: var(--td-warning-color);
    background: rgba(255, 155, 24, 0.1);
  }

  &.confidence-low {
    color: var(--td-error-color);
    background: rgba(229, 75, 75, 0.1);
  }

  &.confidence-insufficient {
    color: var(--td-text-color-placeholder);
    background: rgba(0, 0, 0, 0.06);
  }
}

.confidence-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.confidence-score {
  font-size: 12px;
  font-weight: 700;
  color: var(--td-brand-color);
}

.confidence-meta {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.confidence-body {
  padding: 0 14px 12px;
  border-top: 1px solid var(--td-bg-color-secondarycontainer);
}

.confidence-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 10px;
}

.confidence-metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  padding-top: 12px;
}

.metric-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--td-bg-color-container);
  border: 1px solid rgba(7, 192, 95, 0.08);
}

.metric-label {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.metric-card strong {
  font-size: 18px;
  color: var(--td-text-color-primary);
}

.metric-tag {
  width: fit-content;
}

.summary-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: 11px;
}

.confidence-state {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  padding-top: 12px;
}

.confidence-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: 12px;
}

.confidence-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--td-bg-color-container);
  border: 1px solid rgba(7, 192, 95, 0.08);
}

.confidence-item-main {
  min-width: 0;
  flex: 1;
}

.confidence-item-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.confidence-item-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.confidence-item-position {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.confidence-item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 6px;
  font-size: 11px;
  color: var(--td-text-color-secondary);
}

.confidence-item-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.source-feedback-active-up {
  color: var(--td-success-color) !important;
  border-color: var(--td-success-color) !important;
  background: rgba(0, 168, 112, 0.06) !important;
}

.source-feedback-active-down {
  color: var(--td-error-color) !important;
  border-color: var(--td-error-color) !important;
  background: rgba(229, 75, 75, 0.06) !important;
}

@media (max-width: 768px) {
  .confidence-metrics {
    grid-template-columns: 1fr;
  }
}
</style>

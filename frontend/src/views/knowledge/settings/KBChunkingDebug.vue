<template>
  <div class="kb-chunking-debug">
    <t-button
      type="button"
      theme="primary"
      variant="text"
      size="medium"
      class="debug-trigger"
      @click="open = true"
    >
      <template #icon><play-circle-icon /></template>
      {{ UI.title }}
    </t-button>

    <t-drawer
      v-model:visible="open"
      :header="UI.title"
      :footer="false"
      size="720px"
      placement="right"
      :close-on-overlay-click="true"
      :destroy-on-close="false"
      attach="body"
      :z-index="3000"
    >
      <div class="drawer-body">
        <section class="drawer-section">
          <div class="section-title-row">
            <div class="section-title">{{ UI.sampleLabel }}</div>
            <div class="sample-presets">
              <span class="presets-label">{{ UI.presetLabel }}</span>
              <t-button
                v-for="p in samples"
                :key="p.id"
                type="button"
                variant="text"
                size="small"
                class="preset-chip"
                @click="loadSample(p.id)"
              >
                {{ p.label }}
              </t-button>
            </div>
          </div>

          <t-textarea
            v-model="sample"
            :placeholder="UI.samplePlaceholder"
            :autosize="{ minRows: 6, maxRows: 12 }"
            :maxlength="MAX_CHARS"
          />

          <div class="input-actions">
            <t-button
              type="button"
              theme="primary"
              :loading="loading"
              :disabled="!sample || sample.length === 0"
              @click.prevent.stop="runPreview"
            >
              <template #icon><play-circle-icon /></template>
              {{ UI.runButton }}
            </t-button>
          </div>
        </section>

        <div v-if="loading" class="debug-loading">
          <t-loading size="small" />
          <span>{{ UI.loading }}</span>
        </div>

        <div v-else-if="error" class="debug-error">
          <error-circle-icon class="error-icon" />
          <div>
            <strong>{{ UI.errorPrefix }}</strong>
            <span>{{ error }}</span>
          </div>
        </div>

        <section v-else-if="result" class="drawer-section debug-result">
          <div class="result-header">
            <div class="tier-row">
              <span class="result-label">{{ UI.selectedTier }}:</span>
              <t-tag
                :theme="tierTheme(result.selected_tier)"
                variant="light-outline"
                size="medium"
              >
                {{ tierDisplay(result.selected_tier) }}
              </t-tag>
              <span v-if="fallbackWarning" class="fallback-warning">
                {{ UI.fallbackWarning }}
              </span>
            </div>

            <div v-if="(result.rejected || []).length > 0" class="tier-row">
              <span class="result-label">{{ UI.rejected }}:</span>
              <span class="rejection-list">
                <t-tag
                  v-for="r in (result.rejected || [])"
                  :key="r.tier"
                  theme="default"
                  variant="light"
                  size="small"
                >
                  {{ tierDisplay(r.tier) }}: {{ r.reason }}
                </t-tag>
              </span>
            </div>
          </div>

          <div class="profile-grid">
            <div class="profile-cell">
              <div class="cell-value">{{ result.profile.total_lines }}</div>
              <div class="cell-label">{{ UI.profile.lines }}</div>
            </div>
            <div class="profile-cell">
              <div class="cell-value">{{ result.profile.total_chars }}</div>
              <div class="cell-label">{{ UI.profile.chars }}</div>
            </div>
            <div class="profile-cell">
              <div class="cell-value">{{ result.profile.md_heading_total }}</div>
              <div class="cell-label">{{ UI.profile.headings }}</div>
            </div>
            <div class="profile-cell">
              <div class="cell-value">{{ result.profile.form_feed_count }}</div>
              <div class="cell-label">{{ UI.profile.pageBreaks }}</div>
            </div>
            <div class="profile-cell">
              <div class="cell-value">
                {{
                  result.profile.german_chapter_count +
                  result.profile.english_chapter_count +
                  result.profile.chinese_chapter_count
                }}
              </div>
              <div class="cell-label">{{ UI.profile.chapterMarkers }}</div>
            </div>
            <div class="profile-cell">
              <div class="cell-value">{{ (result.profile.detected_langs || []).join(', ') || '—' }}</div>
              <div class="cell-label">{{ UI.profile.languages }}</div>
            </div>
          </div>

          <div class="chunk-stats">
            <span class="stats-count">
              <strong>{{ result.stats.count }}</strong>
              {{ UI.stats.chunks }}
            </span>
            <span class="stats-sep">|</span>
            <span>avg {{ result.stats.avg_chars }}</span>
            <span class="stats-sep">|</span>
            <span>sd {{ result.stats.stddev_chars }}</span>
            <span class="stats-sep">|</span>
            <span>min {{ result.stats.min_chars }}</span>
            <span class="stats-sep">|</span>
            <span>max {{ result.stats.max_chars }}</span>
            <span v-if="result.stats.truncated_to" class="truncation-hint">
              {{ UI.stats.truncated(result.stats.truncated_to) }}
            </span>
          </div>

          <ol class="chunks-list">
            <li
              v-for="c in result.chunks"
              :key="c.seq"
              class="chunk-card"
              :class="{ expanded: expandedChunks.has(c.seq) }"
            >
              <button
                type="button"
                class="chunk-meta"
                :aria-expanded="expandedChunks.has(c.seq)"
                @click="toggleChunk(c.seq)"
              >
                <span class="chunk-seq">#{{ c.seq }}</span>
                <span class="chunk-size">
                  {{ c.size_chars }} {{ UI.characters }}
                  <span class="chunk-tokens">| ~{{ c.size_tokens_approx }} tok</span>
                </span>
                <span class="chunk-pos">{{ c.start }} - {{ c.end }}</span>
                <span v-if="c.context_header" class="chunk-context-pill" :title="c.context_header">
                  {{ c.context_header }}
                </span>
                <chevron-down-icon class="chunk-toggle" :class="{ open: expandedChunks.has(c.seq) }" />
              </button>
              <div class="chunk-body" :class="{ collapsed: !expandedChunks.has(c.seq) }">
                <pre class="chunk-text">{{ c.content }}</pre>
              </div>
            </li>
          </ol>
        </section>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  ChevronDownIcon,
  ErrorCircleIcon,
  PlayCircleIcon,
} from 'tdesign-icons-vue-next'
import { previewChunking } from '@/api/chunker'
import type { PreviewChunkingResponse, StrategyTier } from '@/types/chunker'
import { CHUNKING_SAMPLES, DEFAULT_SAMPLE_ID } from './chunkingSamples'

interface Props {
  config: {
    chunkSize: number
    chunkOverlap: number
    separators: string[]
    strategy?: string
    tokenLimit?: number
    languages?: string[]
  }
}

const props = defineProps<Props>()

const UI = {
  title: '分块预览',
  sampleLabel: '测试样例',
  presetLabel: '样例',
  samplePlaceholder: '粘贴一段文本，预览三层 chunking 如何切分。',
  runButton: '开始预览',
  loading: '正在分块...',
  errorPrefix: '预览失败',
  selectedTier: '命中的层级',
  fallbackWarning: '已回退到兜底层级',
  rejected: '被拒绝的层级',
  profile: {
    lines: '行数',
    chars: '字符数',
    headings: '标题数',
    pageBreaks: '分页符',
    chapterMarkers: '章节标记',
    languages: '识别语言',
  },
  stats: {
    chunks: '块',
    truncated: (total: number) => `已截断到 ${total} 条`,
  },
  characters: '字符',
} as const

// Mirrors handler.previewMaxChars on the backend. Keep in sync.
const MAX_CHARS = 64 * 1024

const open = ref(false)
const sample = ref('')
const loading = ref(false)
const error = ref('')
const result = ref<PreviewChunkingResponse | null>(null)
const expandedChunks = ref(new Set<number>())

const samples = CHUNKING_SAMPLES

watch(open, (isOpen) => {
  if (isOpen && sample.value.trim() === '') {
    loadSample(DEFAULT_SAMPLE_ID)
  }
})

const loadSample = (id: string) => {
  const preset = samples.find((s) => s.id === id)
  if (!preset) return
  sample.value = preset.text
  result.value = null
  error.value = ''
  expandedChunks.value = new Set()
}

const fallbackWarning = computed(() => {
  if (!result.value) return false
  return result.value.selected_tier === 'legacy' && (result.value.rejected || []).length > 0
})

const runPreview = async () => {
  loading.value = true
  error.value = ''
  result.value = null
  expandedChunks.value = new Set()
  try {
    const resp = await previewChunking({
      text: sample.value,
      chunking_config: {
        chunk_size: props.config.chunkSize,
        chunk_overlap: props.config.chunkOverlap,
        separators: props.config.separators,
        strategy: props.config.strategy ?? '',
        token_limit: props.config.tokenLimit ?? 0,
        languages: props.config.languages ?? []
      }
    })
    if (!resp) {
      throw new Error('empty response')
    }
    if (resp.success !== true) {
      throw new Error((resp as any).error || 'preview failed')
    }
    if (!resp.data) {
      throw new Error('response missing data')
    }
    result.value = resp.data
  } catch (e: any) {
    const msg = e?.message || (typeof e === 'string' ? e : '') || 'unknown error'
    error.value = msg
    console.error('[KBChunkingDebug] previewChunking failed:', e)
    MessagePlugin.error(`${UI.errorPrefix}: ${msg}`)
  } finally {
    loading.value = false
  }
}

const toggleChunk = (seq: number) => {
  const next = new Set(expandedChunks.value)
  if (next.has(seq)) next.delete(seq)
  else next.add(seq)
  expandedChunks.value = next
}

const tierDisplay = (tier: StrategyTier) => {
  switch (tier) {
    case 'heading':
      return '标题层级'
    case 'heuristic':
      return '启发式'
    case 'recursive':
      return '递归分隔'
    case 'legacy':
    default:
      return '兼容兜底'
  }
}

const tierTheme = (tier: StrategyTier) => {
  switch (tier) {
    case 'heading':
    case 'heuristic':
      return 'success'
    case 'recursive':
      return 'primary'
    case 'legacy':
    default:
      return 'default'
  }
}
</script>

<style lang="less" scoped>
.kb-chunking-debug {
  flex-shrink: 0;
}

.debug-trigger {
  --td-bg-color-container-hover: transparent;
  padding-left: 0;
  padding-right: 0;

  &:hover,
  &:focus,
  &.t-is-active,
  &:active {
    background-color: transparent !important;
    color: var(--td-brand-color-hover);
  }

  &:active {
    color: var(--td-brand-color-active);
  }
}

.drawer-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.drawer-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.section-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.sample-presets {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;

  .presets-label {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    margin-right: 4px;
  }

  .preset-chip {
    --td-comp-paddinglr-s: 8px;
    color: var(--td-text-color-secondary);
    font-size: 12px;

    &:hover {
      color: var(--td-brand-color);
    }
  }
}

.input-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 4px;
}

.debug-result {
  gap: 0;
}

.debug-loading {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  background: var(--td-bg-color-container-hover);
  border-radius: 6px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.debug-error {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  background: var(--td-error-color-light);
  border-radius: 6px;
  font-size: 13px;
  color: var(--td-error-color);

  .error-icon {
    flex-shrink: 0;
    margin-top: 2px;
    font-size: 16px;
  }

  strong {
    display: block;
    margin-bottom: 2px;
  }

  span {
    color: var(--td-error-color);
    font-weight: 400;
    word-break: break-word;
  }
}

.result-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px dashed var(--td-component-stroke);
}

.tier-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 13px;
}

.result-label {
  color: var(--td-text-color-secondary);
  font-weight: 500;
  min-width: 120px;
}

.rejection-list {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.fallback-warning {
  color: var(--td-warning-color);
  font-size: 12px;
}

.profile-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 1px;
  margin-bottom: 16px;
  background: var(--td-component-stroke);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
}

.profile-cell {
  padding: 12px 8px;
  text-align: center;
  background: var(--td-bg-color-container);
}

.cell-value {
  font-size: 18px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--td-text-color-primary);
  line-height: 1.2;
}

.cell-label {
  margin-top: 4px;
  font-size: 11px;
  color: var(--td-text-color-secondary);
}

.chunk-stats {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 12px;
  padding: 10px 14px;
  background: var(--td-bg-color-container-hover);
  border-radius: 6px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  font-variant-numeric: tabular-nums;

  .stats-count strong {
    margin-right: 4px;
    color: var(--td-text-color-primary);
    font-size: 14px;
    font-weight: 600;
  }

  .stats-sep {
    color: var(--td-text-color-placeholder);
  }
}

.truncation-hint {
  margin-left: auto;
  color: var(--td-warning-color);
  font-size: 12px;
}

.chunks-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.chunk-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  overflow: hidden;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &.expanded {
    border-color: var(--td-brand-color-light-active);
    box-shadow: 0 0 0 1px var(--td-brand-color-light) inset;
  }
}

.chunk-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  margin: 0;
  padding: 10px 14px;
  background: var(--td-bg-color-container-hover);
  border: none;
  cursor: pointer;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  text-align: left;

  &:hover {
    background: var(--td-bg-color-component-hover);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color-focus);
    outline-offset: -2px;
  }
}

.chunk-seq {
  flex-shrink: 0;
  color: var(--td-text-color-primary);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.chunk-size {
  flex-shrink: 0;
  color: var(--td-text-color-primary);
  font-variant-numeric: tabular-nums;

  .chunk-tokens {
    color: var(--td-text-color-secondary);
    font-weight: 400;
  }
}

.chunk-pos {
  flex-shrink: 0;
  color: var(--td-text-color-placeholder);
  font-family: var(--td-font-family-mono, ui-monospace, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace);
  font-variant-numeric: tabular-nums;
}

.chunk-context-pill {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 240px;
  padding: 2px 8px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  border-radius: 10px;
  font-size: 11px;
  font-family: var(--td-font-family-mono, ui-monospace, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chunk-toggle {
  margin-left: auto;
  flex-shrink: 0;
  font-size: 16px;
  color: var(--td-text-color-secondary);
  transition: transform 0.15s ease;

  &.open {
    transform: rotate(180deg);
  }
}

.chunk-body {
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}

.chunk-text {
  margin: 0;
  padding: 12px 14px;
  font-size: 12.5px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--td-text-color-primary);
  font-family: var(--td-font-family-mono, ui-monospace, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace);
}

.chunk-body.collapsed .chunk-text {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  position: relative;

  &::after {
    content: '';
    position: absolute;
    inset: auto 0 0 0;
    height: 28px;
    background: linear-gradient(
      to bottom,
      transparent,
      var(--td-bg-color-container)
    );
    pointer-events: none;
  }
}
</style>

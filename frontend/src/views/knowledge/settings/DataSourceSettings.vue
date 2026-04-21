<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  listDataSources,
  deleteDataSource,
  triggerSync,
  pauseDataSource,
  resumeDataSource,
  type DataSource,
} from '@/api/datasource'
import { humanizeCron, relativeTime } from '@/utils/cronHumanize'
import DataSourceEditorDialog from './DataSourceEditorDialog.vue'
import DataSourceSyncLogs from './DataSourceSyncLogs.vue'
import DataSourceTypeIcon from './DataSourceTypeIcon.vue'

const props = defineProps<{ kbId: string }>()
const emit = defineEmits<{ (e: 'count', value: number): void }>()
const { t } = useI18n()

const dataSources = ref<DataSource[]>([])
const loading = ref(false)
const editorVisible = ref(false)
const editingDs = ref<DataSource | null>(null)
const editorFocusHint = ref('')
const logsVisible = ref(false)
const logsDsId = ref('')
const logsDsName = ref('')
const pollTimer = ref<number | null>(null)
const expandedPreviewId = ref('')

function stopPolling() {
  if (pollTimer.value !== null) {
    window.clearTimeout(pollTimer.value)
    pollTimer.value = null
  }
}

function schedulePolling() {
  stopPolling()
  pollTimer.value = window.setTimeout(() => {
    loadList(true)
  }, 3000)
}

async function loadList(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await listDataSources(props.kbId)
    dataSources.value = res?.data || res || []
    emit('count', dataSources.value.length)

    const hasRunningSync = dataSources.value.some(ds => ds.latest_sync_log?.status === 'running')
    if (hasRunningSync) {
      schedulePolling()
    } else {
      stopPolling()
    }
  } catch (e: any) {
    console.error(e)
  } finally {
    if (!silent) loading.value = false
  }
}

function openCreate() {
  editingDs.value = null
  editorFocusHint.value = ''
  editorVisible.value = true
}

function resolveEditorFocusHint(ds: DataSource) {
  const key = remediationKey(ds)
  if (key === 'storage') return 'storage'
  if (key === 'feeds') return 'feed_urls'
  if (key === 'pages') return 'urls'
  if (key === 'auth' || key === 'config') return 'primary'
  if (key === 'format' && ds.type === 'rss') return 'feed_urls'
  if (key === 'format' && ds.type === 'web_crawler') return 'urls'
  return ''
}

function openEdit(ds: DataSource, focusHint = '') {
  editingDs.value = ds
  editorFocusHint.value = focusHint
  editorVisible.value = true
}

function openLogs(ds: DataSource) {
  logsDsId.value = ds.id
  logsDsName.value = ds.name
  logsVisible.value = true
}

function togglePreview(ds: DataSource) {
  expandedPreviewId.value = expandedPreviewId.value === ds.id ? '' : ds.id
}

function handleDelete(ds: DataSource) {
  const confirmDialog = DialogPlugin.confirm({
    header: t('datasource.delete'),
    body: t('datasource.deleteConfirm'),
    confirmBtn: { content: t('datasource.delete'), theme: 'danger' },
    cancelBtn: t('common.cancel'),
    onConfirm: async () => {
      try {
        await deleteDataSource(ds.id)
        MessagePlugin.success(t('datasource.deleteSuccess'))
        await loadList()
        confirmDialog.hide()
      } catch (e: any) {
        MessagePlugin.error(e?.message || e?.error || t('datasource.deleteFailed'))
      }
    },
  })
}

async function handleSync(ds: DataSource) {
  try {
    await triggerSync(ds.id)
    MessagePlugin.success(t('datasource.syncTriggered'))
    await loadList(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.syncFailed'))
  }
}

async function handlePause(ds: DataSource) {
  try {
    await pauseDataSource(ds.id)
    MessagePlugin.success(t('datasource.paused'))
    loadList()
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.pauseFailed'))
  }
}

async function handleResume(ds: DataSource) {
  try {
    await resumeDataSource(ds.id)
    MessagePlugin.success(t('datasource.resumed'))
    loadList()
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.resumeFailed'))
  }
}

function statusTheme(status: string): 'success' | 'danger' | 'default' | 'warning' {
  if (status === 'active') return 'success'
  if (status === 'error') return 'danger'
  if (status === 'paused') return 'warning'
  return 'default'
}

function statusLabel(status: string) {
  return t(`datasource.status.${status}`)
}

function syncModeLabel(mode: string) {
  return t(`datasource.syncMode.${mode}`)
}

function connectorLabel(type: string) {
  return t(`datasource.connector.${type}`) || type
}

function scheduleLabel(cron: string) {
  return humanizeCron(cron, t)
}

function lastSyncTime(ds: DataSource) {
  return relativeTime(ds.last_sync_at, t)
}

function lastSyncFullTime(ds: DataSource) {
  if (!ds.last_sync_at) return ''
  return new Date(ds.last_sync_at).toLocaleString()
}

function syncResultPills(ds: DataSource) {
  const log = ds.latest_sync_log
  if (!log) return []
  const pills: { text: string; cls: string }[] = []
  if (log.items_created > 0) pills.push({ text: `+${log.items_created}`, cls: 'created' })
  if (log.items_updated > 0) pills.push({ text: `~${log.items_updated}`, cls: 'updated' })
  if (log.items_deleted > 0) pills.push({ text: `-${log.items_deleted}`, cls: 'deleted' })
  if (log.items_failed > 0) pills.push({ text: `${log.items_failed} ${t('datasource.logMetric.failed')}`, cls: 'failed' })
  if (log.items_skipped > 0) pills.push({ text: `${log.items_skipped} ${t('datasource.logMetric.skipped')}`, cls: 'skipped' })
  return pills
}

function lastSyncStatusLabel(ds: DataSource) {
  const log = ds.latest_sync_log
  if (!log) return '--'
  return t(`datasource.logStatus.${log.status}`)
}

function lastSyncStatusColor(ds: DataSource) {
  const log = ds.latest_sync_log
  if (!log) return ''
  switch (log.status) {
    case 'success': return 'var(--td-success-color)'
    case 'failed': return 'var(--td-error-color)'
    case 'running': return 'var(--td-brand-color)'
    case 'partial': return 'var(--td-warning-color)'
    default: return ''
  }
}

function isSyncRunning(ds: DataSource) {
  return ds.latest_sync_log?.status === 'running'
}

function getSettingString(ds: DataSource, key: string) {
  const value = ds.config?.settings?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function getSettingList(ds: DataSource, key: string) {
  const value = ds.config?.settings?.[key]
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
  }
  if (typeof value === 'string' && value.trim()) {
    return [value.trim()]
  }
  return []
}

function sourceSummary(ds: DataSource) {
  if (ds.type === 'rss') {
    const feeds = getSettingList(ds, 'feed_urls')
    return feeds.length > 0
      ? t('datasource.summary.rssFeeds', { count: feeds.length })
      : t('datasource.summary.notConfigured')
  }

  if (ds.type === 'web_crawler') {
    const urls = getSettingList(ds, 'urls')
    const sitemap = getSettingString(ds, 'sitemap_url')
    if (urls.length > 0 && sitemap) {
      return t('datasource.summary.webMixed', { count: urls.length })
    }
    if (urls.length > 0) {
      return t('datasource.summary.webUrls', { count: urls.length })
    }
    if (sitemap) {
      return t('datasource.summary.webSitemap')
    }
    return t('datasource.summary.notConfigured')
  }

  const resourceCount = Array.isArray(ds.config?.resource_ids) ? ds.config.resource_ids.length : 0
  if (resourceCount > 0) {
    return t('datasource.summary.resources', { count: resourceCount })
  }
  return t('datasource.summary.notConfigured')
}

function latestFailureReason(ds: DataSource) {
  if (ds.error_message) {
    return ds.error_message
  }
  const log = ds.latest_sync_log
  if (!log) {
    return ''
  }
  if (log.error_message) {
    return log.error_message
  }
  if (Array.isArray(log.result?.errors) && log.result.errors.length > 0) {
    return log.result.errors[0]
  }
  return ''
}

function normalizedFailureReason(ds: DataSource) {
  return latestFailureReason(ds).toLowerCase()
}

function remediationKey(ds: DataSource) {
  const msg = normalizedFailureReason(ds)
  if (!msg) return ''
  if (msg.includes('storage engine') || msg.includes('storage provider') || msg.includes('storage')) {
    return 'storage'
  }
  if (msg.includes('feed_urls is required') || msg.includes('page url') || msg.includes('urls is required') || msg.includes('sitemap')) {
    return ds.type === 'rss' ? 'feeds' : 'pages'
  }
  if (msg.includes('403') || msg.includes('401') || msg.includes('forbidden') || msg.includes('unauthorized') || msg.includes('access denied')) {
    return 'auth'
  }
  if (msg.includes('404') || msg.includes('not found') || msg.includes('nosuchbucket')) {
    return 'missing'
  }
  if (msg.includes('timeout') || msg.includes('deadline exceeded') || msg.includes('i/o timeout') || msg.includes('connection reset')) {
    return 'network'
  }
  if (msg.includes('unsupported feed format') || msg.includes('invalid feed') || msg.includes('xml')) {
    return 'format'
  }
  if (msg.includes('invalid') || msg.includes('validate') || msg.includes('required')) {
    return 'config'
  }
  return 'retry'
}

function issueKind(ds: DataSource): 'config' | 'sync' | 'none' {
  if (ds.status === 'error' || !!ds.error_message) {
    return 'config'
  }
  if (ds.latest_sync_log?.status === 'failed' || ds.latest_sync_log?.status === 'partial') {
    return 'sync'
  }
  return 'none'
}

function issueLabel(ds: DataSource) {
  const kind = issueKind(ds)
  if (kind === 'config') return t('datasource.issue.config')
  if (kind === 'sync') return t('datasource.issue.sync')
  return ''
}

function issueTheme(ds: DataSource): 'danger' | 'warning' | 'default' {
  const kind = issueKind(ds)
  if (kind === 'config') return 'danger'
  if (kind === 'sync') return 'warning'
  return 'default'
}

function needsManualAction(ds: DataSource) {
  return issueKind(ds) !== 'none'
}

function manualActionTheme(ds: DataSource): 'danger' | 'warning' | 'default' {
  const key = remediationKey(ds)
  if (['storage', 'feeds', 'pages', 'auth', 'format', 'config'].includes(key)) {
    return 'danger'
  }
  if (['missing', 'network', 'retry'].includes(key)) {
    return 'warning'
  }
  return 'default'
}

function manualActionLabel(ds: DataSource) {
  const key = remediationKey(ds)
  if (!key) return ''
  return t(`datasource.manualAction.${key}`)
}

function manualActionHint(ds: DataSource) {
  const key = remediationKey(ds)
  if (!key) return ''
  return t(`datasource.manualHint.${key}`)
}

function syncBlockedReason(ds: DataSource) {
  const key = remediationKey(ds)
  if (['storage', 'feeds', 'pages', 'auth', 'format', 'config'].includes(key)) {
    return t(`datasource.syncBlocked.${key}`)
  }
  return ''
}

function canSyncNow(ds: DataSource) {
  return !syncBlockedReason(ds)
}

function syncActionLabel(ds: DataSource) {
  return canSyncNow(ds) ? t('datasource.syncNow') : t('datasource.fixBeforeSync')
}

function handlePrimaryAction(ds: DataSource) {
  if (canSyncNow(ds)) {
    handleSync(ds)
    return
  }
  openEdit(ds, resolveEditorFocusHint(ds))
}

function truncationSafeText(text: string, limit = 140) {
  const normalized = text.replace(/\s+/g, ' ').trim()
  if (normalized.length <= limit) {
    return normalized
  }
  return `${normalized.slice(0, limit)}...`
}

function isPreviewOpen(ds: DataSource) {
  return expandedPreviewId.value === ds.id
}

function previewMetricItems(ds: DataSource) {
  const log = ds.latest_sync_log
  if (!log) return []
  return [
    { key: 'created', value: log.items_created },
    { key: 'updated', value: log.items_updated },
    { key: 'deleted', value: log.items_deleted },
    { key: 'skipped', value: log.items_skipped },
    { key: 'failed', value: log.items_failed },
  ]
}

function previewErrorList(ds: DataSource) {
  const log = ds.latest_sync_log
  if (!log) return []
  if (Array.isArray(log.result?.errors) && log.result.errors.length > 0) {
    return log.result.errors.filter(Boolean).slice(0, 3)
  }
  if (log.error_message) {
    return [log.error_message]
  }
  if (ds.error_message) {
    return [ds.error_message]
  }
  return []
}

function onEditorSaved() {
  editorVisible.value = false
  loadList()
}

onMounted(loadList)
onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="ds-settings">
    <div class="section-header">
      <h2 class="section-title">{{ t('datasource.title') }}</h2>
      <p class="section-desc">{{ t('datasource.description') }}</p>
    </div>

    <div v-if="loading" class="ds-loading">
      <t-loading size="small" />
    </div>

    <div v-else-if="dataSources.length === 0" class="ds-empty">
      <div class="ds-empty-icon">
        <t-icon name="cloud-download" size="32px" />
      </div>
      <div class="ds-empty-text">
        <p class="ds-empty-title">{{ t('datasource.empty') }}</p>
      </div>
      <t-button theme="primary" @click="openCreate">
        <template #icon><t-icon name="add" /></template>
        {{ t('datasource.addFirst') }}
      </t-button>
    </div>

    <div v-else class="ds-list">
      <div v-for="ds in dataSources" :key="ds.id" class="ds-card">
        <div class="ds-card-header">
          <div class="ds-card-title-wrap">
            <DataSourceTypeIcon :type="ds.type" :size="32" />
            <div class="ds-title-text">
              <div class="ds-name-row">
                <span class="ds-name" :title="ds.name">{{ ds.name }}</span>
                <t-tag size="small" :theme="statusTheme(ds.status)" variant="light-outline" class="ds-status-tag">
                  {{ statusLabel(ds.status) }}
                </t-tag>
              </div>
              <span class="ds-type-desc">{{ connectorLabel(ds.type) }}</span>
            </div>
          </div>
          
          <div class="ds-card-actions">
            <t-tooltip :content="isSyncRunning(ds) ? t('datasource.logStatus.running') : (syncBlockedReason(ds) || syncActionLabel(ds))">
              <t-button
                size="small"
                variant="text"
                :theme="canSyncNow(ds) ? 'primary' : 'warning'"
                :disabled="isSyncRunning(ds)"
                @click="handlePrimaryAction(ds)"
              >
                <template #icon>
                  <t-icon :name="canSyncNow(ds) ? 'refresh' : 'edit-1'" :class="{ 'ds-icon-spin': isSyncRunning(ds) }" />
                </template>
              </t-button>
            </t-tooltip>
            <t-tooltip :content="t('datasource.logs')">
              <t-button size="small" variant="text" @click="openLogs(ds)">
                <template #icon><t-icon name="root-list" /></template>
              </t-button>
            </t-tooltip>
            <t-dropdown trigger="click" :min-column-width="120">
              <t-tooltip :content="t('datasource.moreActions')">
                <t-button size="small" variant="text" shape="square">
                  <template #icon><t-icon name="ellipsis" /></template>
                </t-button>
              </t-tooltip>
              <template #dropdown>
                <t-dropdown-menu>
                  <t-dropdown-item @click="openEdit(ds)">
                    <t-icon name="edit" /> {{ t('datasource.edit') }}
                  </t-dropdown-item>
                  <t-dropdown-item
                    v-if="ds.status === 'active'"
                    @click="handlePause(ds)"
                  >
                    <t-icon name="pause-circle" /> {{ t('datasource.pause') }}
                  </t-dropdown-item>
                  <t-dropdown-item
                    v-else-if="ds.status === 'paused'"
                    @click="handleResume(ds)"
                  >
                    <t-icon name="play-circle" /> {{ t('datasource.resume') }}
                  </t-dropdown-item>
                  <t-dropdown-item theme="error" @click="handleDelete(ds)">
                    <t-icon name="delete" /> {{ t('datasource.delete') }}
                  </t-dropdown-item>
                </t-dropdown-menu>
              </template>
            </t-dropdown>
          </div>
        </div>

        <div class="ds-card-stats">
          <div class="ds-stat-item">
            <span class="ds-stat-label">{{ t('datasource.syncModeLabel') }}</span>
            <span class="ds-stat-value">{{ syncModeLabel(ds.sync_mode) }}</span>
          </div>
          <div class="ds-stat-item">
            <span class="ds-stat-label">{{ t('datasource.schedule') }}</span>
            <span class="ds-stat-value">{{ scheduleLabel(ds.sync_schedule) }}</span>
          </div>
          <div class="ds-stat-item">
            <span class="ds-stat-label">{{ t('datasource.lastSync') }}</span>
            <t-tooltip :content="lastSyncFullTime(ds)" :disabled="!lastSyncFullTime(ds)">
              <span class="ds-stat-value">{{ lastSyncTime(ds) }}</span>
            </t-tooltip>
          </div>
          <div class="ds-stat-item" style="flex: 1.2">
            <span class="ds-stat-label">{{ t('datasource.lastStatus') }}</span>
            <div class="ds-stat-value">
              <template v-if="ds.latest_sync_log">
                <span :style="{ color: lastSyncStatusColor(ds), fontWeight: 500 }">{{ lastSyncStatusLabel(ds) }}</span>
                <span v-for="pill in syncResultPills(ds)" :key="pill.cls" :class="['ds-pill', pill.cls]">{{ pill.text }}</span>
              </template>
              <span v-else class="ds-stat-placeholder">--</span>
            </div>
          </div>
        </div>

        <div class="ds-card-meta">
          <div class="ds-meta-row">
            <span class="ds-meta-label">{{ t('datasource.summaryLabel') }}</span>
            <span class="ds-meta-value">{{ sourceSummary(ds) }}</span>
          </div>
          <div class="ds-meta-row">
            <span class="ds-meta-label">{{ t('datasource.deletionModeLabel') }}</span>
            <span class="ds-meta-value">
              {{ ds.sync_deletions ? t('datasource.deletionMode.sync') : t('datasource.deletionMode.ignore') }}
            </span>
          </div>
        </div>

        <div v-if="ds.latest_sync_log" class="ds-preview-toggle">
          <t-button size="small" variant="text" theme="primary" @click="togglePreview(ds)">
            {{ isPreviewOpen(ds) ? t('datasource.hideRunDetails') : t('datasource.showRunDetails') }}
          </t-button>
        </div>

        <div v-if="ds.latest_sync_log && isPreviewOpen(ds)" class="ds-sync-preview">
          <div class="ds-sync-preview-header">
            <span class="ds-sync-preview-title">{{ t('datasource.latestRunDetails') }}</span>
            <span class="ds-sync-preview-time">
              {{ ds.latest_sync_log.finished_at ? new Date(ds.latest_sync_log.finished_at).toLocaleString() : t('datasource.logStatus.running') }}
            </span>
          </div>

          <div class="ds-sync-preview-grid">
            <div v-for="item in previewMetricItems(ds)" :key="item.key" class="ds-preview-metric">
              <span class="ds-preview-metric-value">{{ item.value }}</span>
              <span class="ds-preview-metric-label">{{ t(`datasource.logMetric.${item.key}`) }}</span>
            </div>
          </div>

          <div class="ds-sync-preview-errors">
            <div class="ds-sync-preview-subtitle">{{ t('datasource.latestErrors') }}</div>
            <template v-if="previewErrorList(ds).length > 0">
              <div
                v-for="(error, index) in previewErrorList(ds)"
                :key="`${ds.id}-error-${index}`"
                class="ds-sync-preview-error"
                :title="error"
              >
                <t-icon name="error-circle-filled" size="14px" />
                <span>{{ truncationSafeText(error, 220) }}</span>
              </div>
            </template>
            <div v-else class="ds-sync-preview-empty">{{ t('datasource.noRecentErrors') }}</div>
          </div>
        </div>

        <div v-if="needsManualAction(ds)" class="ds-manual-action">
          <div class="ds-manual-action-header">
            <t-tag size="small" :theme="manualActionTheme(ds)" variant="light-outline">
              {{ t('datasource.manualActionRequired') }}
            </t-tag>
            <span class="ds-manual-action-label">{{ manualActionLabel(ds) }}</span>
          </div>
          <div class="ds-manual-action-hint">{{ manualActionHint(ds) }}</div>
        </div>

        <div v-if="issueKind(ds) !== 'none'" class="ds-card-issue">
          <div class="ds-issue-header">
            <t-tag size="small" :theme="issueTheme(ds)" variant="light-outline">
              {{ issueLabel(ds) }}
            </t-tag>
            <t-button size="small" variant="text" theme="primary" @click="openLogs(ds)">
              {{ t('datasource.viewLogs') }}
            </t-button>
          </div>
          <div class="ds-issue-body">
            <t-icon name="error-circle-filled" size="16px" />
            <span :title="latestFailureReason(ds)">
              {{ truncationSafeText(latestFailureReason(ds)) }}
            </span>
          </div>
        </div>
      </div>

      <div class="ds-card-add" @click="openCreate">
        <div class="ds-card-add-icon">
          <t-icon name="add" size="20px" />
        </div>
        <span>{{ t('datasource.addCard') }}</span>
      </div>
    </div>

    <DataSourceEditorDialog
      v-model:visible="editorVisible"
      :kb-id="kbId"
      :data-source="editingDs"
      :focus-hint="editorFocusHint"
      @saved="onEditorSaved"
    />

    <DataSourceSyncLogs
      v-model:visible="logsVisible"
      :data-source-id="logsDsId"
      :data-source-name="logsDsName"
    />
  </div>
</template>

<style scoped>
.ds-settings {
  padding: 0;
}

/* --- Section header --- */
.section-header {
  margin-bottom: 20px;
}

.section-title {
  margin: 0 0 6px 0;
  font-size: 20px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  letter-spacing: -0.01em;
}

.section-desc {
  margin: 0;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
  line-height: 20px;
}

/* --- Loading --- */
.ds-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
}

/* --- Empty state --- */
.ds-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 56px 24px;
  gap: 16px;
  border: 1px dashed var(--td-border-level-2-color);
  border-radius: 12px;
  background: var(--td-bg-color-container);
}

.ds-empty-icon {
  width: 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
}

.ds-empty-text {
  text-align: center;
}

.ds-empty-title {
  margin: 0;
  font-size: 14px;
  color: var(--td-text-color-secondary);
  line-height: 22px;
}

/* --- List --- */
.ds-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* --- Card --- */
.ds-card {
  position: relative;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-border-level-2-color);
  border-radius: 8px;
  padding: 15px 20px;
  transition: all 0.2s ease;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.02);
}

.ds-card:hover {
  border-color: var(--td-brand-color);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.06);
}

/* --- Card header --- */
.ds-card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
}

.ds-card-title-wrap {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
  flex: 1;
}

.ds-title-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.ds-name-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.ds-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 24px;
}

.ds-status-tag {
  border-radius: 4px;
}

.ds-type-desc {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  line-height: 18px;
}

.ds-card-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  opacity: 0.6;
  transition: opacity 0.2s ease;
}

.ds-card:hover .ds-card-actions {
  opacity: 1;
}

.ds-card-actions :deep(.t-button) {
  border-radius: 6px;
}

/* --- Info stats --- */
.ds-card-stats {
  display: flex;
  gap: 24px;
  padding-top: 16px;
  border-top: 1px solid var(--td-border-level-1-color);
}

.ds-card-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 16px;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--td-border-level-1-color);
}

.ds-preview-toggle {
  display: flex;
  justify-content: flex-start;
  margin-top: 8px;
}

.ds-sync-preview {
  margin-top: 10px;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid var(--td-border-level-1-color);
  background: linear-gradient(180deg, var(--td-bg-color-container-hover) 0%, var(--td-bg-color-container) 100%);
}

.ds-sync-preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.ds-sync-preview-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.ds-sync-preview-time {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ds-sync-preview-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}

.ds-preview-metric {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px;
  border-radius: 10px;
  background: var(--td-bg-color-secondarycontainer);
}

.ds-preview-metric-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  line-height: 1;
}

.ds-preview-metric-label {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ds-sync-preview-errors {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ds-sync-preview-subtitle {
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-secondary);
}

.ds-sync-preview-error {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-error-color-6);
}

.ds-sync-preview-empty {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ds-manual-action {
  margin-top: 12px;
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--td-bg-color-container-hover);
  border: 1px dashed var(--td-border-level-2-color);
}

.ds-manual-action-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.ds-manual-action-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.ds-manual-action-hint {
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

.ds-meta-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.ds-meta-label {
  font-size: 11px;
  font-weight: 500;
  color: var(--td-text-color-placeholder);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  line-height: 16px;
}

.ds-meta-value {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  line-height: 20px;
  word-break: break-word;
}

.ds-stat-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
}

.ds-stat-label {
  font-size: 11px;
  font-weight: 500;
  color: var(--td-text-color-placeholder);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  line-height: 16px;
  white-space: nowrap;
}

.ds-stat-value {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 13px;
  color: var(--td-text-color-primary);
  line-height: 20px;
}

.ds-stat-placeholder {
  color: var(--td-text-color-disabled);
}

/* --- Sync result pills --- */
.ds-pill {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  line-height: 18px;
}

.ds-pill.created { background: var(--td-success-color-1); color: var(--td-success-color); }
.ds-pill.updated { background: var(--td-brand-color-light); color: var(--td-brand-color); }
.ds-pill.deleted { background: var(--td-warning-color-1); color: var(--td-warning-color); }
.ds-pill.skipped { background: var(--td-bg-color-component); color: var(--td-text-color-placeholder); }
.ds-pill.failed  { background: var(--td-error-color-1); color: var(--td-error-color); }

.ds-icon-spin {
  animation: ds-spin 1s linear infinite;
}

@keyframes ds-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* --- Error alert --- */
.ds-card-issue {
  margin-top: 14px;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--td-bg-color-container-hover);
  border: 1px solid var(--td-border-level-1-color);
}

.ds-issue-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.ds-issue-body {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 20px;
}

.ds-issue-body :deep(.t-icon) {
  margin-top: 2px;
  color: var(--td-warning-color);
  flex-shrink: 0;
}

/* --- Add card --- */
.ds-card-add {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 6px 15px;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--td-text-color-secondary);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  margin-top: 4px;
}

.ds-card-add-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--td-bg-color-component);
  color: var(--td-text-color-placeholder);
  transition: all 0.2s ease;
}

.ds-card-add:hover {
  background: var(--td-bg-color-container);
  border-color: var(--td-brand-color);
  color: var(--td-brand-color);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04);
}

.ds-card-add:hover .ds-card-add-icon {
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
}

@media (max-width: 900px) {
  .ds-card-stats,
  .ds-card-meta {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }

  .ds-sync-preview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .ds-card {
    padding: 14px 16px;
  }

  .ds-card-header {
    flex-direction: column;
    gap: 12px;
    margin-bottom: 16px;
  }

  .ds-card-actions {
    width: 100%;
    justify-content: flex-end;
  }

  .ds-card-stats,
  .ds-card-meta {
    grid-template-columns: 1fr;
  }

  .ds-sync-preview-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .ds-sync-preview-grid {
    grid-template-columns: 1fr;
  }
}
</style>

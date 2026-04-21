<script setup lang="ts">
import { ref, computed, watch, nextTick, type ComponentPublicInstance } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useUIStore } from '@/stores/ui'
import {
  createDataSource,
  updateDataSource,
  triggerSync,
  validateConnection,
  validateCredentials,
  listResources,
  deleteDataSource,
  type DataSource,
  type Resource,
} from '@/api/datasource'
import { getKnowledgeBaseById } from '@/api/knowledge-base'
import DataSourceTypeIcon from './DataSourceTypeIcon.vue'

const props = defineProps<{
  kbId: string
  dataSource: DataSource | null
  focusHint?: string
}>()

const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const uiStore = useUIStore()

const isEdit = computed(() => !!props.dataSource)
const step = ref(0)
const submitting = ref(false)

// Form data
const form = ref({
  name: '',
  type: '',
  config: {
    credentials: {} as Record<string, any>,
    resource_ids: [] as string[],
    settings: {} as Record<string, any>,
  },
  sync_schedule: '0 0 */6 * * *',
  sync_mode: 'incremental' as 'incremental' | 'full',
  conflict_strategy: 'overwrite' as 'overwrite' | 'skip',
  sync_deletions: true,
})

// Step 2: Resources
const resources = ref<Resource[]>([])
const loadingResources = ref(false)
const selectedResourceIds = ref<string[]>([])

// Connection test
const testing = ref(false)
const testResult = ref<'success' | 'error' | ''>('')
const testErrorMsg = ref('')
const kbStorageLoading = ref(false)
const kbStorageProvider = ref('')
const highlightedFocusHint = ref('')
const fieldRefs = new Map<string, HTMLElement>()

// Collapsible prereq in Step 1
const prereqExpanded = ref(false)


// Temp data source for resource listing
const tempDsId = ref('')

// Schedule presets
const schedulePresets = computed(() => [
  { label: t('datasource.schedule30min'), value: '0 */30 * * * *' },
  { label: t('datasource.schedule1h'), value: '0 0 * * * *' },
  { label: t('datasource.schedule6h'), value: '0 0 */6 * * *' },
  { label: t('datasource.schedule12h'), value: '0 0 */12 * * *' },
  { label: t('datasource.schedule24h'), value: '0 0 2 * * *' },
])

// --- Connector definitions ---
interface ConnectorDef {
  type: string
  available: boolean
  docUrl: string
  permissionDocUrl: string
  permissionPageUrl: string
  requiredPermissions: string[]
  fields: {
    key: string
    labelKey: string
    placeholder: string
    secret?: boolean
    required?: boolean
    scope?: 'credentials' | 'settings'
    kind?: 'text' | 'multiline-list'
  }[]
}

const connectorDefs = computed<ConnectorDef[]>(() => [
  {
    type: 'feishu',
    available: true,
    docUrl: 'https://open.feishu.cn/app',
    permissionDocUrl: 'https://open.feishu.cn/document/server-docs/docs/wiki-v2/wiki-overview',
    permissionPageUrl: 'https://open.feishu.cn/app',
    requiredPermissions: [
      'wiki:wiki:readonly',
      'drive:drive:readonly',
      'drive:export:readonly',
      'docx:document:readonly',
    ],
    fields: [
      { key: 'app_id', labelKey: 'datasource.field.appId', placeholder: 'cli_xxxx' },
      { key: 'app_secret', labelKey: 'datasource.field.appSecret', placeholder: '', secret: true },
    ],
  },
  {
    type: 'web_crawler',
    available: true,
    docUrl: 'https://www.sitemaps.org/protocol.html',
    permissionDocUrl: '',
    permissionPageUrl: '',
    requiredPermissions: [],
    fields: [
      {
        key: 'urls',
        labelKey: 'datasource.field.pageUrls',
        placeholder: t('datasource.fieldHint.pageUrls'),
        scope: 'settings',
        kind: 'multiline-list',
        required: false,
      },
      {
        key: 'sitemap_url',
        labelKey: 'datasource.field.sitemapUrl',
        placeholder: 'https://example.com/sitemap.xml',
        scope: 'settings',
        required: false,
      },
      {
        key: 'user_agent',
        labelKey: 'datasource.field.userAgent',
        placeholder: 'WeKnora-Crawler/1.0',
        scope: 'settings',
        required: false,
      },
    ],
  },
  {
    type: 'rss',
    available: true,
    docUrl: 'https://validator.w3.org/feed/',
    permissionDocUrl: '',
    permissionPageUrl: '',
    requiredPermissions: [],
    fields: [
      {
        key: 'feed_urls',
        labelKey: 'datasource.field.feedUrls',
        placeholder: t('datasource.fieldHint.feedUrls'),
        scope: 'settings',
        kind: 'multiline-list',
      },
      {
        key: 'user_agent',
        labelKey: 'datasource.field.userAgent',
        placeholder: 'WeKnora-RSS/1.0',
        scope: 'settings',
        required: false,
      },
    ],
  },
  {
    type: 'notion',
    available: false,
    docUrl: 'https://www.notion.so/my-integrations',
    permissionDocUrl: '',
    permissionPageUrl: '',
    requiredPermissions: [],
    fields: [
      { key: 'api_key', labelKey: 'datasource.field.integrationToken', placeholder: 'ntn_xxxx', secret: true },
    ],
  },
  {
    type: 'yuque',
    available: false,
    docUrl: 'https://www.yuque.com/settings/tokens',
    permissionDocUrl: '',
    permissionPageUrl: '',
    requiredPermissions: [],
    fields: [
      { key: 'api_token', labelKey: 'datasource.field.apiToken', placeholder: '', secret: true },
    ],
  },
])


const currentDef = computed(() => connectorDefs.value.find(d => d.type === form.value.type))
const isKbStorageConfigured = computed(() => {
  const provider = kbStorageProvider.value.trim()
  return !!provider && provider !== '__pending_env__'
})

function ensureConfigBuckets() {
  if (!form.value.config) {
    form.value.config = { credentials: {}, resource_ids: [], settings: {} }
  }
  if (!form.value.config.credentials) {
    form.value.config.credentials = {}
  }
  if (!form.value.config.settings) {
    form.value.config.settings = {}
  }
  if (!Array.isArray(form.value.config.resource_ids)) {
    form.value.config.resource_ids = []
  }
}

function getFieldBucket(field: ConnectorDef['fields'][number]) {
  ensureConfigBuckets()
  return field.scope === 'settings' ? form.value.config.settings : form.value.config.credentials
}

function getFieldValue(field: ConnectorDef['fields'][number]) {
  const bucket = getFieldBucket(field)
  const value = bucket[field.key]
  if (field.kind === 'multiline-list') {
    if (Array.isArray(value)) {
      return value.join('\n')
    }
    return typeof value === 'string' ? value : ''
  }
  return typeof value === 'string' ? value : ''
}

function setFieldValue(field: ConnectorDef['fields'][number], value: string) {
  const bucket = getFieldBucket(field)
  if (field.kind === 'multiline-list') {
    bucket[field.key] = value
      .split(/\r?\n|,/)
      .map(item => item.trim())
      .filter(Boolean)
    return
  }
  bucket[field.key] = value
}

function handleFieldChange(field: ConnectorDef['fields'][number], value: string | number) {
  setFieldValue(field, String(value ?? ''))
}

function supportsRawCredentialValidation(def?: ConnectorDef) {
  if (!def || def.fields.length === 0) {
    return false
  }
  return def.fields.every(field => (field.scope || 'credentials') === 'credentials')
}

async function loadKnowledgeBaseStorageStatus() {
  if (!props.kbId) return
  kbStorageLoading.value = true
  try {
    const res: any = await getKnowledgeBaseById(props.kbId)
    const kb = res?.data || res
    kbStorageProvider.value = kb?.storage_provider_config?.provider || kb?.storage_config?.provider || ''
  } catch {
    kbStorageProvider.value = ''
  } finally {
    kbStorageLoading.value = false
  }
}

function ensureStorageConfigured() {
  if (isKbStorageConfigured.value) {
    return true
  }
  MessagePlugin.warning(t('datasource.storageRequired'))
  return false
}

function goToStorageSettings() {
  if (!props.kbId) {
    return
  }
  uiStore.kbEditorInitialSection = 'storage'
  visible.value = false
}

function setFieldRef(key: string, el: Element | ComponentPublicInstance | null) {
  const element = el instanceof HTMLElement
    ? el
    : el && '$el' in el && el.$el instanceof HTMLElement
      ? el.$el
      : null

  if (element) {
    fieldRefs.set(key, element)
    return
  }
  fieldRefs.delete(key)
}

function resolveFocusFieldKey() {
  if (props.focusHint === 'storage') {
    return 'storage'
  }
  if (props.focusHint === 'primary') {
    return currentDef.value?.fields?.[0]?.key || ''
  }
  return props.focusHint || ''
}

async function focusHintTarget() {
  const key = resolveFocusFieldKey()
  highlightedFocusHint.value = key
  if (!key) {
    return
  }
  await nextTick()
  if (key === 'storage') {
    const warning = document.querySelector('.ds-storage-warning') as HTMLElement | null
    warning?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    return
  }
  const target = fieldRefs.get(key)
  target?.scrollIntoView({ behavior: 'smooth', block: 'center' })
}

// --- Dialog lifecycle ---
watch(visible, (v) => {
  if (!v) return
  step.value = isEdit.value ? 1 : 0
  testResult.value = ''
  testErrorMsg.value = ''
  tempDsId.value = ''
  kbStorageProvider.value = ''
  prereqExpanded.value = false
  resources.value = []
  selectedResourceIds.value = []
  loadKnowledgeBaseStorageStatus()

  if (isEdit.value && props.dataSource) {
    form.value = {
      name: props.dataSource.name,
      type: props.dataSource.type,
      config: props.dataSource.config || { credentials: {}, resource_ids: [], settings: {} },
      sync_schedule: props.dataSource.sync_schedule,
      sync_mode: props.dataSource.sync_mode,
      conflict_strategy: props.dataSource.conflict_strategy,
      sync_deletions: props.dataSource.sync_deletions,
    }
    selectedResourceIds.value = form.value.config?.resource_ids || []
    tempDsId.value = props.dataSource.id
  } else {
    form.value = {
      name: '',
      type: '',
      config: { credentials: {}, resource_ids: [], settings: {} },
      sync_schedule: '0 0 */6 * * *',
      sync_mode: 'incremental',
      conflict_strategy: 'overwrite',
      sync_deletions: true,
    }
  }
  ensureConfigBuckets()
  focusHintTarget()
})

function selectType(def: ConnectorDef) {
  if (!def.available) return
  form.value.type = def.type
  form.value.name = t(`datasource.connector.${def.type}`)
  step.value = 1
}

// --- Test connection (stateless, no DB write) ---
async function testConnection() {
  const fields = currentDef.value?.fields || []
  for (const f of fields) {
    const required = f.required !== false
    const value = getFieldBucket(f)[f.key]
    const hasValue = Array.isArray(value) ? value.length > 0 : !!String(value || '').trim()
    if (required && !hasValue) {
      MessagePlugin.warning(`${t(f.labelKey)} ${t('datasource.isRequired')}`)
      return
    }
  }

  testing.value = true
  testResult.value = ''
  testErrorMsg.value = ''
  try {
    if (isEdit.value && tempDsId.value) {
      await updateDataSource(tempDsId.value, {
        ...form.value,
        knowledge_base_id: props.kbId,
      } as any)
      await validateConnection(tempDsId.value)
    } else if (supportsRawCredentialValidation(currentDef.value)) {
      await validateCredentials(form.value.type, form.value.config.credentials)
    } else {
      if (!tempDsId.value) {
        const res = await createDataSource({
          ...form.value,
          knowledge_base_id: props.kbId,
          status: 'paused',
        } as any)
        const created = res?.data || res
        tempDsId.value = created.id
      } else {
        await updateDataSource(tempDsId.value, {
          ...form.value,
          knowledge_base_id: props.kbId,
        } as any)
      }
      await validateConnection(tempDsId.value)
    }
    testResult.value = 'success'
    MessagePlugin.success(t('datasource.testSuccess'))
  } catch (e: any) {
    testResult.value = 'error'
    testErrorMsg.value = e?.message || e?.error || ''
    MessagePlugin.error(t('datasource.testFailed'))
  }
  testing.value = false
}

// --- Load resources ---
async function loadResources() {
  loadingResources.value = true
  try {
    if (!tempDsId.value) {
      const res = await createDataSource({
        ...form.value,
        knowledge_base_id: props.kbId,
        status: 'paused',
      } as any)
      const created = res?.data || res
      tempDsId.value = created.id
    } else {
      await updateDataSource(tempDsId.value, {
        ...form.value,
        knowledge_base_id: props.kbId,
      } as any)
    }

    const res = await listResources(tempDsId.value)
    resources.value = res?.data || res || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.resourceLoadFailed'))
  }
  loadingResources.value = false
}

function toggleResource(id: string) {
  const idx = selectedResourceIds.value.indexOf(id)
  if (idx >= 0) {
    selectedResourceIds.value.splice(idx, 1)
  } else {
    selectedResourceIds.value.push(id)
  }
}

function validateStep1Fields(): boolean {
  const fields = currentDef.value?.fields || []
  for (const f of fields) {
    const required = f.required !== false
    const value = getFieldBucket(f)[f.key]
    const hasValue = Array.isArray(value) ? value.length > 0 : !!String(value || '').trim()
    if (required && !hasValue) {
      MessagePlugin.warning(`${t(f.labelKey)} ${t('datasource.isRequired')}`)
      return false
    }
  }
  return true
}

function nextStep() {
  if (step.value === 1) {
    if (!validateStep1Fields()) return
    if (testResult.value !== 'success') {
      MessagePlugin.warning(t('datasource.pleaseTestFirst'))
      return
    }
  }
  step.value++
  if (step.value === 2) {
    loadResources()
  }
}

function prevStep() {
  step.value--
}

// --- Final submit ---
async function handleSubmit() {
  if (!ensureStorageConfigured()) {
    return
  }
  form.value.config.resource_ids = selectedResourceIds.value
  submitting.value = true
  try {
    let dataSourceId = tempDsId.value

    if (tempDsId.value) {
      await updateDataSource(tempDsId.value, {
        ...form.value,
        knowledge_base_id: props.kbId,
        status: 'active',
      } as any)
    } else {
      const res = await createDataSource({
        ...form.value,
        knowledge_base_id: props.kbId,
        status: 'active',
      } as any)
      const created = res?.data || res
      dataSourceId = created.id
      tempDsId.value = created.id
    }

    if (isEdit.value) {
      MessagePlugin.success(t('datasource.updateSuccess'))
    } else {
      try {
        await triggerSync(dataSourceId)
        MessagePlugin.success(t('datasource.createAndSyncSuccess'))
      } catch (e: any) {
        MessagePlugin.warning(e?.message || e?.error || t('datasource.createButSyncFailed'))
      }
    }

    emit('saved')
    visible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || e?.error || t('datasource.saveFailed'))
  }
  submitting.value = false
}

// --- Cleanup on dialog close ---
async function handleClose() {
  if (!isEdit.value && tempDsId.value) {
    try {
      await deleteDataSource(tempDsId.value)
    } catch {
      // Ignore cleanup errors
    }
    tempDsId.value = ''
  }
  visible.value = false
}

const resourceTypeLabelMap: Record<string, string> = {
  wiki_space: 'datasource.resourceType.wikiSpace',
  doc_category: 'datasource.resourceType.docCategory',
}

function resourceTypeLabel(type: string): string {
  const key = resourceTypeLabelMap[type]
  return key ? t(key) : type
}

const stepTitles = computed(() => [
  t('datasource.step.selectType'),
  t('datasource.step.credentials'),
  t('datasource.step.resources'),
  t('datasource.step.strategy'),
])
</script>

<template>
  <t-dialog
    v-model:visible="visible"
    :header="isEdit ? t('datasource.editTitle') : t('datasource.createTitle')"
    :footer="false"
    width="640px"
    destroy-on-close
    :on-close="handleClose"
  >
    <!-- Step indicator -->
    <div class="ds-steps">
      <div
        v-for="(title, i) in stepTitles"
        :key="i"
        :class="['ds-step', { active: step === i, done: step > i }]"
      >
        <span class="ds-step-num">{{ step > i ? '&#10003;' : i + 1 }}</span>
        <span class="ds-step-title">{{ title }}</span>
      </div>
    </div>

    <!-- Step 0: Select connector type -->
    <div v-if="step === 0" class="ds-step-content">
      <div class="ds-type-grid">
        <div
          v-for="def in connectorDefs"
          :key="def.type"
          :class="['ds-type-card', { disabled: !def.available }]"
          @click="selectType(def)"
        >
          <div class="ds-type-header">
            <DataSourceTypeIcon :type="def.type" :size="20" />
            <span class="ds-type-name">{{ t(`datasource.connector.${def.type}`) }}</span>
            <span v-if="!def.available" class="ds-type-soon">{{ t('datasource.comingSoon') }}</span>
          </div>
          <div class="ds-type-desc">{{ t(`datasource.connectorDesc.${def.type}`) }}</div>
        </div>
      </div>
    </div>

    <!-- Step 1: Credentials -->
    <div v-if="step === 1" class="ds-step-content">
      <!-- Compact collapsible prereq hint -->
      <div v-if="currentDef && currentDef.requiredPermissions.length > 0" class="ds-prereq-bar" @click="prereqExpanded = !prereqExpanded">
        <t-icon name="help-circle" size="14px" />
        <span>{{ t(`datasource.prereqBarText_${form.type}`, t('datasource.prereqBarText')) }}</span>
        <t-icon :name="prereqExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="ds-prereq-arrow" />
      </div>
      <div v-if="prereqExpanded && currentDef" class="ds-prereq-detail">
        <div class="ds-prereq-item">
          <span class="ds-prereq-num">1</span>
          <div>
            <div class="ds-prereq-item-title">{{ t(`datasource.prereqStep1Brief_${form.type}`, t('datasource.prereqBotBrief')) }}</div>
            <div class="ds-prereq-item-desc">{{ t(`datasource.prereqStep1Desc_${form.type}`, t('datasource.prereqBotDesc')) }}</div>
          </div>
        </div>
        <div class="ds-prereq-item">
          <span class="ds-prereq-num">2</span>
          <div>
            <div class="ds-prereq-item-title">{{ t(`datasource.prereqStep2Brief_${form.type}`, t('datasource.prereqPermBrief')) }}</div>
            <div class="ds-prereq-item-desc">
              <template v-if="!t(`datasource.prereqStep2Desc_${form.type}`)">
                <code v-for="perm in currentDef.requiredPermissions" :key="perm" class="ds-perm-tag">{{ perm }}</code>
              </template>
              <template v-else>{{ t(`datasource.prereqStep2Desc_${form.type}`) }}</template>
            </div>
          </div>
        </div>
        <div class="ds-prereq-item">
          <span class="ds-prereq-num">3</span>
          <div>
            <div class="ds-prereq-item-title">{{ t(`datasource.prereqStep3Brief_${form.type}`, t('datasource.prereqMemberBrief')) }}</div>
            <div class="ds-prereq-item-desc">{{ t(`datasource.prereqStep3Desc_${form.type}`, t('datasource.prereqMemberDesc')) }}</div>
          </div>
        </div>
        <a :href="currentDef.permissionPageUrl" target="_blank" rel="noopener" class="ds-prereq-link">
          {{ t('datasource.prereqOpenConsole') }}
        </a>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('datasource.nameLabel') }}</label>
        <t-input v-model="form.name" :placeholder="t('datasource.namePlaceholder')" />
      </div>

      <div
        v-if="!kbStorageLoading && !isKbStorageConfigured"
        :class="['ds-storage-warning', { 'is-highlighted': highlightedFocusHint === 'storage' }]"
      >
        <t-icon name="error-circle-filled" size="16px" />
        <div class="ds-storage-warning-content">
          <span class="ds-storage-warning-title">{{ t('datasource.storageWarningTitle') }}</span>
          <span>{{ t('datasource.storageWarningDesc') }}</span>
        </div>
        <t-button size="small" variant="outline" theme="warning" @click="goToStorageSettings">
          {{ t('knowledgeBase.goToStorageSettings') }}
        </t-button>
      </div>

      <div v-if="currentDef?.docUrl" class="ds-doc-link">
        <t-icon name="info-circle" size="14px" />
        <span>{{ t('datasource.docHint') }}</span>
        <a :href="currentDef.docUrl" target="_blank" rel="noopener">{{ currentDef.docUrl }}</a>
      </div>

      <div
        v-for="field in currentDef?.fields || []"
        :key="field.key"
        :ref="(el) => setFieldRef(field.key, el)"
        :class="['form-item', { 'is-highlighted': highlightedFocusHint === field.key }]"
      >
        <label class="form-label">{{ t(field.labelKey) }}</label>
        <t-input
          v-if="field.kind !== 'multiline-list'"
          :value="getFieldValue(field)"
          :placeholder="field.placeholder"
          :type="field.secret ? 'password' : 'text'"
          @change="handleFieldChange(field, $event)"
        />
        <t-textarea
          v-else
          :value="getFieldValue(field)"
          :placeholder="field.placeholder"
          :autosize="{ minRows: 4, maxRows: 8 }"
          @change="handleFieldChange(field, $event)"
        />
        <div v-if="field.kind === 'multiline-list'" class="form-tip compact">
          {{ t('datasource.multilineHint') }}
        </div>
      </div>

      <div class="form-actions">
        <t-button variant="outline" :loading="testing" @click="testConnection">
          {{ t('datasource.testConnection') }}
        </t-button>
        <span v-if="testResult === 'success'" class="test-ok">
          <t-icon name="check-circle-filled" size="14px" />
          {{ t('datasource.connected') }}
        </span>
      </div>
      <div v-if="testResult === 'error'" class="test-error-box">
        <t-icon name="error-circle-filled" size="16px" />
        <div class="test-error-content">
          <span class="test-error-title">{{ t('datasource.connectionFailed') }}</span>
          <span v-if="testErrorMsg" class="test-error-detail">{{ testErrorMsg }}</span>
        </div>
      </div>

      <div class="ds-dialog-footer">
        <t-button variant="outline" @click="step = 0" v-if="!isEdit">{{ t('datasource.back') }}</t-button>
        <t-button theme="primary" @click="nextStep">{{ t('datasource.next') }}</t-button>
      </div>
    </div>

    <!-- Step 2: Select resources -->
    <div v-if="step === 2" class="ds-step-content">
      <p class="form-tip">{{ t('datasource.resourceHint') }}</p>
      <div v-if="loadingResources" style="text-align:center;padding:20px"><t-loading /></div>
      <div v-else-if="resources.length > 0" class="ds-resource-list">
        <div
          v-for="r in resources"
          :key="r.external_id"
          :class="['ds-resource-row', { selected: selectedResourceIds.includes(r.external_id) }]"
          @click="toggleResource(r.external_id)"
        >
          <t-checkbox
            :checked="selectedResourceIds.includes(r.external_id)"
            @click.stop
            @change="toggleResource(r.external_id)"
          />
          <div class="ds-resource-info">
            <div class="ds-resource-name">{{ r.name }}</div>
            <div class="ds-resource-meta">
              <span class="ds-resource-type">{{ resourceTypeLabel(r.type) }}</span>
              <span v-if="r.description" class="ds-resource-desc">{{ r.description }}</span>
            </div>
          </div>
        </div>
      </div>
      <!-- Empty state: concise guide -->
      <div v-else class="ds-resource-empty">
        <t-icon name="info-circle" size="32px" style="color: var(--td-warning-color); margin-bottom: 8px;" />
        <p class="ds-empty-title">{{ t('datasource.noResources') }}</p>
        <p class="ds-empty-desc">{{ t(`datasource.noResourcesDesc_${form.type}`, t('datasource.noResourcesDesc')) }}</p>
        <div class="ds-guide-steps">
          <div class="ds-guide-step">
            <span class="ds-guide-num">1</span>
            <span>{{ t(`datasource.guideStep1_${form.type}`, t('datasource.guideStep1')) }}</span>
          </div>
          <div class="ds-guide-step">
            <span class="ds-guide-num">2</span>
            <span>{{ t(`datasource.guideStep2_${form.type}`, t('datasource.guideStep2')) }}</span>
          </div>
          <div class="ds-guide-step">
            <span class="ds-guide-num">3</span>
            <span>{{ t(`datasource.guideStep3_${form.type}`, t('datasource.guideStep3')) }}</span>
          </div>
        </div>
        <div class="ds-empty-actions">
          <t-button variant="outline" size="small" @click="loadResources">
            {{ t('datasource.retryLoadResources') }}
          </t-button>
          <a v-if="currentDef?.permissionDocUrl" :href="currentDef.permissionDocUrl" target="_blank" rel="noopener" class="ds-doc-link-inline">
            {{ t('datasource.permissionDocLink') }}
          </a>
        </div>
      </div>

      <div class="ds-dialog-footer">
        <t-button variant="outline" @click="prevStep">{{ t('datasource.back') }}</t-button>
        <t-button theme="primary" @click="nextStep">{{ t('datasource.next') }}</t-button>
      </div>
    </div>

    <!-- Step 3: Sync strategy -->
    <div v-if="step === 3" class="ds-step-content">
      <div class="form-item">
        <label class="form-label">{{ t('datasource.syncScheduleLabel') }}</label>
        <t-select v-model="form.sync_schedule">
          <t-option v-for="p in schedulePresets" :key="p.value" :value="p.value" :label="p.label" />
        </t-select>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('datasource.syncModeLabel') }}</label>
        <t-radio-group v-model="form.sync_mode">
          <t-radio value="incremental">{{ t('datasource.syncMode.incremental') }}</t-radio>
          <t-radio value="full">{{ t('datasource.syncMode.full') }}</t-radio>
        </t-radio-group>
      </div>

      <div class="form-item">
        <label class="form-label">{{ t('datasource.conflictLabel') }}</label>
        <t-radio-group v-model="form.conflict_strategy">
          <t-radio value="overwrite">{{ t('datasource.conflict.overwrite') }}</t-radio>
          <t-radio value="skip">{{ t('datasource.conflict.skip') }}</t-radio>
        </t-radio-group>
      </div>

      <div class="form-item">
        <t-checkbox v-model="form.sync_deletions">{{ t('datasource.syncDeletions') }}</t-checkbox>
      </div>

      <div class="ds-dialog-footer">
        <t-button variant="outline" @click="prevStep">{{ t('datasource.back') }}</t-button>
        <t-button theme="primary" :loading="submitting" @click="handleSubmit">
          {{ isEdit ? t('datasource.save') : t('datasource.createAndSync') }}
        </t-button>
      </div>
    </div>
  </t-dialog>
</template>

<style scoped>
.ds-steps {
  display: flex;
  gap: 4px;
  margin-bottom: 24px;
  border-bottom: 1px solid var(--td-border-level-2-color);
  padding-bottom: 16px;
}

.ds-step {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.ds-step.active { color: var(--td-brand-color); font-weight: 600; }
.ds-step.done { color: var(--td-success-color); }

.ds-step-num {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  border: 1px solid currentColor;
}

.ds-step.active .ds-step-num { background: var(--td-brand-color); color: #fff; border-color: var(--td-brand-color); }
.ds-step.done .ds-step-num { background: var(--td-success-color); color: #fff; border-color: var(--td-success-color); }

.ds-step-content { min-height: 200px; }

/* --- Step 0: type cards --- */
.ds-type-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}

.ds-type-card {
  border: 1px solid var(--td-border-level-2-color);
  border-radius: 8px;
  padding: 14px;
  cursor: pointer;
  transition: all 0.2s;
}

.ds-type-card:hover:not(.disabled) { border-color: var(--td-brand-color); background: var(--td-brand-color-light); }
.ds-type-card.disabled { opacity: 0.5; cursor: not-allowed; }

.ds-type-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.ds-type-name { font-size: 13px; font-weight: 600; }
.ds-type-soon { font-size: 10px; color: var(--td-text-color-placeholder); background: var(--td-bg-color-component); padding: 1px 6px; border-radius: 3px; }
.ds-type-desc { font-size: 11px; color: var(--td-text-color-secondary); line-height: 1.5; }

/* --- Step 1: collapsible prereq --- */
.ds-prereq-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  margin-bottom: 16px;
  border-radius: 6px;
  background: var(--td-warning-color-1);
  color: var(--td-warning-color);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  user-select: none;
  transition: background 0.15s;
}

.ds-prereq-bar:hover {
  background: var(--td-warning-color-2);
}

.ds-prereq-arrow {
  margin-left: auto;
}

.ds-prereq-detail {
  border: 1px solid var(--td-border-level-2-color);
  border-radius: 8px;
  padding: 14px;
  margin-bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.ds-storage-warning {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  margin-bottom: 16px;
  border-radius: 8px;
  background: var(--td-warning-color-1);
  color: var(--td-warning-color-7);
  border: 1px solid var(--td-warning-color-3);
}

.ds-storage-warning.is-highlighted,
.form-item.is-highlighted {
  box-shadow: 0 0 0 2px rgba(0, 82, 217, 0.16);
  border-radius: 10px;
  transition: box-shadow 0.2s ease;
}

.form-item.is-highlighted {
  padding: 10px 12px;
  margin: 0 -12px 12px;
  background: var(--td-brand-color-light);
}

.ds-storage-warning-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  line-height: 1.6;
}

.ds-storage-warning-title {
  font-weight: 600;
}

.ds-prereq-item {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.ds-prereq-num {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--td-brand-color);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-top: 1px;
}

.ds-prereq-item-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 20px;
}

.ds-prereq-item-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 2px;
  line-height: 1.5;
}

.ds-perm-tag {
  font-size: 11px;
  padding: 1px 5px;
  border-radius: 3px;
  background: var(--td-bg-color-component);
  color: var(--td-text-color-secondary);
  font-family: monospace;
  margin-right: 4px;
}

.ds-prereq-link {
  font-size: 12px;
  color: var(--td-brand-color);
  padding-left: 30px;
}

/* --- Step 1: doc link & form --- */
.ds-doc-link {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-component);
  padding: 8px 12px;
  border-radius: 6px;
  margin-bottom: 16px;
}

.ds-doc-link a {
  color: var(--td-brand-color);
  word-break: break-all;
}

.form-item { margin-bottom: 16px; }
.form-label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 6px; color: var(--td-text-color-primary); }
.form-tip { font-size: 12px; color: var(--td-text-color-placeholder); margin: 4px 0 12px; }
.form-actions { display: flex; align-items: center; gap: 8px; margin-top: 12px; }
.test-ok { color: var(--td-success-color); font-size: 13px; display: flex; align-items: center; gap: 4px; }

.test-error-box {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-top: 10px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--td-error-color-1);
  color: var(--td-error-color);
  font-size: 13px;
  line-height: 20px;
}

.test-error-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.test-error-title {
  font-weight: 500;
}

.test-error-detail {
  font-size: 12px;
  color: var(--td-error-color);
  opacity: 0.8;
  word-break: break-word;
}

.ds-dialog-footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 24px; padding-top: 16px; border-top: 1px solid var(--td-border-level-2-color); }

/* --- Step 2: resource list --- */
.ds-resource-list { max-height: 320px; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; }

.ds-resource-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
}

.ds-resource-row:hover {
  background: var(--td-bg-color-container-hover);
}

.ds-resource-row.selected {
  border-color: var(--td-brand-color);
  background: none;
}

.ds-resource-info {
  flex: 1;
  min-width: 0;
}

.ds-resource-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}

.ds-resource-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}

.ds-resource-type {
  font-size: 11px;
  padding: 0 5px;
  border-radius: 3px;
  background: var(--td-bg-color-component);
  color: var(--td-text-color-placeholder);
  line-height: 18px;
}

.ds-resource-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* --- Step 2: empty state --- */
.ds-resource-empty {
  text-align: center;
  padding: 24px 0;
}

.ds-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0 0 4px;
}

.ds-empty-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin: 0 0 16px;
}

.ds-guide-steps {
  display: flex;
  flex-direction: column;
  gap: 8px;
  text-align: left;
  max-width: 440px;
  margin: 0 auto 16px;
}

.ds-guide-step {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  line-height: 1.5;
}

.ds-guide-num {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  font-size: 11px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-top: 1px;
}

.ds-empty-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

.ds-doc-link-inline {
  color: var(--td-brand-color);
  font-size: 12px;
}
</style>

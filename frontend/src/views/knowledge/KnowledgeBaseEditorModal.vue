<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="visible" class="settings-overlay" @click.self="handleClose">
        <div class="settings-modal">
          <!-- 关闭按钮 -->
          <button class="close-btn" @click="handleClose" :aria-label="$t('general.close')">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
              <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            </svg>
          </button>

          <div class="settings-container">
            <!-- 左侧导航 -->
            <div class="settings-sidebar">
              <div class="sidebar-header">
                <h2 class="sidebar-title">{{ mode === 'create' ? $t('knowledgeEditor.titleCreate') : $t('knowledgeEditor.titleEdit') }}</h2>
              </div>
              <div class="settings-nav">
                <div 
                  v-for="(item, index) in navItems" 
                  :key="index"
                  :class="['nav-item', { 'active': currentSection === item.key }]"
                  @click="currentSection = item.key"
                >
                  <t-icon :name="item.icon" class="nav-icon" />
                  <span class="nav-label">{{ item.label }}</span>
                  <span v-if="item.badge" class="nav-badge">{{ item.badge }}</span>
                </div>
              </div>
            </div>

            <!-- 右侧内容区域 -->
            <div class="settings-content">
              <div class="content-wrapper">
                <!-- 基本信息 -->
                <div v-show="currentSection === 'basic'" class="section">
                  <div v-if="formData" class="section-content">
                    <div class="section-header">
                      <h3 class="section-title">{{ $t('knowledgeEditor.basic.title') }}</h3>
                      <p class="section-desc">{{ $t('knowledgeEditor.basic.description') }}</p>
                    </div>
                    <div class="section-body">
                      <div class="form-item">
                        <label class="form-label required">{{ $t('knowledgeEditor.basic.typeLabel') }}</label>
                        <t-radio-group
                          v-model="formData.type"
                          :disabled="mode === 'edit'"
                        >
                          <t-radio-button value="document">{{ $t('knowledgeEditor.basic.typeDocument') }}</t-radio-button>
                          <t-radio-button value="faq">{{ $t('knowledgeEditor.basic.typeFAQ') }}</t-radio-button>
                        </t-radio-group>
                        <p class="form-tip">{{ $t('knowledgeEditor.basic.typeDescription') }}</p>
                      </div>
                      <div class="form-item">
                        <label class="form-label required">{{ $t('knowledgeEditor.basic.nameLabel') }}</label>
                        <t-input 
                          v-model="formData.name" 
                          :placeholder="$t('knowledgeEditor.basic.namePlaceholder')"
                          :maxlength="50"
                        />
                      </div>
                      <div class="form-item">
                        <label class="form-label">{{ $t('knowledgeEditor.basic.descriptionLabel') }}</label>
                        <t-textarea 
                          v-model="formData.description" 
                          :placeholder="$t('knowledgeEditor.basic.descriptionPlaceholder')"
                          :maxlength="200"
                          :autosize="{ minRows: 3, maxRows: 6 }"
                        />
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 模型配置 -->
                <div v-if="hasVisitedSection('models')" v-show="currentSection === 'models'" class="section">
                  <KBModelConfig
                    ref="modelConfigRef"
                    v-if="formData"
                    :config="formData.modelConfig"
                    :has-files="hasFiles"
                    :all-models="allModels"
                    @update:config="handleModelConfigUpdate"
                  />
                </div>

                <!-- FAQ 配置 -->
                <div v-if="isFAQ && formData && hasVisitedSection('faq')" v-show="currentSection === 'faq'" class="section">
                  <div class="section-content">
                    <div class="section-header">
                      <h3 class="section-title">{{ $t('knowledgeEditor.faq.title') }}</h3>
                      <p class="section-desc">{{ $t('knowledgeEditor.faq.description') }}</p>
                    </div>
                    <div class="section-body">
                      <div class="form-item">
                        <label class="form-label required">{{ $t('knowledgeEditor.faq.indexModeLabel') }}</label>
                        <t-radio-group
                          v-model="formData.faqConfig.indexMode"
                        >
                          <t-radio-button value="question_only">{{ $t('knowledgeEditor.faq.modes.questionOnly') }}</t-radio-button>
                          <t-radio-button value="question_answer">{{ $t('knowledgeEditor.faq.modes.questionAnswer') }}</t-radio-button>
                        </t-radio-group>
                        <p class="form-tip">{{ $t('knowledgeEditor.faq.indexModeDescription') }}</p>
                      </div>
                      <div class="form-item">
                        <label class="form-label required">{{ $t('knowledgeEditor.faq.questionIndexModeLabel') }}</label>
                        <t-radio-group
                          v-model="formData.faqConfig.questionIndexMode"
                        >
                          <t-radio-button value="combined">{{ $t('knowledgeEditor.faq.modes.combined') }}</t-radio-button>
                          <t-radio-button value="separate">{{ $t('knowledgeEditor.faq.modes.separate') }}</t-radio-button>
                        </t-radio-group>
                        <p class="form-tip">{{ $t('knowledgeEditor.faq.questionIndexModeDescription') }}</p>
                      </div>
                      <div class="faq-guide">
                        <p>{{ $t('knowledgeEditor.faq.entryGuide') }}</p>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 解析引擎 -->
                <div v-if="!isFAQ && formData && hasVisitedSection('parser')" v-show="currentSection === 'parser'" class="section">
                  <KBParserSettings
                    :parser-engine-rules="formData.chunkingConfig.parserEngineRules"
                    @update:parser-engine-rules="handleParserEngineRulesUpdate"
                  />
                </div>

                <!-- 存储引擎 -->
                <div v-if="!isFAQ && formData && hasVisitedSection('storage')" v-show="currentSection === 'storage'" class="section">
                  <KBStorageSettings
                    :storage-provider="formData.storageProvider"
                    :has-files="mode === 'edit' && hasFiles"
                    @update:storage-provider="handleStorageProviderUpdate"
                  />
                </div>

                <!-- 分块设置 -->
                <div v-if="!isFAQ && hasVisitedSection('chunking')" v-show="currentSection === 'chunking'" class="section">
                  <KBChunkingSettings
                    v-if="formData"
                    :config="formData.chunkingConfig"
                    @update:config="handleChunkingConfigUpdate"
                  />
                </div>

                <!-- 图谱设置 -->
                <div v-if="!isFAQ && formData && hasVisitedSection('indexing')" v-show="currentSection === 'indexing'" class="section">
                  <div class="section-content kb-indexing-settings">
                    <div class="section-header">
                      <h3 class="section-title">索引策略</h3>
                      <p class="section-desc">按知识库控制检索索引能力，用于在召回质量、成本和导入速度之间做权衡。</p>
                    </div>
                    <div class="section-body">
                      <div class="setting-row">
                        <div class="setting-info">
                          <label>向量索引</label>
                          <p class="desc">启用语义召回。关闭后可不依赖 Embedding 检索。</p>
                        </div>
                        <t-switch v-model="formData.indexingStrategy.vector_enabled" />
                      </div>
                      <div class="setting-row">
                        <div class="setting-info">
                          <label>关键词索引</label>
                          <p class="desc">启用关键词/全文召回，适合制度编号、术语和精确短语。</p>
                        </div>
                        <t-switch v-model="formData.indexingStrategy.keyword_enabled" />
                      </div>
                      <div class="setting-row">
                        <div class="setting-info">
                          <label>Wiki 索引</label>
                          <p class="desc">预留给 Wiki Mode，后续阶段接入。</p>
                        </div>
                        <t-switch v-model="formData.indexingStrategy.wiki_enabled" />
                      </div>
                      <div class="setting-row disabled">
                        <div class="setting-info">
                          <label>知识图谱索引</label>
                          <p class="desc">预留给图谱索引管线，当前仅保留配置字段。</p>
                        </div>
                        <t-switch v-model="formData.indexingStrategy.knowledge_graph_enabled" disabled />
                      </div>
                      <div v-if="mode === 'edit' && kbId" class="setting-row wiki-entry-row">
                        <div class="setting-info">
                          <label>Wiki 工作区</label>
                          <p class="desc">启用 Wiki 索引后可进入页面浏览、图谱查看、索引页和重建链接。</p>
                        </div>
                        <div class="setting-control">
                          <t-button
                            theme="primary"
                            variant="outline"
                            :disabled="!canOpenWikiWorkspace"
                            @click="openWikiWorkspace"
                          >
                            进入 Wiki
                          </t-button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div v-if="!isFAQ && hasVisitedSection('graph')" v-show="currentSection === 'graph'" class="section">
                  <GraphSettings
                    v-if="formData"
                    :graph-extract="formData.nodeExtractConfig"
                    :model-id="formData.modelConfig.llmModelId"
                    :all-models="allModels"
                    @update:graphExtract="handleNodeExtractUpdate"
                  />
                </div>

                <!-- 多模态配置 -->
                <div v-if="!isFAQ && hasVisitedSection('multimodal')" v-show="currentSection === 'multimodal'" class="section">
                  <div v-if="formData" class="kb-multimodal-settings">
                    <div class="section-header">
                      <h2>{{ $t('knowledgeEditor.multimodal.title') }}</h2>
                      <p class="section-description">{{ $t('knowledgeEditor.multimodal.description') }}</p>
                    </div>

                    <div class="settings-group">
                      <!-- 多模态开关 -->
                      <div class="setting-row">
                        <div class="setting-info">
                          <label>{{ $t('knowledgeEditor.advanced.multimodal.label') }}</label>
                          <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.description') }}</p>
                        </div>
                        <div class="setting-control">
                          <t-switch
                            v-model="formData.multimodalConfig.enabled"
                            @change="handleMultimodalToggle"
                            size="medium"
                          />
                        </div>
                      </div>

                      <!-- VLLM 模型选择（多模态启用时） -->
                      <div v-if="formData.multimodalConfig.enabled" class="setting-row">
                        <div class="setting-info">
                          <label>{{ $t('knowledgeEditor.advanced.multimodal.vllmLabel') }} <span class="required">*</span></label>
                          <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.vllmDescription') }}</p>
                        </div>
                        <div class="setting-control">
                          <ModelSelector
                            model-type="VLLM"
                            :selected-model-id="formData.multimodalConfig.vllmModelId"
                            :all-models="allModels"
                            @update:selected-model-id="handleMultimodalVLLMChange"
                            @add-model="handleAddVLLMModel"
                            :placeholder="$t('knowledgeEditor.advanced.multimodal.vllmPlaceholder')"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 高级设置 -->
                <div v-if="!isFAQ && hasVisitedSection('advanced')" v-show="currentSection === 'advanced'" class="section">
                  <KBAdvancedSettings
                    ref="advancedSettingsRef"
                    v-if="formData"
                    :question-generation="formData.questionGenerationConfig"
                    :all-models="allModels"
                    @update:question-generation="handleQuestionGenerationUpdate"
                  />
                </div>

                <!-- 数据源管理（仅编辑模式） -->
                <div v-if="mode === 'edit' && kbId && hasVisitedSection('datasource')" v-show="currentSection === 'datasource'" class="section">
                  <DataSourceSettings :kb-id="kbId" @count="dsCount = $event" />
                </div>

                <!-- 共享设置（仅编辑模式） -->
                <div v-if="mode === 'edit' && kbId && hasVisitedSection('share')" v-show="currentSection === 'share'" class="section">
                  <KBShareSettings :kb-id="kbId" />
                </div>
              </div>

              <!-- 保存按钮 -->
              <div class="settings-footer">
                <t-button theme="default" variant="outline" @click="handleClose">
                  {{ $t('common.cancel') }}
                </t-button>
                <t-button theme="primary" @click="handleSubmit" :loading="saving">
                  {{ mode === 'create' ? $t('knowledgeEditor.buttons.create') : $t('knowledgeEditor.buttons.save') }}
                </t-button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { defineAsyncComponent, ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { DialogPlugin } from 'tdesign-vue-next/es/dialog'
import { MessagePlugin } from 'tdesign-vue-next/es/message'
import { createKnowledgeBase, getKnowledgeBaseById, listKnowledgeFiles, updateKnowledgeBase } from '@/api/knowledge-base'
import { updateKBConfig, type KBModelConfigRequest } from '@/api/initialization'
import { listModels, type ModelConfig } from '@/api/model'
import { useUIStore } from '@/stores/ui'
import ModelSelector from '@/components/ModelSelector.vue'
import { useI18n } from 'vue-i18n'

const KBModelConfig = defineAsyncComponent(() => import('./settings/KBModelConfig.vue'))
const KBParserSettings = defineAsyncComponent(() => import('./settings/KBParserSettings.vue'))
const KBStorageSettings = defineAsyncComponent(() => import('./settings/KBStorageSettings.vue'))
const KBChunkingSettings = defineAsyncComponent(() => import('./settings/KBChunkingSettings.vue'))
const KBAdvancedSettings = defineAsyncComponent(() => import('./settings/KBAdvancedSettings.vue'))
const GraphSettings = defineAsyncComponent(() => import('./settings/GraphSettings.vue'))
const KBShareSettings = defineAsyncComponent(() => import('./settings/KBShareSettings.vue'))
const DataSourceSettings = defineAsyncComponent(() => import('./settings/DataSourceSettings.vue'))

const uiStore = useUIStore()
const router = useRouter()
const { t } = useI18n()

// Props
const props = defineProps<{
  visible: boolean
  mode: 'create' | 'edit'
  kbId?: string
  initialType?: 'document' | 'faq'
}>()

// Emits
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'success', kbId: string): void
}>()

const currentSection = ref<string>('basic')
const visitedSections = ref(new Set<string>(['basic']))
const saving = ref(false)
const loading = ref(false)
const allModels = ref<ModelConfig[]>([])
const hasFiles = ref(false)
const initialStorageProvider = ref<string>('')
const dsCount = ref(0)
const persistedWikiEnabled = ref(false)

const navItems = computed(() => {
  const items: { key: string; icon: string; label: string; badge?: number }[] = [
    { key: 'basic', icon: 'info-circle', label: '基本信息' },
    { key: 'models', icon: 'control-platform', label: '模型配置' }
  ]
  if (formData.value?.type === 'faq') {
    items.push({ key: 'faq', icon: 'help-circle', label: 'FAQ 配置' })
  } else {
    items.push(
      { key: 'parser', icon: 'file-search', label: '解析引擎' },
      { key: 'storage', icon: 'cloud', label: '存储引擎' },
      { key: 'chunking', icon: 'file-copy', label: '分块设置' },
      { key: 'indexing', icon: 'database', label: '索引策略' },
      { key: 'graph', icon: 'chart-bubble', label: '图谱设置' },
      { key: 'multimodal', icon: 'image', label: '多模态配置' },
      { key: 'advanced', icon: 'setting', label: '高级设置' }
    )
    if (props.mode === 'edit' && props.kbId) {
      items.push({ key: 'datasource', icon: 'cloud-download', label: '数据源管理', badge: dsCount.value || undefined })
    }
  }
  if (props.mode === 'edit' && props.kbId) {
    items.push({ key: 'share', icon: 'share', label: '共享设置' })
  }
  return items
})

// 模型配置引用
const modelConfigRef = ref<any>()
const advancedSettingsRef = ref<any>()

// 表单数据
const formData = ref<any>(null)
const isFAQ = computed(() => formData.value?.type === 'faq')
const hasVisitedSection = (section: string) => visitedSections.value.has(section)
type RequiredModelType = 'KnowledgeQA' | 'Embedding'

const markSectionVisited = (section: string) => {
  if (!section || visitedSections.value.has(section)) return
  visitedSections.value = new Set([...visitedSections.value, section])
}

watch(currentSection, (section) => {
  markSectionVisited(section)
})

watch(
  () => formData.value?.type,
  (newType, oldType) => {
    if (!formData.value) return
    if (newType === 'faq') {
      if (!formData.value.faqConfig) {
        formData.value.faqConfig = { indexMode: 'question_only', questionIndexMode: 'separate' }
      }
      if (!['basic', 'models', 'faq'].includes(currentSection.value)) {
        currentSection.value = 'faq'
      }
    } else if (oldType === 'faq' && currentSection.value === 'faq') {
      currentSection.value = 'basic'
    }
  }
)

// 初始化表单数据
const initFormData = (type: 'document' | 'faq' = 'document') => {
  return {
    type,
    name: '',
    description: '',
    faqConfig: {
      indexMode: 'question_only',
      questionIndexMode: 'separate'
    },
    modelConfig: {
      llmModelId: '',
      embeddingModelId: ''
    },
    chunkingConfig: {
      chunkSize: 512,
      chunkOverlap: 100,
      separators: ['\n\n', '\n', '。', '！', '？', ';', '；'],
      parserEngineRules: undefined as any,
      enableParentChild: true,
      parentChunkSize: 4096,
      childChunkSize: 384,
      strategy: 'legacy',
      tokenLimit: 0,
      languages: [] as string[]
    },
    storageProvider: '' as string,
    multimodalConfig: {
      enabled: false,
      vllmModelId: ''
    },
    nodeExtractConfig: {
      enabled: false,
      text: '',
      tags: [] as string[],
      nodes: [] as Array<{
        name: string
        attributes: string[]
      }>,
      relations: [] as Array<{
        node1: string
        node2: string
        type: string
      }>
    },
    questionGenerationConfig: {
      enabled: true,
      questionCount: 3
    },
    indexingStrategy: {
      vector_enabled: true,
      keyword_enabled: true,
      wiki_enabled: false,
      knowledge_graph_enabled: false
    },
  }
}

// 加载所有模型
const pickDefaultModelId = (type: RequiredModelType) => {
  const candidates = allModels.value.filter((model) => model.type === type && model.id)
  return (
    candidates.find((model) => model.is_default)?.id ||
    candidates.find((model) => model.is_platform || model.is_builtin)?.id ||
    candidates.find((model) => model.is_builtin)?.id ||
    candidates[0]?.id ||
    ''
  )
}

const applyCreateDefaultModels = () => {
  if (props.mode !== 'create' || !formData.value) return

  const modelConfig = { ...formData.value.modelConfig }
  if (!modelConfig.llmModelId) {
    modelConfig.llmModelId = pickDefaultModelId('KnowledgeQA')
  }
  if (!modelConfig.embeddingModelId) {
    modelConfig.embeddingModelId = pickDefaultModelId('Embedding')
  }
  formData.value.modelConfig = modelConfig
}

const loadAllModels = async () => {
  try {
    const models = await listModels()
    allModels.value = models || []
    applyCreateDefaultModels()
  } catch (error) {
    console.error('Failed to load model list:', error)
    MessagePlugin.error(t('knowledgeEditor.messages.loadModelsFailed'))
    allModels.value = []
  }
}

// 加载知识库数据（编辑模式）
const loadKBData = async () => {
  if (props.mode !== 'edit' || !props.kbId) return
  
  loading.value = true
  try {
    const [kbInfo, models, filesResult] = await Promise.all([
      getKnowledgeBaseById(props.kbId),
      loadAllModels(),
      listKnowledgeFiles(props.kbId, { page: 1, page_size: 1 })
    ])
    
    if (!kbInfo || !kbInfo.data) {
      throw new Error(t('knowledgeEditor.messages.notFound'))
    }

    const kb = kbInfo.data
    hasFiles.value = (filesResult as any)?.total > 0
    
    // 设置表单数据
    const kbType = (kb.type as 'document' | 'faq') || 'document'
    formData.value = {
      type: kbType,
      name: kb.name || '',
      description: kb.description || '',
      faqConfig: {
        indexMode: kb.faq_config?.index_mode || 'question_only',
        questionIndexMode: kb.faq_config?.question_index_mode || 'separate'
      },
      modelConfig: {
        llmModelId: kb.summary_model_id || '',
        embeddingModelId: kb.embedding_model_id || ''
      },
      chunkingConfig: {
        chunkSize: kb.chunking_config?.chunk_size || 512,
        chunkOverlap: kb.chunking_config?.chunk_overlap || 100,
        separators: kb.chunking_config?.separators || ['\n\n', '\n', '。', '！', '？', ';', '；'],
        parserEngineRules: kb.chunking_config?.parser_engine_rules || undefined,
        enableParentChild: kb.chunking_config?.enable_parent_child || false,
        parentChunkSize: kb.chunking_config?.parent_chunk_size || 4096,
        childChunkSize: kb.chunking_config?.child_chunk_size || 384,
        strategy: kb.chunking_config?.strategy || 'legacy',
        tokenLimit: kb.chunking_config?.token_limit || 0,
        languages: kb.chunking_config?.languages || []
      },
      storageProvider: (kb.storage_provider_config?.provider || kb.storage_config?.provider || 'local') as string,
      multimodalConfig: {
        enabled: !!kb.vlm_config?.enabled,
        vllmModelId: kb.vlm_config?.model_id || ''
      },
      nodeExtractConfig: {
        enabled: kb.extract_config?.enabled || false,
        text: kb.extract_config?.text || '',
        tags: kb.extract_config?.tags || [],
        nodes: (kb.extract_config?.nodes || []).map((node: any) => ({
          name: node.name,
          attributes: node.attributes || []
        })),
        relations: kb.extract_config?.relations || []
      },
      questionGenerationConfig: {
        enabled: kb.question_generation_config?.enabled || false,
        questionCount: kb.question_generation_config?.question_count || 3
      },
      indexingStrategy: {
        vector_enabled: kb.indexing_strategy?.vector_enabled ?? true,
        keyword_enabled: kb.indexing_strategy?.keyword_enabled ?? true,
        wiki_enabled: kb.indexing_strategy?.wiki_enabled ?? false,
        knowledge_graph_enabled: kb.indexing_strategy?.graph_enabled ?? kb.indexing_strategy?.knowledge_graph_enabled ?? false
      },
    }
    initialStorageProvider.value = formData.value.storageProvider
    persistedWikiEnabled.value = !!kb.indexing_strategy?.wiki_enabled
  } catch (error) {
    console.error('Failed to load knowledge base data:', error)
    MessagePlugin.error(t('knowledgeEditor.messages.loadDataFailed'))
    handleClose()
  } finally {
    loading.value = false
  }
}

// 处理配置更新
const handleModelConfigUpdate = (config: any) => {
  if (formData.value) {
    formData.value.modelConfig = { ...config }
  }
}

const handleChunkingConfigUpdate = (config: any) => {
  if (formData.value) {
    formData.value.chunkingConfig = { ...config }
  }
}

const handleParserEngineRulesUpdate = (rules: any[]) => {
  if (formData.value) {
    formData.value.chunkingConfig.parserEngineRules = rules?.length ? rules : undefined
  }
}

const handleMultimodalToggle = () => {
  if (formData.value && !formData.value.multimodalConfig.enabled) {
    formData.value.multimodalConfig.vllmModelId = ''
  }
}

const handleMultimodalVLLMChange = (modelId: string) => {
  if (formData.value) {
    formData.value.multimodalConfig.vllmModelId = modelId
  }
}

const handleAddVLLMModel = () => {
  uiStore.openSettings('models', 'vllm')
}

const canOpenWikiWorkspace = computed(() => {
  return props.mode === 'edit' && !!props.kbId && persistedWikiEnabled.value
})

const openWikiWorkspace = async () => {
  if (!props.kbId || !canOpenWikiWorkspace.value) {
    MessagePlugin.info('请先启用并保存 Wiki 索引后再进入工作区')
    return
  }
  await router.push(`/platform/knowledge-bases/${props.kbId}/wiki`)
  handleClose()
}

const guideModelSetup = (subSection: 'chat' | 'embedding') => {
  currentSection.value = 'models'
  markSectionVisited('models')
  uiStore.openSettings('models', subSection)
}

const handleStorageProviderUpdate = (value: string) => {
  if (formData.value) {
    formData.value.storageProvider = value || 'local'
  }
}

const handleQuestionGenerationUpdate = (config: any) => {
  if (formData.value) {
    formData.value.questionGenerationConfig = { ...config }
  }
}

const handleNodeExtractUpdate = (config: any) => {
  if (formData.value) {
    formData.value.nodeExtractConfig = { ...config }
  }
}

// 验证表单
const validateForm = (): boolean => {
  if (!formData.value) return false

  // 验证基本信息
  if (!formData.value.name || !formData.value.name.trim()) {
    MessagePlugin.warning(t('knowledgeEditor.messages.nameRequired'))
    currentSection.value = 'basic'
    return false
  }

  // 验证模型配置 - 必须配置 embedding 和 summary 模型
  if (
    !formData.value.indexingStrategy?.vector_enabled &&
    !formData.value.indexingStrategy?.keyword_enabled &&
    !formData.value.indexingStrategy?.wiki_enabled &&
    !formData.value.indexingStrategy?.knowledge_graph_enabled
  ) {
    MessagePlugin.warning('请至少启用一种索引方式')
    currentSection.value = 'indexing'
    return false
  }

  if (formData.value.indexingStrategy?.vector_enabled && !formData.value.modelConfig.embeddingModelId) {
    MessagePlugin.warning(t('knowledgeEditor.messages.embeddingRequired'))
    guideModelSetup('embedding')
    return false
  }

  if (!formData.value.modelConfig.llmModelId) {
    MessagePlugin.warning(t('knowledgeEditor.messages.summaryRequired'))
    guideModelSetup('chat')
    return false
  }

  // 验证多模态配置（如果启用）
  if (formData.value.multimodalConfig.enabled && !formData.value.multimodalConfig.vllmModelId) {
    MessagePlugin.warning(t('knowledgeEditor.messages.multimodalInvalid'))
    currentSection.value = 'multimodal'
    return false
  }

  if (formData.value.type === 'faq' && !formData.value.faqConfig?.indexMode) {
    MessagePlugin.warning(t('knowledgeEditor.messages.indexModeRequired'))
    currentSection.value = 'faq'
    return false
  }

  return true
}

// 构建提交数据
const buildSubmitData = () => {
  if (!formData.value) return null

  const data: any = {
    name: formData.value.name,
    description: formData.value.description,
    type: formData.value.type,
    chunking_config: {
      chunk_size: formData.value.chunkingConfig.chunkSize,
      chunk_overlap: formData.value.chunkingConfig.chunkOverlap,
      separators: formData.value.chunkingConfig.separators,
      enable_multimodal: formData.value.multimodalConfig.enabled,
      enable_parent_child: formData.value.chunkingConfig.enableParentChild,
      parent_chunk_size: formData.value.chunkingConfig.parentChunkSize,
      child_chunk_size: formData.value.chunkingConfig.childChunkSize,
      strategy: formData.value.chunkingConfig.strategy || 'legacy',
      token_limit: formData.value.chunkingConfig.tokenLimit || 0,
      languages: formData.value.chunkingConfig.languages || [],
      ...(formData.value.chunkingConfig.parserEngineRules?.length
        ? { parser_engine_rules: formData.value.chunkingConfig.parserEngineRules }
        : {})
    },
    embedding_model_id: formData.value.modelConfig.embeddingModelId,
    summary_model_id: formData.value.modelConfig.llmModelId,
    indexing_strategy: {
      vector_enabled: !!formData.value.indexingStrategy?.vector_enabled,
      keyword_enabled: !!formData.value.indexingStrategy?.keyword_enabled,
      wiki_enabled: !!formData.value.indexingStrategy?.wiki_enabled,
      graph_enabled: !!formData.value.indexingStrategy?.knowledge_graph_enabled,
      knowledge_graph_enabled: !!formData.value.indexingStrategy?.knowledge_graph_enabled
    },
    wiki_config: {
      synthesis_model_id: formData.value.modelConfig.llmModelId || '',
      max_pages_per_ingest: 25,
      extraction_granularity: 'standard'
    }
  }

  // 添加多模态配置
  data.vlm_config = {
    enabled: formData.value.multimodalConfig.enabled,
    model_id: formData.value.multimodalConfig.enabled
      ? (formData.value.multimodalConfig.vllmModelId || '')
      : ''
  }

  // 存储引擎：仅传 provider，参数从全局设置读取
  // Write to storage_provider_config (authoritative) + storage_config (legacy dual-write)
  data.storage_provider_config = {
    provider: formData.value.storageProvider || 'local'
  }
  data.storage_config = {
    provider: formData.value.storageProvider || 'local'
  }

  // 添加知识图谱配置
  if (formData.value.nodeExtractConfig.enabled) {
    data.extract_config = {
      enabled: true,
      text: formData.value.nodeExtractConfig.text,
      tags: formData.value.nodeExtractConfig.tags,
      nodes: formData.value.nodeExtractConfig.nodes,
      relations: formData.value.nodeExtractConfig.relations
    }
  }

  // 添加问题生成配置
  if (formData.value.questionGenerationConfig?.enabled) {
    data.question_generation_config = {
      enabled: true,
      question_count: formData.value.questionGenerationConfig.questionCount || 3
    }
  }

  if (formData.value.type === 'faq') {
    data.faq_config = {
      index_mode: formData.value.faqConfig?.indexMode || 'question_only',
      question_index_mode: formData.value.faqConfig?.questionIndexMode || 'separate'
    }
  }

  return data
}

// 提交表单
const handleSubmit = async () => {
  if (!validateForm()) {
    return
  }

  // 编辑模式下，若已有文件且存储引擎发生了变化，弹窗确认
  if (
    props.mode === 'edit' &&
    hasFiles.value &&
    formData.value &&
    initialStorageProvider.value &&
    formData.value.storageProvider !== initialStorageProvider.value
  ) {
    const dialog = DialogPlugin.confirm({
      header: t('common.confirm'),
      body: t('knowledgeEditor.messages.storageChangeConfirm'),
      confirmBtn: t('common.confirm'),
      cancelBtn: t('common.cancel'),
      onConfirm: () => {
        dialog.destroy()
        doSubmit()
      },
      onCancel: () => {
        dialog.destroy()
      },
    })
    return
  }

  doSubmit()
}

const doSubmit = async () => {
  saving.value = true
  try {
    const data = buildSubmitData()
    if (!data) {
      throw new Error(t('knowledgeEditor.messages.buildDataFailed'))
    }

    if (props.mode === 'create') {
      // 创建模式：一次性创建知识库及所有配置
      const result: any = await createKnowledgeBase(data)
      if (!result.success || !result.data?.id) {
        throw new Error(result.message || t('knowledgeEditor.messages.createFailed'))
      }
      MessagePlugin.success(t('knowledgeEditor.messages.createSuccess'))
      emit('success', result.data.id)
    } else {
      // 编辑模式：分别更新基本信息和配置
      if (!props.kbId) {
        throw new Error(t('knowledgeEditor.messages.missingId'))
      }

      // 1. 更新基本信息（名称、描述）和 FAQ 配置
      const updateConfig: any = {}
      if (formData.value.type === 'faq' && formData.value.faqConfig) {
        updateConfig.faq_config = {
          index_mode: formData.value.faqConfig.indexMode || 'question_only',
          question_index_mode: formData.value.faqConfig.questionIndexMode || 'separate'
        }
      }
      updateConfig.indexing_strategy = data.indexing_strategy
      updateConfig.wiki_config = data.wiki_config
      await updateKnowledgeBase(props.kbId, {
        name: data.name,
        description: data.description,
        config: updateConfig
      })

      // 2. 更新完整配置（模型、分块、多模态、存储引擎、知识图谱等）
      const config: KBModelConfigRequest = {
        llmModelId: data.summary_model_id,
        embeddingModelId: data.embedding_model_id,
        vlm_config: data.vlm_config,
        documentSplitting: {
          chunkSize: data.chunking_config.chunk_size,
          chunkOverlap: data.chunking_config.chunk_overlap,
          separators: data.chunking_config.separators,
          parserEngineRules: data.chunking_config.parser_engine_rules || undefined,
          enableParentChild: data.chunking_config.enable_parent_child || false,
          parentChunkSize: data.chunking_config.parent_chunk_size || 4096,
          childChunkSize: data.chunking_config.child_chunk_size || 384
        },
        multimodal: {
          enabled: !!data.vlm_config?.enabled
        },
        storageProvider: data.storage_provider_config?.provider || data.storage_config?.provider || 'local',
        indexingStrategy: data.indexing_strategy,
        nodeExtract: {
          enabled: data.extract_config?.enabled || false,
          text: data.extract_config?.text || '',
          tags: data.extract_config?.tags || [],
          nodes: data.extract_config?.nodes || [],
          relations: data.extract_config?.relations || []
        },
        questionGeneration: {
          enabled: data.question_generation_config?.enabled || false,
          questionCount: data.question_generation_config?.question_count || 3
        }
      }

      await updateKBConfig(props.kbId, config)
      MessagePlugin.success(t('knowledgeEditor.messages.updateSuccess'))
      emit('success', props.kbId)
    }
    
    handleClose()
  } catch (error: any) {
    console.error('Knowledge base operation failed:', error)
    MessagePlugin.error(error?.message || t('common.operationFailed'))
  } finally {
    saving.value = false
  }
}

// 重置所有状态
const resetState = () => {
  currentSection.value = 'basic'
  visitedSections.value = new Set(['basic'])
  formData.value = null
  hasFiles.value = false
  initialStorageProvider.value = ''
  persistedWikiEnabled.value = false
  saving.value = false
  loading.value = false
}

// 关闭弹窗
const handleClose = () => {
  emit('update:visible', false)
  setTimeout(() => {
    resetState()
  }, 300)
}

// 监听弹窗打开/关闭
watch(() => props.visible, async (newVal) => {
  if (newVal) {
    // 打开弹窗时，先重置状态
    resetState()
    
    // 检查是否有初始 section，如果有则跳转
    if (uiStore.kbEditorInitialSection) {
      currentSection.value = uiStore.kbEditorInitialSection
      markSectionVisited(uiStore.kbEditorInitialSection)
    }
    
    // 加载模型列表
    await loadAllModels()
    
    // 根据模式加载数据
    if (props.mode === 'edit' && props.kbId) {
      await loadKBData()
    } else {
      // 创建模式：初始化空表单
      formData.value = initFormData(props.initialType || 'document')
      applyCreateDefaultModels()
      hasFiles.value = false
    }
  } else {
    // 关闭弹窗时，延迟重置状态（等待动画结束）
    setTimeout(() => {
      resetState()
      currentSection.value = 'basic' // 重置为默认 section
    }, 300)
  }
})

// 监听全局设置弹窗关闭后刷新模型列表
watch(
  () => uiStore.showSettingsModal,
  async (visible, previous) => {
    if (!visible && previous && props.visible) {
      await loadAllModels()
    }
  }
)

watch(
  () => uiStore.kbEditorInitialSection,
  (section) => {
    if (!props.visible || !section || currentSection.value === section) {
      return
    }
    currentSection.value = section
    markSectionVisited(section)
  }
)
</script>

<style scoped lang="less">
// 复用创建知识库的样式
.settings-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

.settings-modal {
  position: relative;
  width: 90vw;
  max-width: 1000px;
  height: 85vh;
  max-height: 750px;
  background: var(--td-bg-color-container);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.close-btn {
  position: absolute;
  top: 20px;
  right: 20px;
  width: 32px;
  height: 32px;
  border: none;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-secondary);
  transition: all 0.2s ease;
  z-index: 10;

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-primary);
  }
}

.settings-container {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.settings-sidebar {
  width: 200px;
  background: var(--td-bg-color-settings-modal);
  border-right: 1px solid var(--td-component-stroke);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-header {
  padding: 24px 20px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.sidebar-title {
  margin: 0;
  font-family: "PingFang SC";
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.settings-nav {
  flex: 1;
  padding: 12px 8px;
  overflow-y: auto;
}

.nav-item {
  display: flex;
  align-items: center;
  padding: 10px 12px;
  margin-bottom: 4px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
  font-family: "PingFang SC";
  font-size: 14px;
  color: var(--td-text-color-secondary);

  &:hover {
    background: var(--td-bg-color-secondarycontainer-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background: var(--td-brand-color-light);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.nav-icon {
  margin-right: 8px;
  font-size: 18px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.nav-label {
  flex: 1;
}

.nav-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  font-size: 11px;
  font-weight: 600;
  background: var(--td-bg-color-component);
  color: var(--td-text-color-secondary);
  line-height: 1;
  flex-shrink: 0;
}

.nav-item.active .nav-badge {
  background: var(--td-brand-color);
  color: #fff;
}

.settings-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content-wrapper {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
}

.section {
  margin-bottom: 32px;

  &:last-child {
    margin-bottom: 0;
  }
}

.section-content {
  .section-header {
    margin-bottom: 20px;
  }

  .section-title {
    margin: 0 0 8px 0;
    font-family: "PingFang SC";
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .section-desc {
    margin: 0;
    font-family: "PingFang SC";
    font-size: 14px;
    color: var(--td-text-color-placeholder);
    line-height: 22px;
  }

  .section-body {
    background: var(--td-bg-color-container);
  }
}

.form-item {
  margin-bottom: 20px;

  &:last-child {
    margin-bottom: 0;
  }
}

.form-label {
  display: block;
  margin-bottom: 8px;
  font-family: "PingFang SC";
  font-size: 15px;
  font-weight: 500;
  color: var(--td-text-color-primary);

  &.required::after {
    content: '*';
    color: var(--td-error-color);
    margin-left: 4px;
  }
}

.form-tip {
  margin-top: 6px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.faq-guide {
  margin-top: 20px;
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 20px;
}

.kb-indexing-settings {
  .setting-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    padding: 18px 0;
    border-bottom: 1px solid var(--td-component-stroke);

    &:last-child {
      border-bottom: none;
    }

    &.disabled {
      opacity: 0.65;
    }
  }

  .setting-info {
    flex: 1;

    label {
      display: block;
      margin-bottom: 4px;
      font-size: 15px;
      font-weight: 500;
      color: var(--td-text-color-primary);
    }

    .desc {
      margin: 0;
      font-size: 13px;
      line-height: 1.5;
      color: var(--td-text-color-secondary);
    }
  }
}

.settings-footer {
  padding: 16px 32px;
  border-top: 1px solid var(--td-component-stroke);
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  flex-shrink: 0;
}

// 过渡动画
.modal-enter-active,
.modal-leave-active {
  transition: all 0.3s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;

  .settings-modal {
    transform: scale(0.95);
  }
}

// Radio-group 样式优化，符合项目主题风格
:deep(.t-radio-group) {
  .t-radio-group--filled {
    background: var(--td-bg-color-secondarycontainer);
  }
  .t-radio-button {
    border-color: var(--td-component-stroke);
    // color: var(--td-text-color-placeholder);

    &:hover:not(.t-is-disabled) {
      border-color: var(--td-brand-color);
      color: var(--td-brand-color);
    }

    &.t-is-checked {
      background: var(--td-brand-color);
      border-color: var(--td-brand-color);
      color: var(--td-text-color-anti);

      &:hover:not(.t-is-disabled) {
        background: var(--td-brand-color-active);
        border-color: var(--td-brand-color-active);
        color: var(--td-text-color-anti);
      }
    }

    // 禁用状态样式
    &.t-is-disabled {
      background: var(--td-bg-color-secondarycontainer);
      border-color: var(--td-component-stroke);
      color: var(--td-text-color-disabled);
      cursor: not-allowed;
      opacity: 0.6;

      &.t-is-checked {
        background: var(--td-bg-color-secondarycontainer);
        border-color: var(--td-component-stroke);
        color: var(--td-text-color-placeholder);
      }
    }
  }
}

// 多模态配置内联样式（与子组件 KBStorageSettings/KBAdvancedSettings 一致）
.kb-multimodal-settings {
  width: 100%;

  .section-header {
    margin-bottom: 32px;

    h2 {
      font-size: 20px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      margin: 0 0 8px 0;
    }

    .section-description {
      font-size: 14px;
      color: var(--td-text-color-secondary);
      margin: 0;
      line-height: 1.5;
    }
  }

  .settings-group {
    display: flex;
    flex-direction: column;
  }

  .setting-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    padding: 20px 0;
    border-bottom: 1px solid var(--td-component-stroke);

    &:last-child {
      border-bottom: none;
    }
  }

  .setting-info {
    flex: 1;
    max-width: 65%;
    padding-right: 24px;

    label {
      font-size: 15px;
      font-weight: 500;
      color: var(--td-text-color-primary);
      display: block;
      margin-bottom: 4px;
    }

    .desc {
      font-size: 13px;
      color: var(--td-text-color-secondary);
      margin: 0;
      line-height: 1.5;
    }
  }

  .setting-control {
    flex-shrink: 0;
    min-width: 280px;
    display: flex;
    justify-content: flex-end;
    align-items: center;
  }

  .required {
    color: var(--td-error-color);
    margin-left: 2px;
    font-weight: 500;
  }
}

.wiki-entry-row {
  .setting-control {
    min-width: 160px;
  }
}
</style>


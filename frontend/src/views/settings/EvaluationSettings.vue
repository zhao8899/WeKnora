<template>
  <div class="evaluation-settings">
    <div class="section-header">
      <div>
        <h2>{{ titleText }}</h2>
        <p class="section-description">{{ subtitleText }}</p>
      </div>
      <div class="header-actions">
        <t-button variant="outline" @click="loadCurrentTask">
          {{ loadTaskText }}
        </t-button>
        <t-button theme="primary" @click="runEvaluation">
          {{ runText }}
        </t-button>
      </div>
    </div>

    <div class="panel-grid">
      <section class="panel">
        <div class="panel-header">
          <h3>{{ formTitleText }}</h3>
        </div>
        <div class="form-grid">
          <label class="field">
            <span>{{ datasetLabelText }}</span>
            <t-input v-model="form.dataset_id" :placeholder="datasetPlaceholderText" />
          </label>
          <label class="field">
            <span>{{ knowledgeBaseLabelText }}</span>
            <t-input v-model="form.knowledge_base_id" :placeholder="knowledgeBasePlaceholderText" />
          </label>
          <label class="field">
            <span>{{ chatModelLabelText }}</span>
            <t-input v-model="form.chat_id" :placeholder="chatModelPlaceholderText" />
          </label>
          <label class="field">
            <span>{{ rerankModelLabelText }}</span>
            <t-input v-model="form.rerank_id" :placeholder="rerankModelPlaceholderText" />
          </label>
        </div>
      </section>

      <section class="panel">
        <div class="panel-header">
          <h3>{{ taskTitleText }}</h3>
          <t-tag v-if="task" :theme="taskTheme(task.task.status)" variant="light">
            {{ taskStatusText(task.task.status) }}
          </t-tag>
        </div>

        <div class="task-toolbar">
          <t-input v-model="taskIdInput" :placeholder="taskIdPlaceholderText" />
          <t-button variant="outline" :loading="loadingTask" @click="loadCurrentTask">
            {{ loadTaskText }}
          </t-button>
        </div>

        <div v-if="!task && !loadingTask" class="panel-empty">
          {{ noResultText }}
        </div>

        <template v-else>
          <div v-if="task" class="task-summary">
            <div class="summary-item">
              <span>{{ taskIdText }}</span>
              <strong>{{ task.task.id }}</strong>
            </div>
            <div class="summary-item">
              <span>{{ progressText }}</span>
              <strong>{{ task.task.finished || 0 }}/{{ task.task.total || 0 }}</strong>
            </div>
            <div class="summary-item">
              <span>{{ startTimeText }}</span>
              <strong>{{ formatTime(task.task.start_time) }}</strong>
            </div>
            <div v-if="task.task.err_msg" class="summary-item summary-error">
              <span>{{ errorText }}</span>
              <strong>{{ task.task.err_msg }}</strong>
            </div>
          </div>

          <div v-if="task && task.metric" class="metric-groups">
            <div class="metric-group">
              <h4>{{ retrievalMetricsText }}</h4>
              <div class="metric-grid">
                <div v-for="item in retrievalMetricCards" :key="item.key" class="metric-card">
                  <span>{{ item.label }}</span>
                  <strong>{{ formatPercent(item.value) }}</strong>
                </div>
              </div>
            </div>

            <div class="metric-group">
              <h4>{{ generationMetricsText }}</h4>
              <div class="metric-grid">
                <div v-for="item in generationMetricCards" :key="item.key" class="metric-card">
                  <span>{{ item.label }}</span>
                  <strong>{{ formatPercent(item.value) }}</strong>
                </div>
              </div>
            </div>
          </div>
        </template>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next/es/message'
import { useI18n } from 'vue-i18n'
import { getEvaluationResult, startEvaluation, type EvaluationDetail } from '@/api/evaluation'

const { t, locale } = useI18n()

const form = reactive({
  dataset_id: '',
  knowledge_base_id: '',
  chat_id: '',
  rerank_id: '',
})
const task = ref<EvaluationDetail | null>(null)
const taskIdInput = ref('')
const loadingTask = ref(false)
const pollingTimer = ref<number | null>(null)

const fallbackText = (key: string, zh: string, en: string) => {
  const translated = t(key)
  if (translated !== key) return translated
  return locale.value.startsWith('zh') ? zh : en
}

const titleText = fallbackText('settings.evaluation.title', '评测与验收', 'Evaluation and Acceptance')
const subtitleText = fallbackText(
  'settings.evaluation.subtitle',
  '发起试点评测并查看检索与生成指标，作为上线前验收入口。',
  'Run trial evaluations and review retrieval and generation metrics before launch.',
)
const runText = fallbackText('settings.evaluation.run', '发起评测', 'Run evaluation')
const loadTaskText = fallbackText('settings.evaluation.loadTask', '加载任务', 'Load task')
const formTitleText = fallbackText('settings.evaluation.formTitle', '评测参数', 'Evaluation inputs')
const taskTitleText = fallbackText('settings.evaluation.taskTitle', '评测结果', 'Evaluation result')
const datasetLabelText = fallbackText('settings.evaluation.datasetId', '数据集 ID', 'Dataset ID')
const knowledgeBaseLabelText = fallbackText('settings.evaluation.knowledgeBaseId', '知识库 ID（可选）', 'Knowledge base ID (optional)')
const chatModelLabelText = fallbackText('settings.evaluation.chatModelId', 'Chat 模型 ID（可选）', 'Chat model ID (optional)')
const rerankModelLabelText = fallbackText('settings.evaluation.rerankModelId', 'Rerank 模型 ID（可选）', 'Rerank model ID (optional)')
const taskIdText = fallbackText('settings.evaluation.taskId', '任务 ID', 'Task ID')
const taskIdPlaceholderText = fallbackText('settings.evaluation.taskIdPlaceholder', '输入任务 ID 查看结果', 'Enter a task ID to load results')
const datasetPlaceholderText = fallbackText('settings.evaluation.datasetIdPlaceholder', 'default 或自定义数据集', 'default or a custom dataset')
const knowledgeBasePlaceholderText = fallbackText('settings.evaluation.knowledgeBaseIdPlaceholder', '留空则自动创建临时知识库', 'Leave empty to create a temporary knowledge base')
const chatModelPlaceholderText = fallbackText('settings.evaluation.chatModelIdPlaceholder', '留空使用默认知识问答模型', 'Leave empty to use the default QA model')
const rerankModelPlaceholderText = fallbackText('settings.evaluation.rerankModelIdPlaceholder', '留空使用默认重排模型', 'Leave empty to use the default rerank model')
const progressText = fallbackText('settings.evaluation.progress', '进度', 'Progress')
const startTimeText = fallbackText('settings.evaluation.startTime', '开始时间', 'Start time')
const errorText = fallbackText('settings.evaluation.error', '错误', 'Error')
const noResultText = fallbackText('settings.evaluation.noResult', '暂无任务结果', 'No evaluation result yet')
const retrievalMetricsText = fallbackText('settings.evaluation.retrievalMetrics', '检索指标', 'Retrieval metrics')
const generationMetricsText = fallbackText('settings.evaluation.generationMetrics', '生成指标', 'Generation metrics')

const retrievalMetricCards = computed(() => {
  const metrics = task.value?.metric?.retrieval_metrics
  return [
    { key: 'precision', label: 'Precision', value: metrics?.precision || 0 },
    { key: 'recall', label: 'Recall', value: metrics?.recall || 0 },
    { key: 'ndcg3', label: 'NDCG@3', value: metrics?.ndcg3 || 0 },
    { key: 'ndcg10', label: 'NDCG@10', value: metrics?.ndcg10 || 0 },
    { key: 'mrr', label: 'MRR', value: metrics?.mrr || 0 },
    { key: 'map', label: 'MAP', value: metrics?.map || 0 },
  ]
})

const generationMetricCards = computed(() => {
  const metrics = task.value?.metric?.generation_metrics
  return [
    { key: 'bleu1', label: 'BLEU-1', value: metrics?.bleu1 || 0 },
    { key: 'bleu2', label: 'BLEU-2', value: metrics?.bleu2 || 0 },
    { key: 'bleu4', label: 'BLEU-4', value: metrics?.bleu4 || 0 },
    { key: 'rouge1', label: 'ROUGE-1', value: metrics?.rouge1 || 0 },
    { key: 'rouge2', label: 'ROUGE-2', value: metrics?.rouge2 || 0 },
    { key: 'rougel', label: 'ROUGE-L', value: metrics?.rougel || 0 },
  ]
})

const formatPercent = (value: number) => `${Math.round((value || 0) * 100)}%`
const formatTime = (value?: string) => (value ? new Date(value).toLocaleString() : '-')
const taskTheme = (status: number) => {
  if (status === 3) return 'danger'
  if (status === 2) return 'success'
  if (status === 1) return 'warning'
  return 'default'
}
const taskStatusText = (status: number) => {
  if (status === 1) return fallbackText('settings.evaluation.running', '运行中', 'Running')
  if (status === 2) return fallbackText('settings.evaluation.success', '已完成', 'Completed')
  if (status === 3) return fallbackText('settings.evaluation.failed', '失败', 'Failed')
  return fallbackText('settings.evaluation.pending', '等待中', 'Pending')
}

const clearPolling = () => {
  if (pollingTimer.value) {
    window.clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

const loadTask = async (taskId: string) => {
  if (!taskId) return
  loadingTask.value = true
  try {
    const res = await getEvaluationResult(taskId)
    task.value = res?.data || null
    taskIdInput.value = taskId
  } catch (error) {
    console.error('[EvaluationSettings] Failed to load evaluation result', error)
    MessagePlugin.error(fallbackText('settings.evaluation.loadFailed', '加载评测结果失败', 'Failed to load evaluation result'))
  } finally {
    loadingTask.value = false
  }
}

const loadCurrentTask = async () => {
  await loadTask(taskIdInput.value.trim())
}

const runEvaluation = async () => {
  if (!form.dataset_id.trim()) {
    MessagePlugin.warning(fallbackText('settings.evaluation.datasetRequired', '请先填写数据集 ID', 'Please enter a dataset ID first'))
    return
  }

  try {
    clearPolling()
    const res = await startEvaluation({
      dataset_id: form.dataset_id.trim(),
      knowledge_base_id: form.knowledge_base_id.trim() || undefined,
      chat_id: form.chat_id.trim() || undefined,
      rerank_id: form.rerank_id.trim() || undefined,
    })
    const data = res?.data || {}
    const newTaskId = data?.task?.id || ''
    task.value = data
    taskIdInput.value = newTaskId
    MessagePlugin.success(fallbackText('settings.evaluation.started', '评测已启动', 'Evaluation started'))
    if (newTaskId) {
      pollingTimer.value = window.setInterval(() => {
        loadTask(newTaskId)
      }, 3000)
    }
  } catch (error) {
    console.error('[EvaluationSettings] Failed to start evaluation', error)
    MessagePlugin.error(fallbackText('settings.evaluation.runFailed', '启动评测失败', 'Failed to start evaluation'))
  }
}

onBeforeUnmount(() => {
  clearPolling()
})
</script>

<style scoped>
.evaluation-settings {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.section-description {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
}

.header-actions {
  display: flex;
  gap: 12px;
  flex-shrink: 0;
}

.panel-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
}

.panel {
  border: 1px solid var(--td-border-level-1-color);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  padding: 16px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.panel-header h3,
.metric-group h4 {
  margin: 0;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.task-toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.task-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.summary-item {
  border: 1px solid var(--td-border-level-1-color);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.summary-error {
  grid-column: 1 / -1;
}

.metric-groups {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.metric-group {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.metric-card {
  border: 1px solid var(--td-border-level-1-color);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.panel-empty {
  color: var(--td-text-color-secondary);
}

@media (max-width: 960px) {
  .form-grid,
  .task-summary,
  .metric-grid {
    grid-template-columns: 1fr;
  }

  .section-header,
  .panel-header,
  .task-toolbar {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>

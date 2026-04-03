<template>
  <div class="faq-list-page">
    <div class="page-header">
      <div>
        <span class="page-eyebrow">FAQ 知识入口</span>
        <h1>FAQ 标准问答</h1>
        <p>把高频标准问答集中维护起来，优先服务制度、流程和常见问题场景。</p>
      </div>
      <t-button v-if="canCreateKnowledgeBase" theme="primary" @click="createVisible = true">新建 FAQ</t-button>
    </div>

    <div v-if="loading" class="state-panel">
      <t-loading size="medium" text="正在加载 FAQ 内容" />
    </div>

    <div v-else-if="faqKnowledgeBases.length === 0" class="state-panel empty-panel">
      <t-icon name="chat-bubble-help" size="32px" />
      <h3>还没有 FAQ 内容</h3>
      <p>{{ emptyStateText }}</p>
      <t-button v-if="canCreateKnowledgeBase" theme="primary" @click="createVisible = true">去创建 FAQ</t-button>
      <t-button v-else variant="outline" @click="router.push('/platform/knowledge-search')">先去提问</t-button>
    </div>

    <div v-else class="faq-grid">
      <div v-for="kb in faqKnowledgeBases" :key="kb.id" class="faq-card">
        <div class="faq-card-header">
          <span class="faq-badge">FAQ</span>
          <span class="faq-date">{{ formatDate(kb.updated_at || kb.created_at) }}</span>
        </div>
        <h3>{{ kb.name }}</h3>
        <p>{{ kb.description || '暂无描述' }}</p>
        <div class="faq-meta">
          <span>{{ kb.chunk_count || 0 }} 条 FAQ</span>
          <span>{{ kb.updated_at || kb.created_at ? '最近有更新' : '待完善' }}</span>
        </div>
        <div class="faq-actions">
          <t-button variant="outline" @click="goToKnowledgeBase(kb.id)">查看内容</t-button>
          <t-button theme="primary" variant="text" @click="goToKnowledgeBase(kb.id)">进入维护</t-button>
        </div>
      </div>
    </div>

    <KnowledgeBaseEditorModal
      v-model:visible="createVisible"
      mode="create"
      initial-type="faq"
      @success="handleCreateSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { useAuthStore } from '@/stores/auth'
import KnowledgeBaseEditorModal from './KnowledgeBaseEditorModal.vue'

interface KnowledgeBaseItem {
  id: string
  name: string
  description?: string
  type?: 'document' | 'faq'
  chunk_count?: number
  created_at?: string
  updated_at?: string
}

const router = useRouter()
const authStore = useAuthStore()
const loading = ref(false)
const createVisible = ref(false)
const knowledgeBases = ref<KnowledgeBaseItem[]>([])
const canCreateKnowledgeBase = computed(() => authStore.hasValidTenant)

const emptyStateText = computed(() =>
  canCreateKnowledgeBase.value
    ? '可以先创建一个 FAQ，把高频问题沉淀成标准答案。'
    : '当前还没有可维护的 FAQ 内容，请联系管理员或空间负责人分配维护权限。'
)

const faqKnowledgeBases = computed(() =>
  knowledgeBases.value
    .filter(item => item.type === 'faq')
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0).getTime() - new Date(a.updated_at || a.created_at || 0).getTime())
)

const loadKnowledgeBases = async () => {
  loading.value = true
  try {
    const response: any = await listKnowledgeBases()
    knowledgeBases.value = Array.isArray(response?.data) ? response.data : []
  } finally {
    loading.value = false
  }
}

const goToKnowledgeBase = (kbId: string) => {
  router.push(`/platform/knowledge-bases/${kbId}`)
}

const handleCreateSuccess = async (kbId: string) => {
  createVisible.value = false
  await loadKnowledgeBases()
  router.push(`/platform/knowledge-bases/${kbId}`)
}

const formatDate = (value?: string) => {
  if (!value) return '未更新'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '未更新'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).format(date)
}

onMounted(() => {
  loadKnowledgeBases()
})
</script>

<style scoped lang="less">
.faq-list-page {
  flex: 1;
  padding: 32px;
  overflow-y: auto;
  background: linear-gradient(180deg, #f8fafc 0%, #ffffff 220px);
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 8px 0 12px;
  font-size: 32px;
  color: #13231a;
}

.page-header p {
  margin: 0;
  color: #66727c;
  line-height: 1.7;
}

.page-eyebrow {
  display: inline-flex;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(22, 93, 255, 0.08);
  color: #1c5bdb;
  font-size: 12px;
  letter-spacing: 0.08em;
}

.faq-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.faq-card {
  padding: 20px;
  border-radius: 20px;
  background: #fff;
  border: 1px solid rgba(28, 91, 219, 0.08);
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.05);
}

.faq-card h3 {
  margin: 16px 0 10px;
  font-size: 18px;
  color: #13231a;
}

.faq-card p {
  min-height: 46px;
  margin: 0;
  color: #66727c;
  line-height: 1.7;
  font-size: 13px;
}

.faq-card-header,
.faq-meta,
.faq-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.faq-badge {
  display: inline-flex;
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(22, 93, 255, 0.08);
  color: #1c5bdb;
  font-size: 12px;
  font-weight: 600;
}

.faq-date {
  color: #82909b;
  font-size: 12px;
}

.faq-meta {
  margin-top: 16px;
  color: #4d5e6b;
  font-size: 13px;
}

.faq-actions {
  margin-top: 18px;
}

.state-panel {
  min-height: 240px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-panel {
  flex-direction: column;
  gap: 12px;
  color: #66727c;
}

.empty-panel h3,
.empty-panel p {
  margin: 0;
}

@media (max-width: 1200px) {
  .faq-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .faq-list-page {
    padding: 20px;
  }

  .page-header {
    flex-direction: column;
  }

  .faq-grid {
    grid-template-columns: 1fr;
  }
}
</style>

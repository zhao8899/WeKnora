<template>
  <div class="home-view">
    <section class="hero-section">
      <div class="hero-copy">
        <span class="hero-eyebrow">普通租户工作台</span>
        <h1>先把知识建起来，再开始稳定问答</h1>
        <p>普通租户默认只围绕建库、导入、提问和共享四件事工作，不再让平台能力抢占首屏注意力。</p>
      </div>

      <div class="hero-progress">
        <div v-for="item in progressItems" :key="item.label" class="progress-card">
          <span class="progress-label">{{ item.label }}</span>
          <strong class="progress-value">{{ item.value }}</strong>
          <span class="progress-desc">{{ item.description }}</span>
        </div>
      </div>
    </section>

    <section class="action-grid">
      <button
        v-for="item in primaryActions"
        :key="item.title"
        type="button"
        class="action-card"
        @click="handlePrimaryAction(item)"
      >
        <div class="action-icon">
          <t-icon :name="item.icon" size="20px" />
        </div>
        <div class="action-content">
          <h3>{{ item.title }}</h3>
          <p>{{ item.description }}</p>
        </div>
        <t-icon name="chevron-right" size="18px" class="action-arrow" />
      </button>
    </section>

    <section class="workspace-grid">
      <article class="panel-card">
        <div class="panel-header">
          <div>
            <h2>最近更新的知识库</h2>
            <p>优先继续维护最近更新过的内容，减少来回找入口。</p>
          </div>
          <t-button variant="text" theme="primary" @click="router.push('/platform/knowledge-bases')">
            全部知识库
          </t-button>
        </div>

        <div v-if="loading" class="state-panel">
          <t-loading size="medium" text="正在加载知识库" />
        </div>

        <div v-else-if="recentKnowledgeBases.length === 0" class="state-panel empty-panel">
          <t-icon name="folder-open" size="28px" />
          <h3>还没有知识库</h3>
          <p>{{ knowledgeEmptyText }}</p>
          <t-button theme="primary" @click="router.push('/platform/knowledge-bases')">
            去创建知识库
          </t-button>
        </div>

        <div v-else class="list-stack">
          <button
            v-for="kb in recentKnowledgeBases"
            :key="kb.id"
            type="button"
            class="list-card"
            @click="router.push(`/platform/knowledge-bases/${kb.id}`)"
          >
            <div class="list-card-main">
              <div class="list-card-top">
                <span class="list-type" :class="kb.type === 'faq' ? 'faq' : 'document'">
                  {{ kb.type === 'faq' ? 'FAQ' : '文档' }}
                </span>
                <span class="list-status" :class="getKnowledgeStatus(kb).tone">
                  {{ getKnowledgeStatus(kb).label }}
                </span>
              </div>
              <h3>{{ kb.name }}</h3>
              <p>{{ kb.description || '暂无描述' }}</p>
            </div>
            <div class="list-card-meta">
              <span>{{ getKnowledgeMeta(kb) }}</span>
              <span>{{ formatDate(kb.updated_at || kb.created_at) }}</span>
            </div>
          </button>
        </div>
      </article>

      <article class="panel-card">
        <div class="panel-header">
          <div>
            <h2>共享空间概览</h2>
            <p>只保留加入空间和共享知识两条主线，不把复杂空间治理塞进首屏。</p>
          </div>
          <t-button variant="text" theme="primary" @click="router.push('/platform/organizations')">
            进入共享空间
          </t-button>
        </div>

        <div v-if="spaceLoading" class="state-panel">
          <t-loading size="medium" text="正在加载共享空间" />
        </div>

        <div v-else-if="recentOrganizations.length === 0" class="state-panel empty-panel">
          <t-icon name="share" size="28px" />
          <h3>还没有共享空间</h3>
          <p>{{ spaceEmptyText }}</p>
          <t-button theme="primary" @click="router.push('/platform/organizations')">
            去查看共享空间
          </t-button>
        </div>

        <div v-else class="list-stack">
          <button
            v-for="org in recentOrganizations"
            :key="org.id"
            type="button"
            class="list-card"
            @click="handleSpaceCardClick(org)"
          >
            <div class="list-card-main">
              <div class="list-card-top">
                <span class="list-type share">共享空间</span>
                <span class="space-role">{{ org.is_owner ? '创建者' : roleLabel(org.my_role) }}</span>
              </div>
              <h3>{{ org.name }}</h3>
              <p>{{ org.description || '暂无描述' }}</p>
            </div>
            <div class="list-card-meta">
              <span>{{ org.member_count || 0 }} 位成员</span>
              <span>{{ org.share_count || 0 }} 个共享知识库</span>
            </div>
          </button>
        </div>
      </article>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { getUsageStats } from '@/api/usage'
import { useOrganizationStore } from '@/stores/organization'
import { useUIStore } from '@/stores/ui'

interface KnowledgeBaseItem {
  id: string
  name: string
  description?: string
  type?: 'document' | 'faq'
  knowledge_count?: number
  chunk_count?: number
  created_at?: string
  updated_at?: string
  embedding_model_id?: string
  summary_model_id?: string
  is_processing?: boolean
}

const router = useRouter()
const organizationStore = useOrganizationStore()
const uiStore = useUIStore()
const loading = ref(false)
const spaceLoading = ref(false)
const knowledgeBases = ref<KnowledgeBaseItem[]>([])
const monthSessions = ref(0)

const primaryActions = [
  {
    title: '新建知识库',
    description: '创建文档知识库或 FAQ 知识库，开始沉淀资料。',
    path: '/platform/knowledge-bases',
    icon: 'folder-add'
  },
  {
    title: '导入资料',
    description: '继续给现有知识库上传文件、网址或手工内容。',
    path: '/platform/knowledge-bases',
    icon: 'upload'
  },
  {
    title: '开始提问',
    description: '直接验证当前知识是否能支撑可信问答。',
    path: '/platform/creatChat',
    icon: 'chat'
  },
  {
    title: '共享给团队',
    description: '把已验证可用的知识共享到空间或团队。',
    path: '/platform/organizations',
    icon: 'share'
  }
]

type PrimaryAction = (typeof primaryActions)[number]

const handlePrimaryAction = (item: PrimaryAction) => {
  if (item.icon === 'folder-add') {
    uiStore.openCreateKB('document')
    router.push(item.path)
    return
  }
  if (item.icon === 'upload') {
    router.push({ path: item.path, query: { intent: 'import' } })
    return
  }
  router.push(item.path)
}

const hasConfiguredKnowledgeBase = computed(() =>
  knowledgeBases.value.some((item) => !!item.embedding_model_id && !!item.summary_model_id)
)

const readyKnowledgeBaseCount = computed(() =>
  knowledgeBases.value.filter((item) => getKnowledgeStatus(item).label === '可问答').length
)

const parsingKnowledgeBaseCount = computed(() =>
  knowledgeBases.value.filter((item) => getKnowledgeStatus(item).label === '解析中').length
)

const progressItems = computed(() => [
  {
    label: '知识库总数',
    value: knowledgeBases.value.length,
    description: '普通租户当前可访问的全部知识库。'
  },
  {
    label: '可问答知识库',
    value: readyKnowledgeBaseCount.value,
    description: '已完成基础配置并可直接验证问答效果。'
  },
  {
    label: '解析中的知识库',
    value: parsingKnowledgeBaseCount.value,
    description: '仍在处理资料，适合继续等待或补充内容。'
  },
  {
    label: '本月问答',
    value: monthSessions.value.toLocaleString(),
    description: '帮助判断租户是否真的开始使用知识问答。'
  }
])

const recentKnowledgeBases = computed(() =>
  [...knowledgeBases.value]
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0).getTime() - new Date(a.updated_at || a.created_at || 0).getTime())
    .slice(0, 5)
)

const recentOrganizations = computed(() =>
  [...organizationStore.organizations].slice(0, 4)
)

const getSpaceKnowledgeCount = (org: any) =>
  organizationStore.resourceCounts?.knowledge_bases?.by_organization?.[org.id] ?? org.share_count ?? 0

const getSpaceAgentCount = (org: any) =>
  organizationStore.resourceCounts?.agents?.by_organization?.[org.id] ?? org.agent_share_count ?? 0

const handleSpaceCardClick = (org: any) => {
  if (getSpaceKnowledgeCount(org) > 0) {
    router.push({ path: '/platform/knowledge-bases', query: { space: org.id } })
    return
  }
  if (getSpaceAgentCount(org) > 0) {
    router.push({ path: '/platform/agents', query: { space: org.id } })
    return
  }
  router.push('/platform/organizations')
}

const knowledgeEmptyText = computed(() =>
  hasConfiguredKnowledgeBase.value
    ? '当前还没有最近更新的知识库。'
    : '先创建一个知识库并导入资料，再回来验证问答效果。'
)

const spaceEmptyText = computed(() =>
  '先在共享空间里加入团队或共享已有知识，后续再扩展协作边界。'
)

const getKnowledgeStatus = (kb: KnowledgeBaseItem) => {
  const hasModels = !!kb.embedding_model_id && !!kb.summary_model_id
  const itemCount = kb.type === 'faq' ? (kb.chunk_count || 0) : (kb.knowledge_count || 0)

  if (!hasModels) {
    return { label: '待配置', tone: 'warning' }
  }
  if (kb.is_processing) {
    return { label: '解析中', tone: 'processing' }
  }
  if (itemCount === 0) {
    return { label: '待导入', tone: 'pending' }
  }
  return { label: '可问答', tone: 'ready' }
}

const getKnowledgeMeta = (kb: KnowledgeBaseItem) => {
  if (kb.type === 'faq') {
    return `${kb.chunk_count || 0} 条 FAQ`
  }
  return `${kb.knowledge_count || 0} 个文档`
}

const loadKnowledgeBases = async () => {
  loading.value = true
  try {
    const response: any = await listKnowledgeBases()
    knowledgeBases.value = Array.isArray(response?.data) ? response.data : []
  } finally {
    loading.value = false
  }
}

const loadUsageStats = async () => {
  try {
    const res = await getUsageStats()
    monthSessions.value = res.data?.month_sessions ?? 0
  } catch (_) {
    monthSessions.value = 0
  }
}

const loadOrganizations = async () => {
  spaceLoading.value = true
  try {
    await organizationStore.fetchOrganizations()
  } finally {
    spaceLoading.value = false
  }
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

const roleLabel = (role?: string) => {
  if (role === 'admin') return '空间负责人'
  if (role === 'editor') return '协作者'
  return '只读成员'
}

onMounted(() => {
  loadKnowledgeBases()
  loadOrganizations()
  loadUsageStats()
})
</script>

<style scoped lang="less">
.home-view {
  flex: 1;
  padding: 32px;
  overflow-y: auto;
  background:
    radial-gradient(circle at top left, rgba(7, 192, 95, 0.08), transparent 28%),
    linear-gradient(180deg, #f6fbf8 0%, var(--td-bg-color-page) 26%);
}

.hero-section {
  display: grid;
  grid-template-columns: 1.1fr 0.9fr;
  gap: 20px;
  padding: 28px;
  border-radius: 28px;
  background: linear-gradient(140deg, #0d4025 0%, #16693f 52%, #1a7a49 100%);
  color: #fff;
  box-shadow: 0 24px 48px rgba(22, 105, 63, 0.18);
}

.hero-copy {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 14px;
}

.hero-copy h1 {
  margin: 0;
  font-size: 36px;
  line-height: 1.15;
  letter-spacing: -0.02em;
}

.hero-copy p {
  margin: 0;
  max-width: 640px;
  line-height: 1.8;
  color: rgba(255, 255, 255, 0.82);
}

.hero-eyebrow {
  display: inline-flex;
  width: fit-content;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.12);
  font-size: 12px;
  letter-spacing: 0.06em;
}

.hero-progress {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.progress-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 18px 20px;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.11);
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.progress-label,
.progress-desc {
  color: rgba(255, 255, 255, 0.8);
}

.progress-label {
  font-size: 13px;
}

.progress-value {
  font-size: 28px;
}

.progress-desc {
  font-size: 12px;
  line-height: 1.5;
}

.action-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
  margin-top: 22px;
}

.action-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 22px 18px;
  border-radius: 20px;
  border: 1px solid rgba(16, 24, 40, 0.08);
  background: rgba(255, 255, 255, 0.88);
  backdrop-filter: blur(8px);
  text-align: left;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}

.action-card:hover {
  transform: translateY(-2px);
  border-color: rgba(7, 192, 95, 0.28);
  box-shadow: 0 18px 36px rgba(16, 24, 40, 0.08);
}

.action-icon {
  width: 46px;
  height: 46px;
  border-radius: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: rgba(7, 192, 95, 0.1);
  color: var(--td-brand-color);
}

.action-content {
  flex: 1;
  min-width: 0;
}

.action-content h3 {
  margin: 0;
  color: var(--td-text-color-primary);
  font-size: 17px;
}

.action-content p {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
  font-size: 13px;
}

.action-arrow {
  color: var(--td-text-color-placeholder);
}

.workspace-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
  margin-top: 24px;
}

.panel-card {
  padding: 24px;
  border-radius: 24px;
  background: var(--td-bg-color-container);
  border: 1px solid rgba(16, 24, 40, 0.08);
  box-shadow: 0 12px 24px rgba(16, 24, 40, 0.04);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 18px;
}

.panel-header h2 {
  margin: 0;
  font-size: 22px;
  color: var(--td-text-color-primary);
}

.panel-header p {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
}

.list-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.list-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px 18px;
  border-radius: 18px;
  border: 1px solid var(--td-component-stroke);
  background: linear-gradient(180deg, var(--td-bg-color-container) 0%, var(--td-bg-color-secondarycontainer) 100%);
  text-align: left;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}

.list-card:hover {
  transform: translateY(-2px);
  border-color: rgba(7, 192, 95, 0.28);
  box-shadow: 0 14px 24px rgba(16, 24, 40, 0.06);
}

.list-card-main h3 {
  margin: 10px 0 6px;
  font-size: 17px;
  color: var(--td-text-color-primary);
}

.list-card-main p {
  margin: 0;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
  font-size: 13px;
}

.list-card-top,
.list-card-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.list-card-meta {
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.list-type,
.list-status,
.space-role {
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.list-type.document {
  background: rgba(7, 192, 95, 0.1);
  color: #0a7f3f;
}

.list-type.faq {
  background: rgba(22, 93, 255, 0.1);
  color: #1c5bdb;
}

.list-type.share,
.space-role {
  background: rgba(21, 93, 56, 0.08);
  color: #0a7f3f;
}

.list-status.warning {
  background: rgba(255, 152, 0, 0.14);
  color: #b65c00;
}

.list-status.processing {
  background: rgba(22, 93, 255, 0.12);
  color: #1c5bdb;
}

.list-status.pending {
  background: rgba(107, 114, 128, 0.12);
  color: #4b5563;
}

.list-status.ready {
  background: rgba(7, 192, 95, 0.12);
  color: #0a7f3f;
}

.state-panel {
  min-height: 260px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-panel {
  flex-direction: column;
  gap: 10px;
  color: var(--td-text-color-secondary);
  text-align: center;
}

.empty-panel h3,
.empty-panel p {
  margin: 0;
}

@media (max-width: 1200px) {
  .hero-section,
  .action-grid,
  .workspace-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .home-view {
    padding: 20px;
  }

  .hero-section,
  .action-grid,
  .workspace-grid,
  .hero-progress {
    grid-template-columns: 1fr;
  }

  .panel-card {
    padding: 20px;
  }
}
</style>

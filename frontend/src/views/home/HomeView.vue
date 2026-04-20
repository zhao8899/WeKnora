<template>
  <div class="home-view">
    <section class="hero-section">
      <div class="hero-copy">
        <span class="hero-eyebrow">企业知识入口</span>
        <h1>统一搜索、统一查看、统一问答</h1>
        <p>先把文档知识、FAQ 和共享空间收敛成一个稳定入口，再逐步扩展后续能力。</p>
        <div class="hero-actions">
          <t-button theme="primary" size="large" @click="router.push('/platform/knowledge-search')">
            <template #icon><t-icon name="search" /></template>
            知识搜索
          </t-button>
          <t-button variant="outline" size="large" @click="router.push('/platform/creatChat')">
            <template #icon><t-icon name="chat" /></template>
            智能问答
          </t-button>
        </div>
      </div>

      <div class="hero-stats">
        <div class="stat-card">
          <span class="stat-label">知识库总数</span>
          <strong class="stat-value">{{ stats.total }}</strong>
          <span class="stat-desc">当前可访问的全部知识内容</span>
        </div>
        <div class="stat-card accent">
          <span class="stat-label">FAQ 数量</span>
          <strong class="stat-value">{{ stats.faq }}</strong>
          <span class="stat-desc">适合沉淀标准问答</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">文档知识库</span>
          <strong class="stat-value">{{ stats.document }}</strong>
          <span class="stat-desc">适合制度、手册和资料</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">本月问答</span>
          <strong class="stat-value">{{ monthSessions.toLocaleString() }}</strong>
          <span class="stat-desc">本月累计问答会话次数</span>
        </div>
      </div>
    </section>

    <section class="shortcut-grid">
      <button
        v-for="item in shortcuts"
        :key="item.title"
        type="button"
        class="shortcut-card"
        @click="router.push(item.path)"
      >
        <div class="shortcut-icon">
          <t-icon :name="item.icon" size="20px" />
        </div>
        <div class="shortcut-content">
          <h3>{{ item.title }}</h3>
          <p>{{ item.description }}</p>
        </div>
      </button>
    </section>

    <section class="knowledge-panel">
      <div class="panel-header">
        <div>
          <h2>最近更新的知识库</h2>
          <p>优先从最近维护的内容继续建设和验证检索效果。</p>
        </div>
        <t-button variant="text" theme="primary" @click="router.push('/platform/knowledge-bases')">
          进入知识库
        </t-button>
      </div>

      <div v-if="loading" class="state-panel">
        <t-loading size="medium" text="正在加载首页数据" />
      </div>

      <div v-else-if="recentKnowledgeBases.length === 0" class="state-panel empty-panel">
        <t-icon name="folder-open" size="32px" />
        <h3>还没有知识库内容</h3>
        <p>{{ knowledgeEmptyText }}</p>
        <t-button
          v-if="canCreateKnowledgeBase"
          theme="primary"
          @click="router.push('/platform/knowledge-bases')"
        >
          去创建知识库
        </t-button>
        <t-button
          v-else
          variant="outline"
          @click="router.push('/platform/knowledge-search')"
        >
          先去提问
        </t-button>
      </div>

      <div v-else class="kb-grid">
        <button
          v-for="kb in recentKnowledgeBases"
          :key="kb.id"
          type="button"
          class="kb-card"
          @click="router.push(`/platform/knowledge-bases/${kb.id}`)"
        >
          <div class="kb-card-top">
            <span class="kb-type" :class="kb.type === 'faq' ? 'faq' : 'document'">
              {{ kb.type === 'faq' ? 'FAQ' : 'DOC' }}
            </span>
            <span class="kb-date">{{ formatDate(kb.updated_at || kb.created_at) }}</span>
          </div>
          <h3 class="kb-title">{{ kb.name }}</h3>
          <p class="kb-desc">{{ kb.description || '暂无描述' }}</p>
          <div class="kb-meta">
            <span>{{ kb.type === 'faq' ? `${kb.chunk_count || 0} 条 FAQ` : `${kb.knowledge_count || 0} 个文档` }}</span>
            <t-icon name="chevron-right" size="16px" />
          </div>
        </button>
      </div>
    </section>

    <section class="space-panel">
      <div class="panel-header">
        <div>
          <h2>共享空间概览</h2>
          <p>先按组织和团队聚合知识，再逐步建立更稳定的共享和协作边界。</p>
        </div>
        <t-button variant="text" theme="primary" @click="router.push('/platform/organizations')">
          进入空间
        </t-button>
      </div>

      <div v-if="spaceLoading" class="state-panel">
        <t-loading size="medium" text="正在加载共享空间" />
      </div>

      <div v-else-if="recentOrganizations.length === 0" class="state-panel empty-panel">
        <t-icon name="share" size="32px" />
        <h3>还没有共享空间</h3>
        <p>{{ spaceEmptyText }}</p>
        <t-button
          v-if="canCreateKnowledgeBase"
          theme="primary"
          @click="router.push('/platform/organizations')"
        >
          去创建空间
        </t-button>
        <t-button
          v-else
          variant="outline"
          @click="router.push('/platform/knowledge-search')"
        >
          先去提问
        </t-button>
      </div>

      <div v-else class="space-grid">
        <button
          v-for="org in recentOrganizations"
          :key="org.id"
          type="button"
          class="space-card"
          @click="router.push('/platform/organizations')"
        >
          <div class="space-card-top">
            <span class="space-name">{{ org.name }}</span>
            <span class="space-role">{{ org.is_owner ? '创建者' : roleLabel(org.my_role) }}</span>
          </div>
          <p class="space-desc">{{ org.description || '暂无描述' }}</p>
          <div class="space-meta">
            <span>{{ org.member_count || 0 }} 位成员</span>
            <span>{{ org.share_count || 0 }} 个共享知识库</span>
          </div>
        </button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { useAuthStore } from '@/stores/auth'
import { useOrganizationStore } from '@/stores/organization'
import { getUsageStats } from '@/api/usage/index'

interface KnowledgeBaseItem {
  id: string
  name: string
  description?: string
  type?: 'document' | 'faq'
  knowledge_count?: number
  chunk_count?: number
  created_at?: string
  updated_at?: string
}

const router = useRouter()
const authStore = useAuthStore()
const organizationStore = useOrganizationStore()
const loading = ref(false)
const spaceLoading = ref(false)
const knowledgeBases = ref<KnowledgeBaseItem[]>([])
const monthSessions = ref(0)
const canCreateKnowledgeBase = computed(() => authStore.hasValidTenant)

const shortcuts = computed(() => [
  {
    title: '知识库',
    description: canCreateKnowledgeBase.value ? '进入并维护文档知识内容' : '进入并查看文档知识内容',
    path: '/platform/knowledge-bases',
    icon: 'folder'
  },
  {
    title: 'FAQ',
    description: canCreateKnowledgeBase.value ? '进入并维护高频标准问答' : '进入并查看高频标准问答',
    path: '/platform/faq',
    icon: 'chat-bubble-help'
  },
  { title: '智能问答', description: '面向企业知识直接提问', path: '/platform/knowledge-search', icon: 'search' },
  {
    title: '共享空间',
    description: canCreateKnowledgeBase.value ? '进入团队共享空间并维护协作内容' : '进入你已加入的共享空间',
    path: '/platform/organizations',
    icon: 'share'
  }
])

const knowledgeEmptyText = computed(() =>
  canCreateKnowledgeBase.value
    ? '可以先创建一个文档知识库或 FAQ 知识库，建立一期试点内容。'
    : '当前还没有可访问的知识库内容，请联系管理员或空间负责人分配知识访问权限。'
)

const spaceEmptyText = computed(() =>
  canCreateKnowledgeBase.value
    ? '可以先新建一个共享空间，把试点知识库共享给对应团队或部门。'
    : '当前还没有加入共享空间，请联系管理员或空间负责人邀请你加入对应共享空间。'
)

const stats = computed(() => {
  const total = knowledgeBases.value.length
  const faq = knowledgeBases.value.filter(item => item.type === 'faq').length
  return {
    total,
    faq,
    document: total - faq
  }
})

const recentKnowledgeBases = computed(() =>
  [...knowledgeBases.value]
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0).getTime() - new Date(a.updated_at || a.created_at || 0).getTime())
    .slice(0, 6)
)

const recentOrganizations = computed(() =>
  [...organizationStore.organizations].slice(0, 4)
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

const loadUsageStats = async () => {
  try {
    const res = await getUsageStats()
    monthSessions.value = res.data?.month_sessions ?? 0
  } catch (_) { }
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
  if (role === 'admin') return '管理员'
  if (role === 'editor') return '编辑'
  return '只读'
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
  background: var(--td-bg-color-page);
}

.hero-section {
  display: grid;
  grid-template-columns: 1.35fr 1fr;
  gap: 24px;
  padding: 28px 32px;
  border-radius: 24px;
  background: linear-gradient(135deg, #0f2f1d 0%, #155d38 100%);
  color: #fff;
  box-shadow: 0 24px 48px rgba(21, 93, 56, 0.18);
}

.hero-copy h1 {
  margin: 8px 0 12px;
  font-size: 32px;
  line-height: 1.2;
}

.hero-copy p {
  max-width: 620px;
  margin: 0;
  color: rgba(255, 255, 255, 0.78);
  line-height: 1.7;
}

.hero-eyebrow {
  display: inline-flex;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.14);
  font-size: 12px;
  letter-spacing: 0.08em;
}

.hero-actions {
  display: flex;
  gap: 12px;
  margin-top: 24px;
  flex-wrap: wrap;
}

.hero-stats {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  align-content: start;
}

.stat-card {
  display: flex;
  flex-direction: column;
  padding: 18px 20px;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.stat-card.accent {
  background: rgba(255, 255, 255, 0.16);
}

.stat-label,
.stat-desc {
  color: rgba(255, 255, 255, 0.78);
}

.stat-label {
  font-size: 13px;
}

.stat-value {
  margin: 8px 0 4px;
  font-size: 30px;
}

.stat-desc {
  font-size: 12px;
}

.shortcut-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
  margin-top: 20px;
}

.shortcut-card {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 20px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 18px;
  background: var(--td-bg-color-container);
  text-align: left;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}

.shortcut-card:hover {
  transform: translateY(-2px);
  border-color: var(--td-brand-color-light);
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.08);
}

.shortcut-icon {
  width: 42px;
  height: 42px;
  border-radius: 14px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(7, 192, 95, 0.1);
  color: #0a7f3f;
  flex-shrink: 0;
}

.shortcut-content h3 {
  margin: 0;
  font-size: 16px;
  color: var(--td-text-color-primary);
}

.shortcut-content p {
  margin: 6px 0 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

.knowledge-panel {
  margin-top: 28px;
  padding: 24px;
  border-radius: 24px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
}

.space-panel {
  margin-top: 20px;
  padding: 24px;
  border-radius: 24px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 20px;
}

.panel-header h2 {
  margin: 0;
  font-size: 22px;
  color: var(--td-text-color-primary);
}

.panel-header p {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
}

.kb-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.space-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.kb-card {
  padding: 18px;
  border-radius: 18px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
  text-align: left;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.kb-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 16px 30px rgba(15, 23, 42, 0.08);
}

.kb-card-top,
.kb-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.kb-type {
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.kb-type.document {
  background: rgba(7, 192, 95, 0.1);
  color: #0a7f3f;
}

.kb-type.faq {
  background: rgba(22, 93, 255, 0.1);
  color: #1c5bdb;
}

.kb-date {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}

.kb-title {
  margin: 16px 0 8px;
  font-size: 18px;
  color: var(--td-text-color-primary);
}

.kb-desc {
  min-height: 44px;
  margin: 0 0 16px;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.kb-meta {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.space-card {
  padding: 18px;
  border-radius: 18px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
  text-align: left;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.space-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 16px 30px rgba(15, 23, 42, 0.08);
}

.space-card-top,
.space-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.space-name {
  font-size: 17px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.space-role {
  display: inline-flex;
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(21, 93, 56, 0.08);
  color: #0a7f3f;
  font-size: 12px;
}

.space-desc {
  min-height: 44px;
  margin: 14px 0 16px;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 1.7;
}

.space-meta {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.state-panel {
  min-height: 180px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-panel {
  flex-direction: column;
  gap: 10px;
  color: #66727c;
}

.empty-panel h3,
.empty-panel p {
  margin: 0;
  color: var(--td-text-color-secondary);
}

@media (max-width: 1200px) {
  .hero-section,
  .shortcut-grid,
  .kb-grid,
  .space-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .home-view {
    padding: 20px;
  }

  .hero-section,
  .shortcut-grid,
  .kb-grid,
  .space-grid {
    grid-template-columns: 1fr;
  }

  .knowledge-panel,
  .space-panel {
    padding: 20px;
  }
}
</style>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { marked } from 'marked'
import {
  createWikiPage,
  getWikiGraph,
  getWikiIndex,
  getWikiLog,
  getWikiPage,
  getWikiStats,
  listWikiPages,
  rebuildWikiLinks,
  searchWikiPages,
  type WikiGraphData,
  type WikiPage,
  type WikiStats
} from '@/api/wiki'
import { getKnowledgeBaseById } from '@/api/knowledge-base'

const route = useRoute()
const router = useRouter()

const kbId = computed(() => String(route.params.kbId || ''))
const viewMode = computed<'browser' | 'graph'>(() => route.name === 'wikiGraph' ? 'graph' : 'browser')

const kbInfo = ref<any>(null)
const loading = ref(false)
const pages = ref<WikiPage[]>([])
const currentPage = ref<WikiPage | null>(null)
const stats = ref<WikiStats | null>(null)
const graph = ref<WikiGraphData>({ nodes: [], edges: [] })
const query = ref('')
const creating = ref(false)
const rebuilding = ref(false)

const pageTypeOptions = [
  { label: '全部', value: '' },
  { label: '摘要', value: 'summary' },
  { label: '实体', value: 'entity' },
  { label: '概念', value: 'concept' },
  { label: '专题', value: 'synthesis' },
  { label: '对比', value: 'comparison' }
]
const pageType = ref('')

const wikiEnabled = computed(() => !!kbInfo.value?.indexing_strategy?.wiki_enabled)
const renderedContent = computed(() => marked.parse(currentPage.value?.content || ''))
const currentInboundLinks = computed(() => currentPage.value?.in_links || [])
const currentOutboundLinks = computed(() => currentPage.value?.out_links || [])
const currentAliases = computed(() => currentPage.value?.aliases || [])
const currentSources = computed(() => currentPage.value?.source_refs || [])
const currentChunks = computed(() => currentPage.value?.chunk_refs || [])
const currentMetadata = computed(() => currentPage.value?.page_metadata || {})
const currentStats = computed(() => [
  { label: '页面', value: stats.value?.total_pages || 0 },
  { label: '链接', value: stats.value?.total_links || 0 },
  { label: '孤立页', value: stats.value?.orphan_count || 0 },
  { label: '待处理', value: stats.value?.pending_tasks || 0 }
])

async function loadKB() {
  if (!kbId.value) return
  const res: any = await getKnowledgeBaseById(kbId.value)
  kbInfo.value = res.data || res
}

async function loadPages() {
  if (!kbId.value || !wikiEnabled.value) return
  loading.value = true
  try {
    const res = query.value
      ? await searchWikiPages(kbId.value, query.value, 50)
      : await listWikiPages(kbId.value, {
          page_type: pageType.value || undefined,
          status: 'published',
          page: 1,
          page_size: 100,
          sort_by: 'updated_at',
          sort_order: 'desc'
        })
    pages.value = query.value ? res.data.pages : res.data.pages

    const targetSlug = String(route.query.slug || '')
    if (targetSlug) {
      await openPage(targetSlug, false)
    } else if (!currentPage.value && pages.value.length) {
      await openPage(pages.value[0].slug, false)
    }
  } finally {
    loading.value = false
  }
}

async function loadStats() {
  if (!kbId.value || !wikiEnabled.value) return
  const res = await getWikiStats(kbId.value)
  stats.value = res.data
}

async function loadGraph() {
  if (!kbId.value || !wikiEnabled.value) return
  const res = await getWikiGraph(kbId.value)
  graph.value = res.data
}

async function openPage(slug: string, push = true) {
  const res = await getWikiPage(kbId.value, slug)
  currentPage.value = res.data
  if (push) {
    router.replace({ query: { ...route.query, slug } })
  }
}

async function openIndex() {
  const res = await getWikiIndex(kbId.value)
  currentPage.value = res.data
  router.replace({ query: {} })
  await loadPages()
}

async function openLog() {
  const res = await getWikiLog(kbId.value)
  currentPage.value = res.data
  router.replace({ query: { ...route.query, slug: 'log' } })
}

async function createStarterPage() {
  if (!kbId.value) return
  creating.value = true
  try {
    const res = await createWikiPage(kbId.value, {
      slug: 'getting-started',
      title: 'Getting Started',
      page_type: 'summary',
      status: 'published',
      summary: 'Wiki 起始页',
      content: '# Getting Started\n\n在这里沉淀从文档中整理出的结构化知识。'
    })
    MessagePlugin.success('Wiki 页面已创建')
    currentPage.value = res.data
    await loadPages()
  } finally {
    creating.value = false
  }
}

async function handleRebuildLinks() {
  rebuilding.value = true
  try {
    await rebuildWikiLinks(kbId.value)
    MessagePlugin.success('Wiki 链接已重建')
    await Promise.all([loadPages(), loadGraph(), loadStats()])
  } finally {
    rebuilding.value = false
  }
}

async function switchView(mode: 'browser' | 'graph') {
  await router.push({
    name: mode === 'graph' ? 'wikiGraph' : 'wikiWorkspace',
    params: { kbId: kbId.value },
    query: route.query
  })
}

watch([kbId, viewMode], async () => {
  await loadKB()
  await Promise.all([loadPages(), loadStats(), viewMode.value === 'graph' ? loadGraph() : Promise.resolve()])
})

watch(
  () => route.query.slug,
  async (slug) => {
    if (!slug || viewMode.value !== 'browser' || !wikiEnabled.value) return
    try {
      await openPage(String(slug), false)
    } catch (error) {
      console.error('Failed to open wiki page from route:', error)
    }
  }
)

onMounted(async () => {
  await loadKB()
  if (wikiEnabled.value) {
    await Promise.all([loadPages(), loadStats(), loadGraph()])
  }
})
</script>

<template>
  <div class="wiki-workspace">
    <header class="wiki-header">
      <div>
        <button class="breadcrumb" @click="router.push(`/platform/knowledge-bases/${kbId}`)">
          <t-icon name="chevron-left" />
          <span>{{ kbInfo?.name || '知识库' }}</span>
        </button>
        <h1>自动知识沉淀</h1>
      </div>
      <div class="header-actions">
        <t-radio-group :value="viewMode" variant="default-filled" @change="switchView($event as 'browser' | 'graph')">
          <t-radio-button value="browser">Wiki</t-radio-button>
          <t-radio-button value="graph">图谱</t-radio-button>
        </t-radio-group>
        <t-button variant="outline" @click="openIndex" :disabled="!wikiEnabled">索引页</t-button>
        <t-button variant="outline" @click="openLog" :disabled="!wikiEnabled">日志页</t-button>
        <t-button variant="outline" @click="handleRebuildLinks" :disabled="!wikiEnabled" :loading="rebuilding">重建链接</t-button>
      </div>
    </header>

    <section v-if="!wikiEnabled" class="empty-state">
      <t-icon name="info-circle" size="28px" />
      <h2>当前知识库未启用 Wiki 索引</h2>
      <p>请在知识库索引策略中启用 Wiki 后再进入自动知识沉淀入口。</p>
    </section>

    <template v-else>
      <div class="stats-row">
        <div v-for="item in currentStats" :key="item.label" class="stat-item">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
        </div>
      </div>

      <main v-if="viewMode === 'browser'" class="wiki-layout">
        <aside class="wiki-sidebar">
          <div class="sidebar-tools">
            <t-input v-model="query" clearable placeholder="搜索 Wiki 页面" @enter="loadPages" @clear="loadPages" />
            <t-select v-model="pageType" :options="pageTypeOptions" @change="loadPages" />
          </div>
          <div class="sidebar-actions">
            <t-button size="small" variant="outline" @click="openIndex">索引页</t-button>
            <t-button size="small" variant="outline" @click="openLog">日志页</t-button>
            <t-button size="small" theme="primary" :loading="creating" @click="createStarterPage">新建起始页</t-button>
          </div>
          <div class="sidebar-note">
            <span>支持 [[wiki-link]] 语法，重建链接后可刷新关系图。</span>
          </div>
          <t-loading :loading="loading">
            <button
              v-for="page in pages"
              :key="page.id"
              class="page-row"
              :class="{ active: currentPage?.slug === page.slug }"
              @click="openPage(page.slug)"
            >
              <span class="page-title">{{ page.title || page.slug }}</span>
              <span class="page-summary">{{ page.summary || page.slug }}</span>
            </button>
          </t-loading>
        </aside>

        <article class="wiki-page">
          <template v-if="currentPage">
            <div class="page-heading">
              <div>
                <span class="page-type">{{ currentPage.page_type }}</span>
                <h2>{{ currentPage.title || currentPage.slug }}</h2>
              </div>
              <span class="page-version">v{{ currentPage.version }}</span>
            </div>
            <div class="page-meta">
              <div class="meta-item">
                <span>Slug</span>
                <strong>{{ currentPage.slug }}</strong>
              </div>
              <div class="meta-item">
                <span>状态</span>
                <strong>{{ currentPage.status }}</strong>
              </div>
              <div class="meta-item">
                <span>别名</span>
                <strong>{{ currentAliases.length || 0 }}</strong>
              </div>
              <div class="meta-item">
                <span>出链</span>
                <strong>{{ currentOutboundLinks.length }}</strong>
              </div>
            </div>
            <div class="markdown-body" v-html="renderedContent" />
            <div class="page-links">
              <div class="link-block">
                <h4>出链</h4>
                <div v-if="currentOutboundLinks.length" class="link-chips">
                  <button
                    v-for="link in currentOutboundLinks"
                    :key="link"
                    class="link-chip"
                    @click="openPage(link)"
                  >
                    {{ link }}
                  </button>
                </div>
                <p v-else class="link-empty">当前页面没有出链。</p>
              </div>
              <div class="link-block">
                <h4>入链</h4>
                <div v-if="currentInboundLinks.length" class="link-chips">
                  <button
                    v-for="link in currentInboundLinks"
                    :key="link"
                    class="link-chip"
                    @click="openPage(link)"
                  >
                    {{ link }}
                  </button>
                </div>
                <p v-else class="link-empty">当前页面没有入链。</p>
              </div>
            </div>
            <div v-if="currentSources.length || currentChunks.length || Object.keys(currentMetadata).length" class="page-extra">
              <div v-if="currentSources.length" class="extra-block">
                <h4>来源</h4>
                <p>{{ currentSources.join(' · ') }}</p>
              </div>
              <div v-if="currentChunks.length" class="extra-block">
                <h4>Chunk 引用</h4>
                <p>{{ currentChunks.join(' · ') }}</p>
              </div>
              <div v-if="Object.keys(currentMetadata).length" class="extra-block">
                <h4>元数据</h4>
                <pre>{{ JSON.stringify(currentMetadata, null, 2) }}</pre>
              </div>
            </div>
          </template>
          <section v-else class="empty-state compact">
            <h2>暂无 Wiki 页面</h2>
            <p>可以先创建起始页，自动生成链路会在后续阶段接入。</p>
          </section>
        </article>
      </main>

      <main v-else class="graph-layout">
        <div class="graph-panel">
          <div v-if="!graph.nodes.length" class="empty-state compact">
            <h2>暂无图谱数据</h2>
            <p>创建带有 [[wiki-link]] 的页面后可在这里查看关系。</p>
          </div>
          <div v-else class="graph-list">
            <button v-for="node in graph.nodes" :key="node.slug" class="graph-node" @click="openPage(node.slug)">
              <span>{{ node.title || node.slug }}</span>
              <small>{{ node.page_type }} · {{ node.link_count }} links</small>
            </button>
          </div>
        </div>
        <aside class="edge-panel">
          <h3>关系</h3>
          <div v-for="edge in graph.edges" :key="`${edge.source}-${edge.target}`" class="edge-row">
            <span>{{ edge.source }}</span>
            <t-icon name="arrow-right" />
            <span>{{ edge.target }}</span>
          </div>
        </aside>
      </main>
    </template>
  </div>
</template>

<style scoped>
.wiki-workspace {
  min-height: 100%;
  padding: 24px;
  background: #f5f7fa;
}

.wiki-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.breadcrumb {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  padding: 0;
  color: #4b5563;
  background: transparent;
  cursor: pointer;
}

.wiki-header h1 {
  margin: 8px 0 0;
  font-size: 24px;
  color: #111827;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.stat-item,
.wiki-sidebar,
.wiki-page,
.graph-panel,
.edge-panel,
.empty-state {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
}

.stat-item {
  padding: 14px 16px;
}

.stat-item span {
  display: block;
  color: #6b7280;
  font-size: 13px;
}

.stat-item strong {
  font-size: 24px;
  color: #111827;
}

.wiki-layout {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  gap: 16px;
}

.wiki-sidebar,
.wiki-page,
.graph-panel,
.edge-panel {
  min-height: 560px;
}

.wiki-sidebar {
  padding: 12px;
}

.sidebar-tools,
.sidebar-actions {
  display: grid;
  gap: 8px;
  margin-bottom: 12px;
}

.sidebar-note {
  margin-bottom: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #f7fafc;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.sidebar-actions {
  grid-template-columns: 1fr 1fr;
}

.page-row {
  display: block;
  width: 100%;
  border: 0;
  border-radius: 6px;
  padding: 10px;
  text-align: left;
  background: transparent;
  cursor: pointer;
}

.page-row:hover,
.page-row.active {
  background: #eef5ff;
}

.page-title,
.page-summary {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.page-title {
  color: #111827;
  font-weight: 600;
}

.page-summary {
  margin-top: 4px;
  color: #6b7280;
  font-size: 12px;
}

.wiki-page {
  padding: 24px;
}

.page-heading {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid #e5e7eb;
  padding-bottom: 16px;
  margin-bottom: 20px;
}

.page-heading h2 {
  margin: 6px 0 0;
  font-size: 22px;
}

.page-type,
.page-version {
  color: #6b7280;
  font-size: 12px;
  text-transform: uppercase;
}

.page-meta {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 18px;
}

.meta-item {
  padding: 12px 14px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fafbfc;
}

.meta-item span,
.meta-item strong {
  display: block;
}

.meta-item span {
  margin-bottom: 4px;
  color: #6b7280;
  font-size: 12px;
}

.meta-item strong {
  color: #111827;
  font-size: 13px;
  word-break: break-word;
}

.markdown-body {
  color: #1f2937;
  line-height: 1.75;
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3) {
  color: #111827;
}

.page-links,
.page-extra {
  margin-top: 18px;
  display: grid;
  gap: 14px;
}

.link-block,
.extra-block {
  padding: 14px 16px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  background: #fafbfc;
}

.link-block h4,
.extra-block h4 {
  margin: 0 0 10px;
  color: #111827;
  font-size: 14px;
}

.link-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.link-chip {
  border: 1px solid #dbe3ef;
  border-radius: 999px;
  padding: 6px 10px;
  background: #fff;
  color: #1d4ed8;
  cursor: pointer;
  font-size: 12px;
}

.link-chip:hover {
  border-color: #93c5fd;
  background: #eff6ff;
}

.link-empty,
.extra-block p,
.extra-block pre {
  margin: 0;
  color: #6b7280;
  font-size: 12px;
  line-height: 1.6;
}

.extra-block pre {
  white-space: pre-wrap;
  word-break: break-word;
}

.graph-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 360px;
  gap: 16px;
}

.graph-panel,
.edge-panel {
  padding: 16px;
}

.graph-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}

.graph-node {
  border: 1px solid #dbe3ef;
  border-radius: 8px;
  padding: 12px;
  text-align: left;
  background: #f8fbff;
  cursor: pointer;
}

.graph-node span,
.graph-node small {
  display: block;
}

.graph-node span {
  color: #111827;
  font-weight: 600;
}

.graph-node small {
  margin-top: 6px;
  color: #6b7280;
}

.edge-panel h3 {
  margin: 0 0 12px;
}

.edge-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 20px minmax(0, 1fr);
  gap: 8px;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid #eef2f7;
  color: #4b5563;
  font-size: 13px;
}

.edge-row span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-state {
  display: grid;
  place-items: center;
  min-height: 360px;
  padding: 32px;
  text-align: center;
  color: #4b5563;
}

.empty-state h2 {
  margin: 12px 0 6px;
  color: #111827;
}

.empty-state.compact {
  min-height: 240px;
}

@media (max-width: 900px) {
  .wiki-workspace {
    padding: 16px;
  }

  .wiki-header,
  .header-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .stats-row,
  .wiki-layout,
  .graph-layout {
    grid-template-columns: 1fr;
  }

  .page-meta {
    grid-template-columns: 1fr 1fr;
  }

  .wiki-sidebar,
  .wiki-page,
  .graph-panel,
  .edge-panel {
    min-height: auto;
  }
}

@media (max-width: 640px) {
  .page-meta {
    grid-template-columns: 1fr;
  }
}
</style>

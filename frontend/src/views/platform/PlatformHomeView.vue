<template>
  <div class="platform-home">
    <section class="hero-card">
      <div class="hero-copy">
        <span class="hero-badge">平台控制台</span>
        <h1>统一管理租户、模型与平台运行状态</h1>
        <p>面向平台超级管理员的总览页，聚合平台治理、租户规模与运行健康，减少在设置页中来回切换。</p>
      </div>
      <div class="hero-actions">
        <t-button theme="primary" size="large" @click="router.push('/platform/settings')">
          <template #icon><t-icon name="setting" /></template>
          平台设置
        </t-button>
        <t-button variant="outline" size="large" @click="router.push('/platform/usage-audit')">
          <template #icon><t-icon name="chart-bar" /></template>
          平台审计
        </t-button>
      </div>
    </section>

    <section class="stats-grid">
      <article class="stat-card">
        <span class="stat-label">租户总数</span>
        <strong class="stat-value">{{ tenantSummary.total }}</strong>
        <span class="stat-desc">当前可见的全部租户数量</span>
      </article>
      <article class="stat-card accent">
        <span class="stat-label">今日会话</span>
        <strong class="stat-value">{{ usageStats.today_sessions.toLocaleString() }}</strong>
        <span class="stat-desc">按当前平台超级管理员视角统计</span>
      </article>
      <article class="stat-card">
        <span class="stat-label">本周响应</span>
        <strong class="stat-value">{{ usageStats.week_responses.toLocaleString() }}</strong>
        <span class="stat-desc">近 7 天助手响应总数</span>
      </article>
      <article class="stat-card">
        <span class="stat-label">本月会话</span>
        <strong class="stat-value">{{ usageStats.month_sessions.toLocaleString() }}</strong>
        <span class="stat-desc">观察平台活跃度变化</span>
      </article>
    </section>

    <section class="panel-grid">
      <article class="panel-card">
        <div class="panel-header">
          <div>
            <h2>平台治理</h2>
            <p>进入设置页统一维护模型、存储和解析能力。</p>
          </div>
          <t-button theme="primary" variant="text" @click="router.push('/platform/settings')">打开设置</t-button>
        </div>
        <ul class="action-list">
          <li>租户切换与平台级配置在左侧侧边栏保留。</li>
          <li>普通租户默认不再进入这些平台级页面。</li>
          <li>后续若补齐租户管理页，可从这里继续扩展。</li>
        </ul>
      </article>

      <article class="panel-card">
        <div class="panel-header">
          <div>
            <h2>租户概览</h2>
            <p>便于快速判断当前平台规模。</p>
          </div>
        </div>
        <div v-if="loadingTenants" class="panel-state">
          <t-loading size="small" text="正在加载租户数据" />
        </div>
        <div v-else class="tenant-overview">
          <div class="tenant-pill">
            <span>租户总量</span>
            <strong>{{ tenantSummary.total }}</strong>
          </div>
          <div class="tenant-pill">
            <span>启用中</span>
            <strong>{{ tenantSummary.active }}</strong>
          </div>
          <div class="tenant-pill">
            <span>停用/异常</span>
            <strong>{{ tenantSummary.inactive }}</strong>
          </div>
        </div>
      </article>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { getUsageStats, type UsageStats } from '@/api/usage'
import { searchTenants, type TenantInfo } from '@/api/tenant'

const router = useRouter()

const loadingTenants = ref(false)
const tenants = ref<TenantInfo[]>([])
const usageStats = ref<UsageStats>({
  total_sessions: 0,
  total_responses: 0,
  today_sessions: 0,
  today_responses: 0,
  week_sessions: 0,
  week_responses: 0,
  month_sessions: 0,
  month_responses: 0,
  feedback_like: 0,
  feedback_dislike: 0,
  channel_breakdown: {}
})

const tenantSummary = computed(() => {
  const total = tenants.value.length
  const active = tenants.value.filter(item => item.status === 'active' || !item.status).length
  return {
    total,
    active,
    inactive: Math.max(total - active, 0)
  }
})

const loadUsageStats = async () => {
  try {
    const response = await getUsageStats()
    usageStats.value = {
      ...usageStats.value,
      ...(response?.data || {})
    }
  } catch (_) {
    // Keep dashboard usable even if stats are unavailable.
  }
}

const loadTenants = async () => {
  loadingTenants.value = true
  try {
    const response = await searchTenants({ page: 1, page_size: 100 })
    tenants.value = response.data?.items || []
  } catch (_) {
    tenants.value = []
  } finally {
    loadingTenants.value = false
  }
}

onMounted(() => {
  loadUsageStats()
  loadTenants()
})
</script>

<style scoped lang="less">
.platform-home {
  flex: 1;
  padding: 32px;
  overflow-y: auto;
  background:
    radial-gradient(circle at top right, rgba(13, 110, 253, 0.08), transparent 20%),
    linear-gradient(180deg, #f5f8fb 0%, #ffffff 260px);
}

.hero-card,
.stat-card,
.panel-card {
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.06);
}

.hero-card {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  padding: 28px 32px;
  border-radius: 24px;
}

.hero-badge {
  display: inline-flex;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(13, 110, 253, 0.08);
  color: #0d6efd;
  font-size: 12px;
  font-weight: 600;
}

.hero-copy h1 {
  margin: 10px 0 12px;
  font-size: 32px;
  line-height: 1.2;
  color: var(--td-text-color-primary);
}

.hero-copy p {
  margin: 0;
  max-width: 680px;
  color: var(--td-text-color-secondary);
  line-height: 1.7;
}

.hero-actions {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
  margin-top: 20px;
}

.stat-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 20px;
  border-radius: 20px;
}

.stat-card.accent {
  background: linear-gradient(135deg, rgba(13, 110, 253, 0.08) 0%, rgba(13, 110, 253, 0.02) 100%);
}

.stat-label {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.stat-value {
  font-size: 30px;
  color: var(--td-text-color-primary);
}

.stat-desc {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}

.panel-grid {
  display: grid;
  grid-template-columns: 1.2fr 0.8fr;
  gap: 16px;
  margin-top: 20px;
}

.panel-card {
  padding: 24px;
  border-radius: 20px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
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
}

.action-list {
  margin: 0;
  padding-left: 18px;
  color: var(--td-text-color-secondary);
  line-height: 1.8;
}

.tenant-overview {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.tenant-pill {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 18px;
  border-radius: 16px;
  background: var(--td-bg-color-secondarycontainer);
}

.tenant-pill span {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.tenant-pill strong {
  color: var(--td-text-color-primary);
  font-size: 28px;
}

.panel-state {
  min-height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
}

@media (max-width: 1200px) {
  .stats-grid,
  .panel-grid,
  .tenant-overview {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .platform-home {
    padding: 20px;
  }

  .hero-card,
  .stats-grid,
  .panel-grid,
  .tenant-overview {
    grid-template-columns: 1fr;
  }
}
</style>

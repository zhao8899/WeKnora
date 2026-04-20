<template>
  <div class="usage-audit-page">
    <div class="page-header">
      <h2 class="page-title">{{ $t('usage.title') }}</h2>
      <div class="period-tabs">
        <span
          v-for="p in periods"
          :key="p.key"
          :class="['period-tab', { active: selectedPeriod === p.key }]"
          @click="selectedPeriod = p.key"
        >{{ p.label }}</span>
      </div>
    </div>

    <!-- 概览卡片 -->
    <div class="stats-grid" v-if="stats">
      <div class="stat-card">
        <div class="stat-icon stat-icon-session">
          <t-icon name="chat" />
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ currentStats.sessions.toLocaleString() }}</div>
          <div class="stat-label">{{ $t('usage.sessions') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-response">
          <t-icon name="robot" />
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ currentStats.responses.toLocaleString() }}</div>
          <div class="stat-label">{{ $t('usage.responses') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-like">
          <t-icon name="thumb-up" />
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ stats.feedback_like.toLocaleString() }}</div>
          <div class="stat-label">{{ $t('usage.feedbackLike') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-dislike">
          <t-icon name="thumb-down" />
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ stats.feedback_dislike.toLocaleString() }}</div>
          <div class="stat-label">{{ $t('usage.feedbackDislike') }}</div>
        </div>
      </div>
    </div>

    <div class="charts-row">
      <!-- 趋势图 -->
      <div class="chart-card trend-chart-card">
        <div class="chart-header">
          <span class="chart-title">{{ $t('usage.dailyTrend') }}</span>
          <div class="chart-legend">
            <span class="legend-dot legend-session"></span>{{ $t('usage.sessions') }}
            <span class="legend-dot legend-response"></span>{{ $t('usage.responses') }}
          </div>
        </div>
        <div class="bar-chart" v-if="trend.length">
          <div class="bar-group" v-for="pt in trend" :key="pt.date">
            <div class="bars">
              <div
                class="bar bar-session"
                :style="{ height: barHeight(pt.sessions, maxValue) + 'px' }"
                :title="`${pt.date}: ${pt.sessions} ${$t('usage.sessions')}`"
              ></div>
              <div
                class="bar bar-response"
                :style="{ height: barHeight(pt.responses, maxValue) + 'px' }"
                :title="`${pt.date}: ${pt.responses} ${$t('usage.responses')}`"
              ></div>
            </div>
            <div class="bar-label">{{ formatBarLabel(pt.date) }}</div>
          </div>
        </div>
        <div v-else class="chart-empty">{{ $t('common.noData') }}</div>
      </div>

      <!-- 渠道分布 + 反馈比例 -->
      <div class="side-charts">
        <!-- 渠道分布 -->
        <div class="chart-card">
          <div class="chart-header">
            <span class="chart-title">{{ $t('usage.channelBreakdown') }}</span>
          </div>
          <div class="channel-list" v-if="stats && hasChannelData">
            <div class="channel-row" v-for="(count, ch) in stats.channel_breakdown" :key="ch">
              <span class="channel-name">{{ channelLabel(String(ch)) }}</span>
              <div class="channel-bar-wrap">
                <div
                  class="channel-bar"
                  :style="{ width: channelPercent(count) + '%' }"
                ></div>
              </div>
              <span class="channel-count">{{ count }}</span>
            </div>
          </div>
          <div v-else class="chart-empty">{{ $t('common.noData') }}</div>
        </div>

        <!-- 反馈质量 -->
        <div class="chart-card">
          <div class="chart-header">
            <span class="chart-title">{{ $t('usage.feedbackQuality') }}</span>
          </div>
          <div class="feedback-ring" v-if="stats && totalFeedback > 0">
            <div class="ring-container">
              <svg viewBox="0 0 80 80" class="ring-svg">
                <circle cx="40" cy="40" r="30" fill="none" stroke="var(--td-bg-color-secondarycontainer)" stroke-width="12" />
                <circle
                  cx="40" cy="40" r="30"
                  fill="none"
                  stroke="var(--td-success-color)"
                  stroke-width="12"
                  stroke-dasharray="188.5"
                  :stroke-dashoffset="188.5 * (1 - likePct)"
                  transform="rotate(-90 40 40)"
                />
              </svg>
              <div class="ring-center">
                <span class="ring-pct">{{ Math.round(likePct * 100) }}%</span>
                <span class="ring-sub">{{ $t('usage.positive') }}</span>
              </div>
            </div>
            <div class="feedback-legend">
              <div><span class="legend-dot legend-like"></span>{{ $t('chat.feedbackLike') }} {{ stats.feedback_like }}</div>
              <div><span class="legend-dot legend-dislike"></span>{{ $t('chat.feedbackDislike') }} {{ stats.feedback_dislike }}</div>
            </div>
          </div>
          <div v-else class="chart-empty">{{ $t('usage.noFeedback') }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { getUsageStats, getDailyTrend, type UsageStats, type DailyUsagePoint } from '@/api/usage/index';

const { t } = useI18n();

const stats = ref<UsageStats | null>(null);
const trend = ref<DailyUsagePoint[]>([]);
const selectedPeriod = ref<'today' | 'week' | 'month' | 'all'>('month');

const periods = computed(() => [
  { key: 'today' as const, label: t('usage.today') },
  { key: 'week'  as const, label: t('usage.thisWeek') },
  { key: 'month' as const, label: t('usage.thisMonth') },
  { key: 'all'   as const, label: t('usage.allTime') },
]);

const currentStats = computed(() => {
  if (!stats.value) return { sessions: 0, responses: 0 };
  const s = stats.value;
  switch (selectedPeriod.value) {
    case 'today': return { sessions: s.today_sessions, responses: s.today_responses };
    case 'week':  return { sessions: s.week_sessions,  responses: s.week_responses };
    case 'month': return { sessions: s.month_sessions, responses: s.month_responses };
    default:      return { sessions: s.total_sessions, responses: s.total_responses };
  }
});

const totalFeedback = computed(() => (stats.value?.feedback_like ?? 0) + (stats.value?.feedback_dislike ?? 0));
const likePct = computed(() => totalFeedback.value > 0 ? (stats.value?.feedback_like ?? 0) / totalFeedback.value : 0);

const hasChannelData = computed(() =>
  stats.value && Object.values(stats.value.channel_breakdown).some(v => v > 0)
);

const maxChannelCount = computed(() =>
  stats.value ? Math.max(...Object.values(stats.value.channel_breakdown), 1) : 1
);

const channelPercent = (count: number) =>
  Math.round((count / maxChannelCount.value) * 100);

const channelLabel = (ch: string) => {
  const map: Record<string, string> = { web: t('chat.channelWeb'), api: t('chat.channelApi'), im: t('chat.channelIm') };
  return map[ch] ?? ch;
};

const maxValue = computed(() => {
  const max = Math.max(...trend.value.flatMap(p => [p.sessions, p.responses]), 1);
  return max;
});

const BAR_MAX_HEIGHT = 120;
const barHeight = (val: number, max: number) =>
  max > 0 ? Math.max(2, Math.round((val / max) * BAR_MAX_HEIGHT)) : 2;

const formatBarLabel = (date: string) => {
  // Show MM/DD for the first of each month and every 7th bar
  const d = new Date(date);
  const idx = trend.value.findIndex(p => p.date === date);
  if (idx === 0 || d.getDate() === 1 || idx % 7 === 0) {
    return `${d.getMonth() + 1}/${d.getDate()}`;
  }
  return '';
};

onMounted(async () => {
  try {
    const [statsRes, trendRes] = await Promise.all([
      getUsageStats(),
      getDailyTrend(30),
    ]);
    stats.value = statsRes.data;
    trend.value = trendRes.data;
  } catch (e) {
    console.error('加载使用量数据失败', e);
  }
});
</script>

<style lang="less" scoped>
.usage-audit-page {
  padding: 24px 32px;
  max-width: 1100px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0;
}

.period-tabs {
  display: flex;
  gap: 4px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  padding: 3px;
}

.period-tab {
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
  color: var(--td-text-color-secondary);
  transition: all 0.15s;

  &.active {
    background: var(--td-bg-color-container);
    color: var(--td-text-color-primary);
    font-weight: 500;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  }

  &:hover:not(.active) {
    color: var(--td-text-color-primary);
  }
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 20px;

  @media (max-width: 900px) {
    grid-template-columns: repeat(2, 1fr);
  }
}

.stat-card {
  background: var(--td-bg-color-container);
  border-radius: 8px;
  padding: 16px;
  display: flex;
  align-items: center;
  gap: 14px;
  border: 1px solid var(--td-component-stroke);
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;

  &.stat-icon-session  { background: rgba(0, 106, 255, 0.08); color: var(--td-brand-color); }
  &.stat-icon-response { background: rgba(0, 168, 112, 0.08); color: var(--td-success-color); }
  &.stat-icon-like     { background: rgba(0, 168, 112, 0.08); color: var(--td-success-color); }
  &.stat-icon-dislike  { background: rgba(229, 75, 75, 0.08);  color: var(--td-error-color); }
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--td-text-color-primary);
  line-height: 1.2;
}

.stat-label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 2px;
}

.charts-row {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.trend-chart-card {
  flex: 1;
  min-width: 0;
}

.side-charts {
  width: 280px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.chart-card {
  background: var(--td-bg-color-container);
  border-radius: 8px;
  padding: 16px;
  border: 1px solid var(--td-component-stroke);
}

.chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.chart-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.chart-legend {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.legend-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 4px;

  &.legend-session  { background: var(--td-brand-color); }
  &.legend-response { background: var(--td-success-color); }
  &.legend-like     { background: var(--td-success-color); }
  &.legend-dislike  { background: var(--td-error-color); }
}

.bar-chart {
  display: flex;
  align-items: flex-end;
  gap: 3px;
  height: 150px;
  padding-bottom: 24px;
  overflow-x: auto;
}

.bar-group {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
  min-width: 14px;
  position: relative;
}

.bars {
  display: flex;
  align-items: flex-end;
  gap: 1px;
}

.bar {
  width: 5px;
  border-radius: 2px 2px 0 0;
  transition: height 0.3s ease;
  cursor: pointer;

  &.bar-session  { background: var(--td-brand-color); opacity: 0.8; }
  &.bar-response { background: var(--td-success-color); opacity: 0.8; }

  &:hover { opacity: 1; }
}

.bar-label {
  position: absolute;
  bottom: -20px;
  font-size: 10px;
  color: var(--td-text-color-placeholder);
  white-space: nowrap;
}

.chart-empty {
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
  padding: 32px 0;
}

.channel-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.channel-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.channel-name {
  width: 36px;
  color: var(--td-text-color-secondary);
  flex-shrink: 0;
}

.channel-bar-wrap {
  flex: 1;
  height: 6px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 3px;
  overflow: hidden;
}

.channel-bar {
  height: 100%;
  background: var(--td-brand-color);
  border-radius: 3px;
  transition: width 0.4s ease;
}

.channel-count {
  width: 30px;
  text-align: right;
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.feedback-ring {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.ring-container {
  position: relative;
  width: 80px;
  height: 80px;
}

.ring-svg {
  width: 100%;
  height: 100%;
  transform: rotate(0deg);
}

.ring-center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.ring-pct {
  font-size: 16px;
  font-weight: 700;
  color: var(--td-text-color-primary);
}

.ring-sub {
  font-size: 10px;
  color: var(--td-text-color-secondary);
}

.feedback-legend {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
</style>

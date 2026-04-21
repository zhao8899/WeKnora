<template>
  <div class="usage-audit-page">
    <section class="hero-card">
      <div class="hero-main">
        <div class="hero-copy">
          <span class="hero-eyebrow">{{ $t('usage.title') }}</span>
          <h2 class="page-title">{{ periodLabel }}{{ $t('usage.sessions') }} / {{ $t('usage.responses') }}</h2>
          <p class="hero-description">
            {{ summaryText }}
          </p>
        </div>
        <div class="period-tabs">
          <span
            v-for="p in periods"
            :key="p.key"
            :class="['period-tab', { active: selectedPeriod === p.key }]"
            @click="selectedPeriod = p.key"
          >{{ p.label }}</span>
        </div>
      </div>

      <div class="hero-metrics">
        <div class="hero-metric">
          <span class="hero-metric-label">{{ $t('usage.sessions') }}</span>
          <strong>{{ currentStats.sessions.toLocaleString() }}</strong>
        </div>
        <div class="hero-metric">
          <span class="hero-metric-label">{{ $t('usage.responses') }}</span>
          <strong>{{ currentStats.responses.toLocaleString() }}</strong>
        </div>
        <div class="hero-metric">
          <span class="hero-metric-label">AI / Session</span>
          <strong>{{ responsePerSession }}</strong>
        </div>
        <div class="hero-metric">
          <span class="hero-metric-label">{{ $t('usage.feedbackQuality') }}</span>
          <strong>{{ feedbackRate }}</strong>
        </div>
      </div>
    </section>

    <div class="stats-grid">
      <div class="stat-card stat-card-primary">
        <div :class="['stat-icon', 'stat-icon-session']">
          <t-icon name="chat" />
        </div>
        <div class="stat-content">
          <div class="stat-label">{{ $t('usage.sessions') }}</div>
          <div class="stat-value">{{ currentStats.sessions.toLocaleString() }}</div>
          <div class="stat-meta">{{ periodLabel }}总会话量</div>
        </div>
      </div>

      <div class="stat-card">
        <div :class="['stat-icon', 'stat-icon-response']">
          <t-icon name="robot" />
        </div>
        <div class="stat-content">
          <div class="stat-label">{{ $t('usage.responses') }}</div>
          <div class="stat-value">{{ currentStats.responses.toLocaleString() }}</div>
          <div class="stat-meta">平均每会话 {{ responsePerSession }} 次</div>
        </div>
      </div>

      <div class="stat-card">
        <div :class="['stat-icon', 'stat-icon-like']">
          <t-icon name="thumb-up" />
        </div>
        <div class="stat-content">
          <div class="stat-label">{{ $t('usage.feedbackLike') }}</div>
          <div class="stat-value">{{ (stats?.feedback_like ?? 0).toLocaleString() }}</div>
          <div class="stat-meta">正向反馈占比 {{ feedbackRate }}</div>
        </div>
      </div>

      <div class="stat-card">
        <div :class="['stat-icon', 'stat-icon-channel']">
          <t-icon name="chart-column" />
        </div>
        <div class="stat-content">
          <div class="stat-label">{{ $t('usage.channelBreakdown') }}</div>
          <div class="stat-value">{{ activeChannelCount }}</div>
          <div class="stat-meta">{{ dominantChannelLabel }}</div>
        </div>
      </div>
    </div>

    <section class="analytics-grid">
      <div class="trend-panel panel-card">
        <div class="panel-header">
          <div>
            <div class="panel-title">{{ $t('usage.dailyTrend') }}</div>
            <div class="panel-subtitle">按日观察会话量与 AI 回复量变化</div>
          </div>
          <div class="chart-legend">
            <span class="legend-item">
              <span class="legend-dot legend-session"></span>{{ $t('usage.sessions') }}
            </span>
            <span class="legend-item">
              <span class="legend-dot legend-response"></span>{{ $t('usage.responses') }}
            </span>
          </div>
        </div>

        <div v-if="trend.length" class="trend-chart-shell">
          <div class="bar-chart">
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
        </div>
        <div v-else class="chart-empty chart-empty-large">
          <t-icon name="chart-column" />
          <span>{{ $t('common.noData') }}</span>
        </div>
      </div>

      <div class="side-panels">
        <div class="panel-card">
          <div class="panel-header">
            <div>
              <div class="panel-title">{{ $t('usage.channelBreakdown') }}</div>
              <div class="panel-subtitle">当前已接入渠道活跃度分布</div>
            </div>
          </div>

          <div v-if="channelEntries.length" class="channel-list">
            <div class="channel-row" v-for="[ch, count] in channelEntries" :key="ch">
              <div class="channel-label-row">
                <span class="channel-name">{{ channelLabel(ch) }}</span>
                <span class="channel-count">{{ count }}</span>
              </div>
              <div class="channel-bar-wrap">
                <div class="channel-bar" :style="{ width: channelPercent(count) + '%' }"></div>
              </div>
            </div>
          </div>
          <div v-else class="chart-empty">
            <t-icon name="chart-column" />
            <span>{{ $t('common.noData') }}</span>
          </div>
        </div>

        <div class="panel-card feedback-card">
          <div class="panel-header">
            <div>
              <div class="panel-title">{{ $t('usage.feedbackQuality') }}</div>
              <div class="panel-subtitle">基于用户点赞与点踩的整体反馈</div>
            </div>
          </div>

          <div v-if="totalFeedback > 0" class="feedback-ring">
            <div class="ring-container">
              <svg viewBox="0 0 120 120" class="ring-svg">
                <defs>
                  <linearGradient id="feedbackGradient" x1="0%" y1="0%" x2="100%" y2="100%">
                    <stop offset="0%" stop-color="#11c48b" />
                    <stop offset="100%" stop-color="#2f7cf6" />
                  </linearGradient>
                </defs>
                <circle cx="60" cy="60" r="44" class="ring-track" />
                <circle
                  cx="60"
                  cy="60"
                  r="44"
                  class="ring-progress"
                  :stroke-dasharray="ringCircumference"
                  :stroke-dashoffset="ringCircumference * (1 - likePct)"
                  transform="rotate(-90 60 60)"
                />
              </svg>
              <div class="ring-center">
                <span class="ring-pct">{{ feedbackRate }}</span>
                <span class="ring-sub">{{ $t('usage.positive') }}</span>
              </div>
            </div>

            <div class="feedback-stats">
              <div class="feedback-stat">
                <span class="legend-dot legend-like"></span>
                <span>{{ $t('chat.feedbackLike') }}</span>
                <strong>{{ stats?.feedback_like ?? 0 }}</strong>
              </div>
              <div class="feedback-stat">
                <span class="legend-dot legend-dislike"></span>
                <span>{{ $t('chat.feedbackDislike') }}</span>
                <strong>{{ stats?.feedback_dislike ?? 0 }}</strong>
              </div>
            </div>
          </div>
          <div v-else class="chart-empty">
            <t-icon name="thumb-up" />
            <span>{{ $t('usage.noFeedback') }}</span>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { getDailyTrend, getUsageStats, type DailyUsagePoint, type UsageStats } from '@/api/usage/index';

const { t } = useI18n();

const stats = ref<UsageStats | null>(null);
const trend = ref<DailyUsagePoint[]>([]);
const selectedPeriod = ref<'today' | 'week' | 'month' | 'all'>('month');

const periods = computed(() => [
  { key: 'today' as const, label: t('usage.today') },
  { key: 'week' as const, label: t('usage.thisWeek') },
  { key: 'month' as const, label: t('usage.thisMonth') },
  { key: 'all' as const, label: t('usage.allTime') },
]);

const currentStats = computed(() => {
  if (!stats.value) return { sessions: 0, responses: 0 };
  const s = stats.value;
  switch (selectedPeriod.value) {
    case 'today':
      return { sessions: s.today_sessions, responses: s.today_responses };
    case 'week':
      return { sessions: s.week_sessions, responses: s.week_responses };
    case 'month':
      return { sessions: s.month_sessions, responses: s.month_responses };
    default:
      return { sessions: s.total_sessions, responses: s.total_responses };
  }
});

const periodLabel = computed(() => periods.value.find((item) => item.key === selectedPeriod.value)?.label ?? '');

const totalFeedback = computed(() => (stats.value?.feedback_like ?? 0) + (stats.value?.feedback_dislike ?? 0));
const likePct = computed(() => {
  if (totalFeedback.value <= 0) return 0;
  return (stats.value?.feedback_like ?? 0) / totalFeedback.value;
});

const feedbackRate = computed(() => `${Math.round(likePct.value * 100)}%`);

const responsePerSession = computed(() => {
  if (currentStats.value.sessions <= 0) return '0.0';
  return (currentStats.value.responses / currentStats.value.sessions).toFixed(1);
});

const channelEntries = computed<[string, number][]>(() => {
  const breakdown = stats.value?.channel_breakdown ?? {};
  return Object.entries(breakdown)
    .filter(([, count]) => count > 0)
    .sort((a, b) => b[1] - a[1]);
});

const activeChannelCount = computed(() => channelEntries.value.length);

const dominantChannelLabel = computed(() => {
  const first = channelEntries.value[0];
  if (!first) return '暂无活跃渠道';
  return `${channelLabel(first[0])} 占主导`;
});

const summaryText = computed(() => {
  if (currentStats.value.sessions === 0 && currentStats.value.responses === 0) {
    return '当前时间范围内还没有统计数据，建议先从会话入口、渠道接入或反馈收集开始观察。';
  }
  return `${periodLabel.value}内累计 ${currentStats.value.sessions.toLocaleString()} 次会话，产出 ${currentStats.value.responses.toLocaleString()} 条 AI 回复，反馈正向率 ${feedbackRate.value}。`;
});

const maxChannelCount = computed(() => Math.max(...channelEntries.value.map(([, count]) => count), 1));

const channelPercent = (count: number) => Math.round((count / maxChannelCount.value) * 100);

const channelLabel = (ch: string) => {
  const map: Record<string, string> = {
    web: t('chat.channelWeb'),
    api: t('chat.channelApi'),
    im: t('chat.channelIm'),
  };
  return map[ch] ?? ch;
};

const maxValue = computed(() => Math.max(...trend.value.flatMap((point) => [point.sessions, point.responses]), 1));

const BAR_MAX_HEIGHT = 220;
const ringCircumference = 2 * Math.PI * 44;

const barHeight = (value: number, max: number) => {
  if (max <= 0) return 8;
  return Math.max(8, Math.round((value / max) * BAR_MAX_HEIGHT));
};

const formatBarLabel = (date: string) => {
  const current = new Date(date);
  const idx = trend.value.findIndex((item) => item.date === date);
  if (idx === 0 || current.getDate() === 1 || idx % 7 === 0) {
    return `${current.getMonth() + 1}/${current.getDate()}`;
  }
  return '';
};

onMounted(async () => {
  try {
    const [statsRes, trendRes] = await Promise.all([getUsageStats(), getDailyTrend(30)]);
    stats.value = statsRes.data;
    trend.value = trendRes.data;
  } catch (error) {
    console.error('Failed to load usage stats', error);
  }
});
</script>

<style lang="less" scoped>
.usage-audit-page {
  flex: 1;
  width: 100%;
  min-width: 0;
  min-height: 100vh;
  box-sizing: border-box;
  align-self: stretch;
  overflow-x: hidden;
  padding: 28px 32px 40px;
  max-width: none;
  margin: 0;
  background:
    radial-gradient(circle at top left, rgba(47, 124, 246, 0.06), transparent 18%),
    linear-gradient(180deg, #fbfdff 0%, #f4f8fc 100%);
  color: var(--td-text-color-primary);
}

.hero-card,
.panel-card,
.stat-card {
  background:
    radial-gradient(circle at top left, rgba(47, 124, 246, 0.08), transparent 32%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(247, 250, 253, 0.98));
  border: 1px solid rgba(15, 23, 42, 0.08);
  box-shadow: 0 18px 42px rgba(15, 23, 42, 0.06);
}

.hero-card {
  border-radius: 24px;
  padding: 26px 28px;
  margin-bottom: 20px;
}

.hero-main {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
}

.hero-copy {
  max-width: 620px;
}

.hero-eyebrow {
  display: inline-flex;
  align-items: center;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(47, 124, 246, 0.1);
  color: #2f7cf6;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.page-title {
  margin: 12px 0 10px;
  font-size: 30px;
  line-height: 1.2;
  font-weight: 700;
}

.hero-description {
  margin: 0;
  font-size: 14px;
  line-height: 1.7;
  color: var(--td-text-color-secondary);
}

.period-tabs {
  display: inline-flex;
  gap: 6px;
  padding: 6px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.04);
  flex-shrink: 0;
}

.period-tab {
  padding: 8px 14px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: all 0.2s ease;

  &.active {
    color: #fff;
    background: linear-gradient(135deg, #101828, #2f7cf6);
    box-shadow: 0 10px 20px rgba(47, 124, 246, 0.22);
  }

  &:hover:not(.active) {
    color: var(--td-text-color-primary);
    background: rgba(255, 255, 255, 0.75);
  }
}

.hero-metrics {
  margin-top: 24px;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.hero-metric {
  border-radius: 18px;
  padding: 16px 18px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid rgba(15, 23, 42, 0.06);

  strong {
    display: block;
    margin-top: 8px;
    font-size: 24px;
    line-height: 1;
  }
}

.hero-metric-label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px;
  border-radius: 20px;
  min-height: 128px;
}

.stat-card-primary {
  background:
    radial-gradient(circle at top left, rgba(17, 196, 139, 0.12), transparent 28%),
    linear-gradient(135deg, rgba(16, 24, 40, 0.98), rgba(34, 93, 204, 0.94));
  color: #fff;

  .stat-label,
  .stat-meta,
  .stat-value {
    color: inherit;
  }
}

.stat-icon {
  width: 52px;
  height: 52px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex-shrink: 0;
}

.stat-icon-session {
  background: rgba(255, 255, 255, 0.16);
  color: #fff;
}

.stat-icon-response {
  background: rgba(17, 196, 139, 0.12);
  color: #11c48b;
}

.stat-icon-like {
  background: rgba(47, 124, 246, 0.12);
  color: #2f7cf6;
}

.stat-icon-channel {
  background: rgba(245, 158, 11, 0.14);
  color: #b45309;
}

.stat-content {
  min-width: 0;
}

.stat-label {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.stat-value {
  margin-top: 8px;
  font-size: 30px;
  line-height: 1;
  font-weight: 700;
}

.stat-meta {
  margin-top: 10px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.analytics-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.65fr) minmax(300px, 0.95fr);
  gap: 16px;
  align-items: start;
}

.side-panels {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.panel-card {
  border-radius: 24px;
  padding: 22px;
}

.panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.panel-title {
  font-size: 16px;
  font-weight: 700;
}

.panel-subtitle {
  margin-top: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.chart-legend {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
}

.legend-item {
  display: inline-flex;
  align-items: center;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  flex-shrink: 0;
}

.legend-session {
  background: #2f7cf6;
}

.legend-response {
  background: #11c48b;
}

.legend-like {
  background: #11c48b;
}

.legend-dislike {
  background: #ef4444;
}

.trend-chart-shell {
  border-radius: 20px;
  padding: 18px 16px 28px;
  background:
    linear-gradient(180deg, rgba(240, 247, 255, 0.85), rgba(255, 255, 255, 0.7));
  border: 1px solid rgba(47, 124, 246, 0.08);
}

.bar-chart {
  display: flex;
  align-items: flex-end;
  gap: 6px;
  min-height: 260px;
  overflow-x: auto;
  padding-bottom: 26px;
}

.bar-group {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
  min-width: 18px;
  position: relative;
}

.bars {
  display: flex;
  align-items: flex-end;
  gap: 3px;
}

.bar {
  width: 7px;
  min-height: 8px;
  border-radius: 999px 999px 2px 2px;
  transition: transform 0.18s ease, opacity 0.18s ease;
  cursor: pointer;

  &:hover {
    opacity: 0.92;
    transform: translateY(-2px);
  }
}

.bar-session {
  background: linear-gradient(180deg, #67a7ff, #2f7cf6);
}

.bar-response {
  background: linear-gradient(180deg, #53ddb0, #11c48b);
}

.bar-label {
  position: absolute;
  bottom: -24px;
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  white-space: nowrap;
}

.channel-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.channel-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.channel-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.channel-name {
  font-size: 13px;
  font-weight: 600;
}

.channel-count {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.channel-bar-wrap {
  height: 10px;
  border-radius: 999px;
  overflow: hidden;
  background: rgba(15, 23, 42, 0.06);
}

.channel-bar {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #2f7cf6, #11c48b);
  transition: width 0.35s ease;
}

.feedback-card {
  min-height: 318px;
}

.feedback-ring {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 22px;
  padding-top: 8px;
}

.ring-container {
  position: relative;
  width: 180px;
  height: 180px;
}

.ring-svg {
  width: 100%;
  height: 100%;
}

.ring-track {
  fill: none;
  stroke: rgba(15, 23, 42, 0.08);
  stroke-width: 14;
}

.ring-progress {
  fill: none;
  stroke: url(#feedbackGradient);
  stroke-width: 14;
  stroke-linecap: round;
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
  font-size: 34px;
  font-weight: 700;
  line-height: 1;
}

.ring-sub {
  margin-top: 8px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.feedback-stats {
  width: 100%;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.feedback-stat {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 16px;
  background: rgba(15, 23, 42, 0.04);
  font-size: 13px;

  strong {
    margin-left: auto;
    font-size: 15px;
  }
}

.chart-empty {
  min-height: 168px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
  border-radius: 20px;
  background: rgba(15, 23, 42, 0.025);

  .t-icon {
    font-size: 24px;
    opacity: 0.75;
  }
}

.chart-empty-large {
  min-height: 286px;
}

@media (max-width: 1080px) {
  .hero-main,
  .panel-header {
    flex-direction: column;
    align-items: stretch;
  }

  .hero-metrics,
  .stats-grid,
  .feedback-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .analytics-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .usage-audit-page {
    padding: 18px 16px 28px;
  }

  .page-title {
    font-size: 24px;
  }

  .hero-card,
  .panel-card,
  .stat-card {
    border-radius: 18px;
  }

  .hero-card,
  .panel-card {
    padding: 18px;
  }

  .hero-metrics,
  .stats-grid,
  .feedback-stats {
    grid-template-columns: 1fr;
  }

  .period-tabs {
    width: 100%;
    overflow-x: auto;
  }

  .period-tab {
    white-space: nowrap;
  }

  .bar-chart {
    min-height: 220px;
  }

  .ring-container {
    width: 150px;
    height: 150px;
  }
}
</style>

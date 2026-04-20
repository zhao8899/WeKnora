import { get } from '../../utils/request';

export interface UsageStats {
  total_sessions: number;
  total_responses: number;
  today_sessions: number;
  today_responses: number;
  week_sessions: number;
  week_responses: number;
  month_sessions: number;
  month_responses: number;
  feedback_like: number;
  feedback_dislike: number;
  channel_breakdown: Record<string, number>;
}

export interface DailyUsagePoint {
  date: string;
  sessions: number;
  responses: number;
}

export function getUsageStats() {
  return get('/api/v1/usage/stats');
}

export function getDailyTrend(days = 30) {
  return get(`/api/v1/usage/daily-trend?days=${days}`);
}

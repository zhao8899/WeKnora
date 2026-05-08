import { get } from '../../utils/request'

export interface HotQuestion {
  message_id: string
  session_id: string
  question: string
  retrieved_count: number
  reranked_count: number
  cited_count: number
  last_access_at: string
}

export interface CoverageGap {
  message_id: string
  session_id: string
  question: string
  confidence_score: number
  confidence_label: string
  evidence_strength_score: number
  evidence_strength_label: string
  source_health_score: number
  source_health_label: string
  source_count: number
  answer_created_at: string
}

export interface StaleDocument {
  knowledge_id: string
  title: string
  source_weight: number
  down_feedback_count: number
  expired_feedback_count: number
  freshness_flag: boolean
  source_health_score: number
  source_health_label: string
  health_status: string
  last_feedback_at: string
}

export interface CitationHeat {
  knowledge_id: string
  title: string
  cited_count: number
  reranked_count: number
  retrieved_count: number
  source_weight: number
  freshness_flag: boolean
  source_health_score: number
  source_health_label: string
  health_status: string
}

export interface AnalyticsQuery {
  limit?: number
  knowledge_base_id?: string
  session_id?: string
  message_id?: string
}

const toLimit = (query?: AnalyticsQuery) => query?.limit || 10

const analyticsPath = (path: string, query?: AnalyticsQuery) => {
  const params = new URLSearchParams()
  params.set('limit', String(toLimit(query)))
  if (query?.knowledge_base_id) params.set('knowledge_base_id', query.knowledge_base_id)
  if (query?.session_id) params.set('session_id', query.session_id)
  if (query?.message_id) params.set('message_id', query.message_id)
  return `/api/v1/analytics/${path}?${params.toString()}`
}

export function getHotQuestions(query?: AnalyticsQuery) {
  return get(analyticsPath('hot-questions', query))
}

export function getCoverageGaps(query?: AnalyticsQuery) {
  return get(analyticsPath('coverage-gaps', query))
}

export function getStaleDocuments(query?: AnalyticsQuery) {
  return get(analyticsPath('stale-documents', query))
}

export function getCitationHeatmap(query?: AnalyticsQuery) {
  return get(analyticsPath('citation-heatmap', query))
}

export interface PendingKnowledgeQuestion {
  message_id: string
  session_id: string
  question: string
  answer_created_at?: string
  source_count?: number
  question_freq?: number
  last_question_at?: string
  reason?: string
  priority?: string
  created_at?: string
}

export function getPendingKnowledgeQuestions(query?: AnalyticsQuery) {
  return get(analyticsPath('unanswered-questions', query))
}

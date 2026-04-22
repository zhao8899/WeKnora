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

export function getHotQuestions(limit = 10) {
  return get(`/api/v1/analytics/hot-questions?limit=${limit}`)
}

export function getCoverageGaps(limit = 10) {
  return get(`/api/v1/analytics/coverage-gaps?limit=${limit}`)
}

export function getStaleDocuments(limit = 10) {
  return get(`/api/v1/analytics/stale-documents?limit=${limit}`)
}

export function getCitationHeatmap(limit = 10) {
  return get(`/api/v1/analytics/citation-heatmap?limit=${limit}`)
}

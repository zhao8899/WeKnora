import { get, post } from '@/utils/request'

export interface EvaluationRequest {
  dataset_id: string
  knowledge_base_id?: string
  chat_id?: string
  rerank_id?: string
}

export interface EvaluationTask {
  id: string
  tenant_id: number
  dataset_id: string
  start_time: string
  status: number
  err_msg?: string
  total?: number
  finished?: number
}

export interface RetrievalMetrics {
  precision: number
  recall: number
  ndcg3: number
  ndcg10: number
  mrr: number
  map: number
}

export interface GenerationMetrics {
  bleu1: number
  bleu2: number
  bleu4: number
  rouge1: number
  rouge2: number
  rougel: number
}

export interface EvaluationMetricResult {
  retrieval_metrics: RetrievalMetrics
  generation_metrics: GenerationMetrics
}

export interface EvaluationDetail {
  task: EvaluationTask
  params?: Record<string, unknown>
  metric?: EvaluationMetricResult
}

export function startEvaluation(payload: EvaluationRequest) {
  return post('/api/v1/evaluation/', payload)
}

export function getEvaluationResult(taskId: string) {
  return get(`/api/v1/evaluation/?task_id=${encodeURIComponent(taskId)}`)
}

import { del, get, post, put } from '@/utils/request'

function encodeSlugPath(slug: string): string {
  return slug.split('/').map(encodeURIComponent).join('/')
}

export interface WikiPage {
  id: string
  tenant_id: number
  knowledge_base_id: string
  slug: string
  title: string
  page_type: string
  status: string
  content: string
  summary: string
  aliases: string[]
  source_refs: string[]
  chunk_refs: string[]
  in_links: string[]
  out_links: string[]
  page_metadata: Record<string, unknown>
  version: number
  created_at: string
  updated_at: string
}

export interface WikiPageListResponse {
  pages: WikiPage[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface WikiGraphData {
  nodes: { slug: string; title: string; page_type: string; link_count: number }[]
  edges: { source: string; target: string }[]
}

export interface WikiStats {
  total_pages: number
  pages_by_type: Record<string, number>
  total_links: number
  orphan_count: number
  recent_updates: WikiPage[]
  pending_tasks: number
  pending_issues: number
  is_active: boolean
}

export function listWikiPages(kbId: string, params?: {
  page_type?: string
  status?: string
  query?: string
  page?: number
  page_size?: number
  sort_by?: string
  sort_order?: string
}): Promise<{ data: WikiPageListResponse }> {
  const query = new URLSearchParams()
  Object.entries(params || {}).forEach(([key, value]) => {
    if (value !== undefined && value !== '') query.set(key, String(value))
  })
  const qs = query.toString()
  return get(`/api/v1/knowledgebase/${kbId}/wiki/pages${qs ? `?${qs}` : ''}`)
}

export function createWikiPage(kbId: string, data: Partial<WikiPage>): Promise<{ data: WikiPage }> {
  return post(`/api/v1/knowledgebase/${kbId}/wiki/pages`, data)
}

export function getWikiPage(kbId: string, slug: string): Promise<{ data: WikiPage }> {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}`)
}

export function updateWikiPage(kbId: string, slug: string, data: Partial<WikiPage>): Promise<{ data: WikiPage }> {
  return put(`/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}`, data)
}

export function deleteWikiPage(kbId: string, slug: string) {
  return del(`/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}`)
}

export function getWikiIndex(kbId: string): Promise<{ data: WikiPage }> {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/index`)
}

export function getWikiLog(kbId: string): Promise<{ data: WikiPage }> {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/log`)
}

export function getWikiGraph(kbId: string): Promise<{ data: WikiGraphData }> {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/graph`)
}

export function getWikiStats(kbId: string): Promise<{ data: WikiStats }> {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/stats`)
}

export function searchWikiPages(kbId: string, q: string, limit?: number): Promise<{ data: { pages: WikiPage[] } }> {
  const params = new URLSearchParams({ q })
  if (limit) params.set('limit', String(limit))
  return get(`/api/v1/knowledgebase/${kbId}/wiki/search?${params.toString()}`)
}

export function rebuildWikiLinks(kbId: string) {
  return post(`/api/v1/knowledgebase/${kbId}/wiki/rebuild-links`, {})
}

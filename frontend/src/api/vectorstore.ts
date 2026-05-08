import { del, get, post, put } from '@/utils/request'

export interface VectorStoreEntity {
  id?: string
  name: string
  engine_type: string
  connection_config: Record<string, any>
  index_config: Record<string, any>
  source?: 'env' | 'user'
  readonly?: boolean
  knowledge_base_count?: number
  tenant_id?: number
  created_at?: string
  updated_at?: string
}

export interface VectorStoreFieldInfo {
  name: string
  type: 'string' | 'number' | 'boolean'
  required: boolean
  sensitive?: boolean
  description?: string
  default?: any
}

export interface VectorStoreTypeInfo {
  type: string
  display_name: string
  supports_knowledge_base_binding?: boolean
  supports_index_config?: boolean
  connection_fields: VectorStoreFieldInfo[]
  index_fields?: VectorStoreFieldInfo[]
}

export interface VectorStoreKnowledgeBaseBinding {
  id: string
  name: string
  type: string
  vector_store_id?: string | null
  knowledge_count?: number
  chunk_count?: number
  updated_at?: string
  is_temporary?: boolean
}

export function listVectorStoreTypes(): Promise<VectorStoreTypeInfo[]> {
  return get('/api/v1/vector-stores/types').then((res: any) => (res?.success && res.data ? res.data : []))
}

export function listVectorStores(): Promise<{ success: boolean; data: VectorStoreEntity[] }> {
  return get('/api/v1/vector-stores')
}

export function createVectorStore(data: Partial<VectorStoreEntity>) {
  return post('/api/v1/vector-stores', data)
}

export function updateVectorStore(id: string, data: Partial<VectorStoreEntity>) {
  return put(`/api/v1/vector-stores/${id}`, data)
}

export function deleteVectorStore(id: string) {
  return del(`/api/v1/vector-stores/${id}`)
}

export function testVectorStoreRaw(data: { engine_type: string; connection_config: Record<string, any> }) {
  return post('/api/v1/vector-stores/test', data)
}

export function testVectorStoreById(id: string) {
  return post(`/api/v1/vector-stores/${id}/test`, {})
}

export function getVectorStore(id: string): Promise<{ success: boolean; data: VectorStoreEntity }> {
  return get(`/api/v1/vector-stores/${id}`)
}

export function listKnowledgeBasesForVectorStore(storeId?: string): Promise<VectorStoreKnowledgeBaseBinding[]> {
  return get('/api/v1/knowledge-bases').then((res: any) => {
    const items = Array.isArray(res?.data) ? res.data : []
    const bindings = items
      .filter((item: any) => !storeId || String(item?.vector_store_id || '') === storeId)
      .map((item: any) => ({
        id: String(item?.id || ''),
        name: item?.name || '',
        type: item?.type || 'document',
        vector_store_id: item?.vector_store_id || null,
        knowledge_count: Number(item?.knowledge_count || 0),
        chunk_count: Number(item?.chunk_count || 0),
        updated_at: item?.updated_at,
        is_temporary: !!item?.is_temporary,
      }))
      .filter((item: VectorStoreKnowledgeBaseBinding) => !!item.id)

    return bindings
  })
}

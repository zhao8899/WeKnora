export type ChatMode = 'chat' | 'rag_fast' | 'rag_deep' | 'agent'

export interface MentionedItemPayload {
  id: string
  name: string
  type: string
  kb_type?: string
}

export interface ImageAttachmentPayload {
  data: string
}

export interface StartChatStreamParams {
  session_id: string
  query: string
  knowledge_base_ids?: string[]
  knowledge_ids?: string[]
  mode?: ChatMode
  agent_enabled?: boolean
  agent_id?: string
  web_search_enabled?: boolean
  web_search_provider_id?: string
  enable_memory?: boolean
  summary_model_id?: string
  mcp_service_ids?: string[]
  mentioned_items?: MentionedItemPayload[]
  images?: ImageAttachmentPayload[]
  method: 'GET' | 'POST'
  url: string
}

export function deriveChatMode(input: {
  agentEnabled: boolean
  webSearchEnabled: boolean
  knowledgeBaseIds: string[]
  knowledgeIds: string[]
}): ChatMode {
  if (input.agentEnabled) {
    return 'agent'
  }
  if (input.webSearchEnabled) {
    return 'rag_deep'
  }
  if (input.knowledgeBaseIds.length > 0 || input.knowledgeIds.length > 0) {
    return 'rag_fast'
  }
  return 'chat'
}

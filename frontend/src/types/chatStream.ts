export type ChatResponseType =
  | 'agent_query'
  | 'session_title'
  | 'references'
  | 'thinking'
  | 'tool_call'
  | 'tool_result'
  | 'reflection'
  | 'answer'
  | 'complete'
  | 'stop'
  | 'error'

export interface ChatStreamChunk {
  id: string
  response_type: ChatResponseType
  content?: string
  done?: boolean
  session_id?: string
  assistant_message_id?: string
  knowledge_references?: any[]
  data?: Record<string, any>
  [key: string]: any
}

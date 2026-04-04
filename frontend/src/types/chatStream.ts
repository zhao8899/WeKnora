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

export type AgentStreamEventType = ChatResponseType | 'plan_task_change' | 'agent_complete'

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

export interface AgentStreamEvent {
  type: AgentStreamEventType
  event_id?: string
  tool_call_id?: string
  tool_name?: string
  content?: string
  done?: boolean
  thinking?: boolean
  pending?: boolean
  success?: boolean
  output?: string
  error?: string
  arguments?: any
  timestamp?: number
  duration?: number
  duration_ms?: number
  completed_at?: number
  tool_data?: Record<string, any>
  display_type?: string
  is_fallback?: boolean
  total_duration_ms?: number
  total_steps?: number
  task?: string
  reason?: string
  startTime?: number
  _mergedContent?: string
}

export interface ChatSessionData {
  isAgentMode?: boolean
  agentEventStream?: AgentStreamEvent[]
  knowledge_references?: any[]
  is_completed?: boolean
  content?: string
}

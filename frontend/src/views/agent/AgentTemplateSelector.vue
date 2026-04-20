<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="visible" class="template-overlay" @click.self="emit('cancel')">
        <div class="template-modal">
          <button class="close-btn" @click="emit('cancel')" :aria-label="t('common.close')">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
              <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            </svg>
          </button>

          <div class="modal-header">
            <h2 class="modal-title">{{ t('agent.template.title') }}</h2>
            <p class="modal-desc">{{ t('agent.template.desc') }}</p>
          </div>

          <div class="template-grid">
            <div
              v-for="tpl in templates"
              :key="tpl.id"
              class="template-card"
              :class="{ selected: selected === tpl.id }"
              @click="selected = tpl.id"
              @dblclick="handleSelect(tpl)"
            >
              <div class="card-icon">{{ tpl.icon }}</div>
              <div class="card-body">
                <div class="card-name">{{ locale === 'zh-CN' ? tpl.nameZh : tpl.nameEn }}</div>
                <div class="card-desc">{{ locale === 'zh-CN' ? tpl.descZh : tpl.descEn }}</div>
                <div class="card-tags">
                  <t-tag
                    v-for="tag in tpl.tags"
                    :key="tag"
                    size="small"
                    variant="light-outline"
                    :theme="tpl.id === 'blank' ? 'default' : 'primary'"
                  >{{ tag }}</t-tag>
                </div>
              </div>
            </div>
          </div>

          <div class="modal-footer">
            <t-button variant="outline" @click="emit('cancel')">{{ t('common.cancel') }}</t-button>
            <t-button theme="primary" :disabled="!selected" @click="confirmSelect">
              {{ t('agent.template.useTemplate') }}
            </t-button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CustomAgent, CustomAgentConfig } from '@/api/agent';

const { t, locale } = useI18n();

defineProps<{ visible: boolean }>();
const emit = defineEmits<{
  (e: 'cancel'): void;
  (e: 'select', template: Partial<CustomAgent> | null): void;
}>();

const selected = ref<string | null>(null);

interface AgentTemplate {
  id: string;
  icon: string;
  nameZh: string;
  nameEn: string;
  descZh: string;
  descEn: string;
  tags: string[];
  agent: Partial<CustomAgent> | null;
}

const templates = computed<AgentTemplate[]>(() => [
  {
    id: 'rag',
    icon: '📚',
    nameZh: '知识库问答',
    nameEn: 'KB Q&A',
    descZh: '基于知识库 RAG 检索快速回答用户问题，适合文档查询和知识检索场景。',
    descEn: 'RAG-based Q&A over your knowledge base. Ideal for document lookup and knowledge retrieval.',
    tags: locale.value === 'zh-CN' ? ['知识库', '快速问答'] : ['Knowledge Base', 'Quick Answer'],
    agent: {
      name: locale.value === 'zh-CN' ? '知识库问答助手' : 'KB Q&A Assistant',
      description: locale.value === 'zh-CN' ? '基于知识库的 RAG 问答助手' : 'RAG Q&A assistant over the knowledge base',
      avatar: '📚',
      config: {
        agent_mode: 'quick-answer',
        kb_selection_mode: 'all',
        web_search_enabled: true,
        web_search_max_results: 5,
        multi_turn_enabled: true,
        history_turns: 5,
        enable_query_expansion: true,
        enable_rewrite: true,
        faq_priority_enabled: true,
        fallback_strategy: 'model',
      } as Partial<CustomAgentConfig>,
    },
  },
  {
    id: 'deep-research',
    icon: '🔬',
    nameZh: '深度研究',
    nameEn: 'Deep Research',
    descZh: '多步骤推理，结合知识库与网络搜索，生成详尽的研究报告。',
    descEn: 'Multi-step reasoning with KB + web search to produce comprehensive research reports.',
    tags: locale.value === 'zh-CN' ? ['智能推理', '网络搜索', '知识库'] : ['ReAct', 'Web Search', 'KB'],
    agent: {
      name: locale.value === 'zh-CN' ? '深度研究助手' : 'Deep Research Assistant',
      description: locale.value === 'zh-CN' ? '多源深度研究，结合知识库与网络搜索' : 'Deep multi-source research combining KB and web search',
      avatar: '🔬',
      config: {
        agent_mode: 'smart-reasoning',
        kb_selection_mode: 'all',
        web_search_enabled: true,
        web_search_max_results: 10,
        multi_turn_enabled: true,
        history_turns: 5,
        reflection_enabled: true,
        max_iterations: 50,
        temperature: 0.5,
        allowed_tools: ['thinking', 'todo_write', 'knowledge_search', 'grep_chunks', 'list_knowledge_chunks', 'get_document_info', 'web_search', 'web_fetch'],
      } as Partial<CustomAgentConfig>,
    },
  },
  {
    id: 'data-analyst',
    icon: '📊',
    nameZh: '数据分析',
    nameEn: 'Data Analyst',
    descZh: '对 CSV/Excel 文件进行 SQL 查询和统计分析，数据洞察一步到位。',
    descEn: 'SQL queries and statistical analysis on CSV/Excel files for instant data insights.',
    tags: locale.value === 'zh-CN' ? ['数据分析', 'SQL', 'CSV/Excel'] : ['Data Analysis', 'SQL', 'CSV/Excel'],
    agent: {
      name: locale.value === 'zh-CN' ? '数据分析师' : 'Data Analyst',
      description: locale.value === 'zh-CN' ? '对 CSV/Excel 文件进行 SQL 查询和统计分析' : 'SQL and statistical analysis on CSV/Excel files',
      avatar: '📊',
      config: {
        agent_mode: 'smart-reasoning',
        kb_selection_mode: 'none',
        web_search_enabled: false,
        multi_turn_enabled: true,
        history_turns: 10,
        reflection_enabled: true,
        max_iterations: 30,
        temperature: 0.3,
        max_completion_tokens: 4096,
        supported_file_types: ['csv', 'xlsx'],
        allowed_tools: ['thinking', 'todo_write', 'data_schema', 'data_analysis'],
      } as Partial<CustomAgentConfig>,
    },
  },
  {
    id: 'doc-assistant',
    icon: '📄',
    nameZh: '文档助手',
    nameEn: 'Document Assistant',
    descZh: '精读、摘要、对比文档内容，帮助用户快速理解知识库中的文档。',
    descEn: 'Reads, summarizes, and compares documents from the knowledge base.',
    tags: locale.value === 'zh-CN' ? ['文档阅读', '摘要', '知识库'] : ['Doc Reading', 'Summary', 'KB'],
    agent: {
      name: locale.value === 'zh-CN' ? '文档助手' : 'Document Assistant',
      description: locale.value === 'zh-CN' ? '专注于文档阅读、摘要与问答' : 'Focused on document reading, summarization and Q&A',
      avatar: '📄',
      config: {
        agent_mode: 'smart-reasoning',
        kb_selection_mode: 'all',
        web_search_enabled: false,
        multi_turn_enabled: true,
        history_turns: 10,
        reflection_enabled: false,
        max_iterations: 20,
        temperature: 0.3,
        allowed_tools: ['thinking', 'todo_write', 'knowledge_search', 'grep_chunks', 'list_knowledge_chunks', 'get_document_info'],
      } as Partial<CustomAgentConfig>,
    },
  },
  {
    id: 'graph-expert',
    icon: '🕸️',
    nameZh: '知识图谱专家',
    nameEn: 'Knowledge Graph Expert',
    descZh: '探索实体关系与语义网络，适合分析知识图谱中的关联关系。',
    descEn: 'Explores entity relationships and semantic networks in the knowledge graph.',
    tags: locale.value === 'zh-CN' ? ['知识图谱', '实体关系', '知识库'] : ['Knowledge Graph', 'Entities', 'KB'],
    agent: {
      name: locale.value === 'zh-CN' ? '知识图谱专家' : 'Knowledge Graph Expert',
      description: locale.value === 'zh-CN' ? '专注于知识图谱中的实体关系探索与分析' : 'Entity relationship exploration and analysis in the knowledge graph',
      avatar: '🕸️',
      config: {
        agent_mode: 'smart-reasoning',
        kb_selection_mode: 'all',
        web_search_enabled: false,
        multi_turn_enabled: true,
        history_turns: 5,
        reflection_enabled: false,
        max_iterations: 30,
        temperature: 0.4,
        allowed_tools: ['thinking', 'todo_write', 'query_knowledge_graph', 'knowledge_search', 'grep_chunks', 'list_knowledge_chunks'],
      } as Partial<CustomAgentConfig>,
    },
  },
  {
    id: 'blank',
    icon: '✨',
    nameZh: '从空白创建',
    nameEn: 'Start from Scratch',
    descZh: '不使用任何模板，完全自定义配置你的智能体。',
    descEn: 'Start with a clean slate and configure everything from scratch.',
    tags: locale.value === 'zh-CN' ? ['自定义'] : ['Custom'],
    agent: null,
  },
]);

const handleSelect = (tpl: AgentTemplate) => {
  selected.value = tpl.id;
  emit('select', tpl.agent);
};

const confirmSelect = () => {
  const tpl = templates.value.find(t => t.id === selected.value);
  if (tpl) emit('select', tpl.agent);
};
</script>

<style scoped lang="less">
.template-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
}

.template-modal {
  background: var(--td-bg-color-container);
  border-radius: 12px;
  width: 760px;
  max-width: 94vw;
  max-height: 86vh;
  overflow-y: auto;
  padding: 32px;
  position: relative;
}

.close-btn {
  position: absolute;
  top: 16px;
  right: 16px;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--td-text-color-placeholder);
  padding: 4px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  &:hover { background: var(--td-bg-color-secondarycontainer); }
}

.modal-header {
  margin-bottom: 24px;
}

.modal-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0 0 6px;
}

.modal-desc {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin: 0;
}

.template-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-bottom: 24px;

  @media (max-width: 600px) {
    grid-template-columns: 1fr 1fr;
  }
}

.template-card {
  border: 1.5px solid var(--td-component-border);
  border-radius: 10px;
  padding: 16px;
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s, background 0.15s;
  display: flex;
  flex-direction: column;
  gap: 10px;

  &:hover {
    border-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }

  &.selected {
    border-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
    box-shadow: 0 0 0 2px rgba(0, 82, 217, 0.12);
  }
}

.card-icon {
  font-size: 28px;
  line-height: 1;
}

.card-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 4px;
}

.card-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  line-height: 1.5;
  margin-bottom: 8px;
}

.card-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
  .template-modal {
    transition: transform 0.2s ease;
  }
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
  .template-modal {
    transform: scale(0.96);
  }
}
</style>

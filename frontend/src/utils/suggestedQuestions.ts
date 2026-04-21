import type { SuggestedQuestion } from '@/api/agent';

const FALLBACK_QUESTIONS = [
  '如何本地部署 Qwen3-ASR + Ollama？',
  '如何提升会议录音识别准确率？',
  '长音频识别速度慢怎么优化？',
  'Ollama 导入模型报错怎么处理？',
  'Docker 和 Ollama 如何开机自启？',
  '识别结果截断怎么排查？',
];

export function normalizeSuggestedQuestions(
  questions: SuggestedQuestion[] | undefined,
  limit = 6,
): SuggestedQuestion[] {
  const seen = new Set<string>();
  const normalized = (questions || [])
    .map((item) => ({
      ...item,
      question: item.question?.trim() || '',
    }))
    .filter((item) => item.question)
    .filter((item) => {
      if (seen.has(item.question)) return false;
      seen.add(item.question);
      return true;
    })
    .slice(0, limit);

  if (normalized.length > 0) return normalized;

  return FALLBACK_QUESTIONS.slice(0, limit).map((question) => ({
    question,
    source: 'agent_config' as const,
  }));
}

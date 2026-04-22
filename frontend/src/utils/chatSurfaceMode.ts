import { BUILTIN_QUICK_ANSWER_ID, BUILTIN_SMART_REASONING_ID } from '@/api/agent';

export type ChatSurfaceMode = 'trusted-qa' | 'deep-research';

type SettingsLike = {
  isAgentEnabled?: boolean;
  selectedAgentId?: string;
  toggleAgent: (enabled: boolean) => void;
  selectAgent: (agentId: string, sourceTenantId?: string | null) => void;
  toggleWebSearch?: (enabled: boolean) => void;
  toggleMemory?: (enabled: boolean) => void;
};

export function resolveChatSurfaceMode(settingsStore: SettingsLike): ChatSurfaceMode {
  if (
    settingsStore.isAgentEnabled ||
    settingsStore.selectedAgentId === BUILTIN_SMART_REASONING_ID
  ) {
    return 'deep-research';
  }
  return 'trusted-qa';
}

export function applyChatSurfaceMode(settingsStore: SettingsLike, mode: ChatSurfaceMode) {
  if (mode === 'deep-research') {
    settingsStore.selectAgent(BUILTIN_SMART_REASONING_ID);
    settingsStore.toggleAgent(true);
    settingsStore.toggleWebSearch?.(true);
    settingsStore.toggleMemory?.(true);
    return;
  }

  settingsStore.selectAgent(BUILTIN_QUICK_ANSWER_ID);
  settingsStore.toggleAgent(false);
}

<template>
  <div class="section-content">
    <div class="panel-toolbar">
      <div class="toolbar-copy">
        <div class="toolbar-title">IM 运营面板</div>
        <div class="toolbar-subtitle">租户总览、当前智能体配置、渠道命令提示与连接状态一起看</div>
      </div>
      <div class="toolbar-controls">
        <t-input
          v-model="searchQuery"
          clearable
          size="small"
          placeholder="搜索名称 / 智能体 / 平台"
          class="toolbar-input"
        />
        <t-select v-model="platformFilter" size="small" clearable class="toolbar-select" placeholder="平台">
          <t-option value="all" label="全部平台" />
          <t-option value="wecom" :label="$t('agentEditor.im.wecom')" />
          <t-option value="feishu" :label="$t('agentEditor.im.feishu')" />
          <t-option value="slack" :label="$t('agentEditor.im.slack')" />
          <t-option value="telegram" :label="$t('agentEditor.im.telegram')" />
          <t-option value="dingtalk" :label="$t('agentEditor.im.dingtalk')" />
          <t-option value="mattermost" :label="$t('agentEditor.im.mattermost')" />
          <t-option value="wechat" :label="$t('agentEditor.im.wechat')" />
        </t-select>
        <t-select v-model="statusFilter" size="small" clearable class="toolbar-select" placeholder="状态">
          <t-option value="all" label="全部状态" />
          <t-option value="enabled" label="仅启用" />
          <t-option value="disabled" label="仅停用" />
        </t-select>
      </div>
    </div>

    <div class="command-hint">
      <div class="command-hint-header">
        <span class="command-hint-title">Slash 命令</span>
        <span class="command-hint-subtitle">IM 端已经支持这些运营常用命令</span>
      </div>
      <div class="command-tags">
        <span class="command-tag">/help</span>
        <span class="command-tag">/search 关键词</span>
        <span class="command-tag">/info</span>
        <span class="command-tag">/clear</span>
        <span class="command-tag">/stop</span>
      </div>
    </div>

    <!-- Tenant overview -->
    <div class="channels-section overview-section">
      <div class="channels-header">
        <span class="channels-title">租户级 IM Channels 概览</span>
        <span class="channels-count">{{ filteredOverviewChannels.length }}</span>
        <t-button
          variant="text"
          theme="default"
          size="small"
          class="refresh-btn"
          :loading="overviewLoading"
          @click="loadOverviewChannels"
        >
          <t-icon name="refresh" />
        </t-button>
      </div>

      <div v-if="overviewLoading && filteredOverviewChannels.length === 0" class="channels-loading">
        <t-loading size="small" />
        <span>{{ $t('common.loading') }}</span>
      </div>

      <div v-else-if="filteredOverviewChannels.length === 0" class="channels-empty">
        <t-icon name="chat-message" class="empty-icon" />
        <span>当前租户还没有 IM 渠道</span>
      </div>

      <div v-else class="channels-list overview-list">
        <div v-for="channel in filteredOverviewChannels" :key="channel.id" class="channel-item overview-item">
          <div class="channel-item-header">
            <div class="channel-info-top overview-top">
              <div class="channel-main">
                <span class="platform-badge" :class="channel.platform">
                  {{ platformLabel(channel.platform) }}
                </span>
                <span class="channel-name">{{ channel.name || $t('agentEditor.im.unnamed') }}</span>
              </div>
              <span class="source-tag">
                <t-icon name="user" class="meta-icon" />
                {{ channel.agent_name || channel.agent_id }}
              </span>
            </div>
            <div class="channel-actions">
              <t-button variant="text" theme="default" size="small" @click="togglePin(channel.id)">
                <t-icon :name="isPinned(channel.id) ? 'pin-filled' : 'pin'" />
              </t-button>
              <t-switch
                :value="channel.enabled"
                size="small"
                @change="handleToggle(channel)"
              />
              <t-button variant="text" theme="default" size="small" @click="editOverviewChannel(channel)">
                <t-icon name="edit" />
              </t-button>
              <t-popconfirm :content="$t('agentEditor.im.deleteConfirm')" @confirm="handleDelete(channel.id)">
                <t-button variant="text" theme="danger" size="small">
                  <t-icon name="delete" />
                </t-button>
              </t-popconfirm>
            </div>
          </div>
          <div class="channel-info">
            <div class="channel-meta">
              <span class="meta-tag">
                <t-icon name="link" class="meta-icon" />
                {{ modeLabel(channel.mode) }}
              </span>
              <span class="meta-tag">
                <t-icon name="play-circle" class="meta-icon" />
                {{ channel.output_mode === 'stream' ? $t('agentEditor.im.outputStream') : $t('agentEditor.im.outputFull') }}
              </span>
              <span v-if="channel.session_mode === 'thread'" class="meta-tag">
                <t-icon name="chat" class="meta-icon" />
                {{ $t('agentEditor.im.sessionModeThread') }}
              </span>
              <span class="meta-tag">
                <t-icon name="user" class="meta-icon" />
                {{ channel.agent_name || channel.agent_id }}
              </span>
            </div>
            <div v-if="channel.mode === 'webhook'" class="callback-url-row">
              <span class="url-label">{{ $t('agentEditor.im.callbackUrl') }}:</span>
              <code class="url-value">{{ getCallbackUrl(channel) }}</code>
              <t-button theme="default" size="small" variant="text" @click="copyUrl(channel)">
                <t-icon name="file-copy" />
              </t-button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Channel list header -->
    <div class="channels-section">
      <div class="channels-header">
        <span class="channels-title">{{ $t('agentEditor.im.addChannel') }}</span>
        <span class="channels-count">{{ filteredAgentChannels.length }}</span>
      </div>

      <div v-if="loading" class="channels-loading">
        <t-loading size="small" />
        <span>{{ $t('common.loading') }}</span>
      </div>

      <div v-else-if="filteredAgentChannels.length === 0" class="channels-empty">
        <t-icon name="chat-message" class="empty-icon" />
        <span>{{ $t('agentEditor.im.empty') }}</span>
      </div>

      <div v-else class="channels-list">
        <div v-for="channel in filteredAgentChannels" :key="channel.id" class="channel-item">
          <div class="channel-info">
            <div class="channel-info-top">
              <div class="channel-main">
                <span class="platform-badge" :class="channel.platform">
                  {{ platformLabel(channel.platform) }}
                </span>
                <span class="channel-name">{{ channel.name || $t('agentEditor.im.unnamed') }}</span>
              </div>
            </div>
            <div class="channel-meta">
              <span class="meta-tag">
                <t-icon name="user" class="meta-icon" />
                当前智能体
              </span>
              <span class="meta-tag">
                <t-icon name="link" class="meta-icon" />
                {{ modeLabel(channel.mode) }}
              </span>
              <span class="meta-tag">
                <t-icon name="play-circle" class="meta-icon" />
                {{ channel.output_mode === 'stream' ? $t('agentEditor.im.outputStream') : $t('agentEditor.im.outputFull') }}
              </span>
              <span v-if="channel.session_mode === 'thread'" class="meta-tag">
                <t-icon name="chat" class="meta-icon" />
                {{ $t('agentEditor.im.sessionModeThread') }}
              </span>
            </div>
            <div v-if="channel.mode === 'webhook'" class="callback-url-row">
              <span class="url-label">{{ $t('agentEditor.im.callbackUrl') }}:</span>
              <code class="url-value">{{ getCallbackUrl(channel) }}</code>
              <t-button theme="default" size="small" variant="text" @click="copyUrl(channel)">
                <t-icon name="file-copy" />
              </t-button>
            </div>
          </div>
          <div class="channel-actions">
            <t-button variant="text" theme="default" size="small" @click="togglePin(channel.id)">
              <t-icon :name="isPinned(channel.id) ? 'pin-filled' : 'pin'" />
            </t-button>
            <t-switch
              :value="channel.enabled"
              size="small"
              @change="handleToggle(channel)"
            />
            <t-button variant="text" theme="default" size="small" @click="editChannel(channel)">
              <t-icon name="edit" />
            </t-button>
            <t-popconfirm :content="$t('agentEditor.im.deleteConfirm')" @confirm="handleDelete(channel.id)">
              <t-button variant="text" theme="danger" size="small">
                <t-icon name="delete" />
              </t-button>
            </t-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <!-- Add button -->
    <t-button theme="default" variant="dashed" block @click="showCreateDialog = true" class="add-btn">
      <t-icon name="add" />
      {{ $t('agentEditor.im.addChannel') }}
    </t-button>

    <!-- Create/Edit dialog -->
    <t-dialog
      v-model:visible="showCreateDialog"
      :header="editingChannel ? $t('agentEditor.im.editChannel') : $t('agentEditor.im.addChannel')"
      :confirm-btn="$t('common.save')"
      :cancel-btn="$t('common.cancel')"
      @confirm="handleSave"
      @close="resetForm"
      width="560px"
    >
      <div class="dialog-form">
        <!-- Platform -->
        <div class="form-item">
          <label class="form-label">{{ $t('agentEditor.im.platform') }}</label>
          <t-radio-group v-model="formData.platform" :disabled="!!editingChannel">
            <t-radio-button value="wecom">{{ $t('agentEditor.im.wecom') }}</t-radio-button>
            <t-radio-button value="feishu">{{ $t('agentEditor.im.feishu') }}</t-radio-button>
            <t-radio-button value="slack">{{ $t('agentEditor.im.slack') }}</t-radio-button>
            <t-radio-button value="telegram">{{ $t('agentEditor.im.telegram') }}</t-radio-button>
            <t-radio-button value="dingtalk">{{ $t('agentEditor.im.dingtalk') }}</t-radio-button>
            <t-radio-button value="mattermost">{{ $t('agentEditor.im.mattermost') }}</t-radio-button>
          </t-radio-group>
        </div>

        <!-- Name -->
        <div class="form-item">
          <label class="form-label">{{ $t('agentEditor.im.channelName') }}</label>
          <t-input v-model="formData.name" :placeholder="$t('agentEditor.im.channelNamePlaceholder')" />
        </div>

        <!-- Mode -->
        <div class="form-item">
          <label class="form-label">{{ $t('agentEditor.im.mode') }}</label>
          <t-radio-group v-model="formData.mode">
            <t-radio-button value="websocket" :disabled="formData.platform === 'mattermost'">WebSocket</t-radio-button>
            <t-radio-button value="webhook">Webhook</t-radio-button>
          </t-radio-group>
          <p v-if="formData.platform === 'mattermost'" class="form-hint">{{ $t('agentEditor.im.mattermostModeHint') }}</p>
          <p v-else class="form-hint">{{ $t('agentEditor.im.modeHint') }}</p>
        </div>

        <!-- Output mode -->
        <div class="form-item">
          <label class="form-label">{{ $t('agentEditor.im.outputMode') }}</label>
          <t-radio-group v-model="formData.output_mode">
            <t-radio-button value="stream">{{ $t('agentEditor.im.outputStream') }}</t-radio-button>
            <t-radio-button value="full">{{ $t('agentEditor.im.outputFull') }}</t-radio-button>
          </t-radio-group>
        </div>

        <!-- Session Mode -->
        <div class="form-item">
          <label class="form-label">{{ $t('agentEditor.im.sessionMode') }}</label>
          <t-radio-group v-model="formData.session_mode">
            <t-radio-button value="user">{{ $t('agentEditor.im.sessionModeUser') }}</t-radio-button>
            <t-radio-button value="thread"
              :disabled="!platformSupportsThread(formData.platform)">
              {{ $t('agentEditor.im.sessionModeThread') }}
            </t-radio-button>
          </t-radio-group>
          <p class="form-hint">{{ $t('agentEditor.im.sessionModeHint') }}</p>
        </div>

        <!-- Knowledge base for file messages -->
        <div class="form-item">
          <label class="form-label">{{ $t('agentEditor.im.fileKnowledgeBase') }}</label>
          <t-select
            v-model="formData.knowledge_base_id"
            :placeholder="$t('agentEditor.im.fileKnowledgeBasePlaceholder')"
            clearable
            filterable
          >
            <t-option v-for="kb in knowledgeBases" :key="kb.id" :value="kb.id" :label="kb.name" />
          </t-select>
          <p class="form-hint">{{ $t('agentEditor.im.fileKnowledgeBaseHint') }}</p>
        </div>

        <!-- Credentials divider -->
        <div class="form-divider"></div>

        <!-- WeCom credentials -->
        <template v-if="formData.platform === 'wecom'">
          <div class="platform-link-hint">
            <t-icon name="jump" class="hint-link-icon" />
            <a href="https://work.weixin.qq.com/" target="_blank" rel="noopener noreferrer" class="hint-link">
              {{ $t('agentEditor.im.wecomConsole') }}
            </a>
            <span class="hint-text">{{ $t('agentEditor.im.consoleTip') }}</span>
          </div>
          <template v-if="formData.mode === 'websocket'">
            <div class="form-item">
              <label class="form-label">Bot ID</label>
              <t-input v-model="formData.credentials.bot_id" placeholder="Bot ID" />
            </div>
            <div class="form-item">
              <label class="form-label">Bot Secret</label>
              <t-input v-model="formData.credentials.bot_secret" type="password" placeholder="Bot Secret" />
            </div>
          </template>
          <template v-else>
            <div class="form-item">
              <label class="form-label">Corp ID</label>
              <t-input v-model="formData.credentials.corp_id" placeholder="Corp ID" />
            </div>
            <div class="form-item">
              <label class="form-label">Agent Secret</label>
              <t-input v-model="formData.credentials.agent_secret" type="password" placeholder="Agent Secret" />
            </div>
            <div class="form-item">
              <label class="form-label">Token</label>
              <t-input v-model="formData.credentials.token" placeholder="Token" />
            </div>
            <div class="form-item">
              <label class="form-label">EncodingAESKey</label>
              <t-input v-model="formData.credentials.encoding_aes_key" placeholder="EncodingAESKey" />
            </div>
            <div class="form-item">
              <label class="form-label">Corp Agent ID</label>
              <t-input-number v-model="formData.credentials.corp_agent_id" placeholder="Corp Agent ID" style="width: 100%;" />
            </div>
          </template>
        </template>

        <!-- Feishu credentials -->
        <template v-if="formData.platform === 'feishu'">
          <div class="platform-link-hint">
            <t-icon name="jump" class="hint-link-icon" />
            <a href="https://open.feishu.cn/" target="_blank" rel="noopener noreferrer" class="hint-link">
              {{ $t('agentEditor.im.feishuConsole') }}
            </a>
            <span class="hint-text">{{ $t('agentEditor.im.consoleTip') }}</span>
          </div>
          <div class="form-item">
            <label class="form-label">App ID</label>
            <t-input v-model="formData.credentials.app_id" placeholder="App ID" />
          </div>
          <div class="form-item">
            <label class="form-label">App Secret</label>
            <t-input v-model="formData.credentials.app_secret" type="password" placeholder="App Secret" />
          </div>
          <template v-if="formData.mode === 'webhook'">
            <div class="form-item">
              <label class="form-label">Verification Token</label>
              <t-input v-model="formData.credentials.verification_token" placeholder="Verification Token" />
            </div>
            <div class="form-item">
              <label class="form-label">Encrypt Key</label>
              <t-input v-model="formData.credentials.encrypt_key" type="password" placeholder="Encrypt Key" />
            </div>
          </template>
        </template>

        <!-- Slack credentials -->
        <template v-if="formData.platform === 'slack'">
          <div class="platform-link-hint">
            <t-icon name="jump" class="hint-link-icon" />
            <a href="https://api.slack.com/apps" target="_blank" rel="noopener noreferrer" class="hint-link">
              {{ $t('agentEditor.im.slackConsole') }}
            </a>
            <span class="hint-text">{{ $t('agentEditor.im.consoleTip') }}</span>
          </div>
          <template v-if="formData.mode === 'websocket'">
            <div class="form-item">
              <label class="form-label">App Token</label>
              <t-input v-model="formData.credentials.app_token" type="password" placeholder="xapp-..." />
            </div>
            <div class="form-item">
              <label class="form-label">Bot Token</label>
              <t-input v-model="formData.credentials.bot_token" type="password" placeholder="xoxb-..." />
            </div>
          </template>
          <template v-else>
            <div class="form-item">
              <label class="form-label">Bot Token</label>
              <t-input v-model="formData.credentials.bot_token" type="password" placeholder="xoxb-..." />
            </div>
            <div class="form-item">
              <label class="form-label">Signing Secret</label>
              <t-input v-model="formData.credentials.signing_secret" type="password" placeholder="Signing Secret" />
            </div>
          </template>
        </template>

        <!-- Telegram credentials -->
        <template v-if="formData.platform === 'telegram'">
          <div class="platform-link-hint">
            <t-icon name="jump" class="hint-link-icon" />
            <a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer" class="hint-link">
              {{ $t('agentEditor.im.telegramConsole') }}
            </a>
            <span class="hint-text">{{ $t('agentEditor.im.consoleTip') }}</span>
          </div>
          <div class="form-item">
            <label class="form-label">Bot Token</label>
            <t-input v-model="formData.credentials.bot_token" type="password" placeholder="123456789:AABBccdd..." />
          </div>
          <template v-if="formData.mode === 'webhook'">
            <div class="form-item">
              <label class="form-label">Secret Token</label>
              <t-input v-model="formData.credentials.secret_token" type="password" placeholder="Secret Token (optional)" />
            </div>
          </template>
        </template>

        <!-- DingTalk credentials -->
        <template v-if="formData.platform === 'dingtalk'">
          <div class="platform-link-hint">
            <t-icon name="jump" class="hint-link-icon" />
            <a href="https://open.dingtalk.com/" target="_blank" rel="noopener noreferrer" class="hint-link">
              {{ $t('agentEditor.im.dingtalkConsole') }}
            </a>
            <span class="hint-text">{{ $t('agentEditor.im.consoleTip') }}</span>
          </div>
          <div class="form-item">
            <label class="form-label">Client ID (AppKey)</label>
            <t-input v-model="formData.credentials.client_id" placeholder="Client ID / AppKey" />
          </div>
          <div class="form-item">
            <label class="form-label">Client Secret (AppSecret)</label>
            <t-input v-model="formData.credentials.client_secret" type="password" placeholder="Client Secret / AppSecret" />
          </div>
          <div class="form-item">
            <label class="form-label">{{ $t('agentEditor.im.dingtalkCardTemplateId') }}</label>
            <t-input v-model="formData.credentials.card_template_id" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx.schema" />
            <p class="form-hint">{{ $t('agentEditor.im.dingtalkCardTemplateIdHint') }}</p>
          </div>
        </template>

        <!-- Mattermost credentials -->
        <template v-if="formData.platform === 'mattermost'">
          <div class="platform-link-hint">
            <t-icon name="jump" class="hint-link-icon" />
            <a href="https://developers.mattermost.com/integrate/webhooks/outgoing/" target="_blank" rel="noopener noreferrer" class="hint-link">
              {{ $t('agentEditor.im.mattermostConsole') }}
            </a>
            <span class="hint-text">{{ $t('agentEditor.im.consoleTip') }}</span>
          </div>
          <div class="form-item">
            <label class="form-label">Site URL</label>
            <t-input v-model="formData.credentials.site_url" placeholder="https://mattermost.example.com" />
          </div>
          <div class="form-item">
            <label class="form-label">Bot Token</label>
            <t-input v-model="formData.credentials.bot_token" type="password" placeholder="Bot Token" />
          </div>
          <div class="form-item">
            <label class="form-label">Outgoing Webhook Token</label>
            <t-input v-model="formData.credentials.outgoing_token" type="password" placeholder="Token from Outgoing Webhook" />
          </div>
          <div class="form-item">
            <label class="form-label">Bot User ID</label>
            <t-input v-model="formData.credentials.bot_user_id" placeholder="Optional — filter bot self-messages" />
          </div>
          <div class="form-item mattermost-post-main-row">
            <div class="mattermost-post-main-label">
              <label class="form-label">{{ $t('agentEditor.im.mattermostPostToMain') }}</label>
              <t-switch
                :value="!!formData.credentials.post_to_main"
                @change="(v: boolean) => { formData.credentials.post_to_main = v }"
              />
            </div>
            <p class="form-hint">{{ $t('agentEditor.im.mattermostPostToMainHint') }}</p>
          </div>
        </template>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next/es/message';
import { listIMChannels, listAgents, createIMChannel, updateIMChannel, deleteIMChannel, toggleIMChannel } from '@/api/agent';
import { listKnowledgeBases } from '@/api/knowledge-base';
import type { IMChannel } from '@/api/agent';

const { t } = useI18n();

const props = defineProps<{
  agentId: string;
}>();

const channels = ref<IMChannel[]>([]);
const overviewChannels = ref<IMChannelOverview[]>([]);
const loading = ref(false);
const overviewLoading = ref(false);
const showCreateDialog = ref(false);
const editingChannel = ref<IMChannel | null>(null);
const searchQuery = ref('');
const platformFilter = ref<'all' | IMChannel['platform'] | 'wechat'>('all');
const statusFilter = ref<'all' | 'enabled' | 'disabled'>('all');
const pinnedChannelIds = ref<string[]>(loadPinnedChannelIds());

// Knowledge base options for file-to-KB feature
const knowledgeBases = ref<{ id: string; name: string }[]>([]);

type IMChannelMode = 'webhook' | 'websocket' | 'longpoll';

interface IMChannelOverview {
  id: string;
  tenant_id: number;
  agent_id: string;
  agent_name?: string;
  platform: IMChannel['platform'] | 'wechat';
  name: string;
  enabled: boolean;
  mode: IMChannelMode;
  output_mode: 'stream' | 'full';
  session_mode?: 'user' | 'thread';
  bot_identity?: string;
  created_at?: string;
  updated_at?: string;
}

const defaultCredentials = (): Record<string, any> => ({});

const formData = ref({
  platform: 'wecom' as 'wecom' | 'feishu' | 'slack' | 'telegram' | 'dingtalk' | 'mattermost',
  name: '',
  mode: 'websocket' as 'webhook' | 'websocket',
  output_mode: 'stream' as 'stream' | 'full',
  session_mode: 'user' as 'user' | 'thread',
  knowledge_base_id: '',
  credentials: defaultCredentials(),
});

function platformLabel(platform: string): string {
  const key = `agentEditor.im.${platform}`;
  return t(key);
}

function platformSupportsThread(platform: string): boolean {
  return ['slack', 'mattermost', 'feishu', 'telegram'].includes(platform);
}

function loadPinnedChannelIds(): string[] {
  try {
    const raw = localStorage.getItem('weknora-im-channel-pins');
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((v) => typeof v === 'string') : [];
  } catch {
    return [];
  }
}

function savePinnedChannelIds() {
  localStorage.setItem('weknora-im-channel-pins', JSON.stringify(pinnedChannelIds.value));
}

function isPinned(id: string): boolean {
  return pinnedChannelIds.value.includes(id);
}

function togglePin(id: string) {
  const next = new Set(pinnedChannelIds.value);
  if (next.has(id)) {
    next.delete(id);
    MessagePlugin.success('已取消置顶');
  } else {
    next.add(id);
    MessagePlugin.success('已置顶');
  }
  pinnedChannelIds.value = Array.from(next);
  savePinnedChannelIds();
}

function modeLabel(mode: string): string {
  if (mode === 'longpoll') return 'LongPoll';
  if (mode === 'websocket') return 'WebSocket';
  if (mode === 'webhook') return 'Webhook';
  return mode || '-';
}

function sortChannels<T extends { id: string; enabled: boolean; updated_at?: string; created_at?: string }>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const pinDelta = Number(isPinned(b.id)) - Number(isPinned(a.id));
    if (pinDelta !== 0) return pinDelta;
    const statusDelta = Number(b.enabled) - Number(a.enabled);
    if (statusDelta !== 0) return statusDelta;
    const aTime = new Date(a.updated_at || a.created_at || 0).getTime();
    const bTime = new Date(b.updated_at || b.created_at || 0).getTime();
    return bTime - aTime;
  });
}

function channelSearchText(channel: {
  name?: string;
  platform?: string;
  mode?: string;
  output_mode?: string;
  session_mode?: string;
  agent_name?: string;
  bot_identity?: string;
  agent_id?: string;
}): string {
  return [
    channel.name,
    channel.platform,
    channel.mode,
    channel.output_mode,
    channel.session_mode,
    channel.agent_name,
    channel.bot_identity,
    channel.agent_id,
  ].filter(Boolean).join(' ').toLowerCase();
}

function matchesFilters(channel: {
  name?: string;
  platform?: string;
  enabled?: boolean;
  mode?: string;
  output_mode?: string;
  session_mode?: string;
  agent_name?: string;
  bot_identity?: string;
  agent_id?: string;
}) {
  const q = searchQuery.value.trim().toLowerCase();
  if (q && !channelSearchText(channel).includes(q)) {
    return false;
  }
  if (platformFilter.value !== 'all' && channel.platform !== platformFilter.value) {
    return false;
  }
  if (statusFilter.value === 'enabled' && !channel.enabled) {
    return false;
  }
  if (statusFilter.value === 'disabled' && channel.enabled) {
    return false;
  }
  return true;
}

const filteredOverviewChannels = computed(() => sortChannels(overviewChannels.value.filter(matchesFilters)));
const filteredAgentChannels = computed(() => sortChannels(channels.value.filter(matchesFilters)));

async function loadOverviewChannels() {
  overviewLoading.value = true;
  try {
    const agentRes = await listAgents();
    const agentSummaries = agentRes.data || [];
    const settled = await Promise.allSettled(agentSummaries.map((agent) => listIMChannels(agent.id)));
    const items: IMChannelOverview[] = [];

    settled.forEach((result, index) => {
      const agent = agentSummaries[index];
      if (result.status !== 'fulfilled') return;
      const rows = result.value.data || [];
      rows.forEach((row: IMChannel) => {
        items.push({
          id: row.id,
          tenant_id: agent?.tenant_id || 0,
          agent_id: row.agent_id || agent.id,
          agent_name: agent?.name || row.agent_id,
          platform: row.platform as IMChannelOverview['platform'],
          name: row.name,
          enabled: row.enabled,
          mode: (row.mode as IMChannelMode) || 'websocket',
          output_mode: row.output_mode,
          session_mode: row.session_mode,
          bot_identity: '',
          created_at: row.created_at,
          updated_at: row.updated_at,
        });
      });
    });

    overviewChannels.value = items;
  } catch {
    overviewChannels.value = [];
  } finally {
    overviewLoading.value = false;
  }
}

watch(
  () => formData.value.platform,
  (p) => {
    if (p === 'mattermost') {
      formData.value.mode = 'webhook';
      if (typeof formData.value.credentials.post_to_main !== 'boolean') {
        formData.value.credentials.post_to_main = false;
      }
    }
    if (!platformSupportsThread(p)) {
      formData.value.session_mode = 'user';
    }
  },
);

async function loadChannels() {
  loading.value = true;
  try {
    const [channelRes, kbRes] = await Promise.all([
      listIMChannels(props.agentId),
      listKnowledgeBases(),
    ]);
    channels.value = channelRes.data || [];
    knowledgeBases.value = (kbRes.data || []).map((kb: any) => ({ id: kb.id, name: kb.name }));
  } catch {
    channels.value = [];
  } finally {
    loading.value = false;
  }
  await loadOverviewChannels();
}

function getCallbackUrl(channel: { id: string }): string {
  const base = import.meta.env.VITE_IS_DOCKER ? window.location.origin : 'http://127.0.0.1:8080';
  return `${base}/api/v1/im/callback/${channel.id}`;
}

async function copyUrl(channel: { id: string }) {
  const text = getCallbackUrl(channel);
  try {
    await navigator.clipboard.writeText(text);
    MessagePlugin.success(t('common.copySuccess'));
  } catch {
    const el = document.createElement('textarea');
    el.value = text;
    el.style.cssText = 'position:fixed;top:-9999px;left:-9999px;opacity:0';
    document.body.appendChild(el);
    el.focus();
    el.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(el);
    if (ok) {
      MessagePlugin.success(t('common.copySuccess'));
    } else {
      MessagePlugin.error(t('common.copyFailed'));
    }
  }
}

function editChannel(channel: IMChannel) {
  editingChannel.value = channel;
  formData.value = {
    platform: channel.platform,
    name: channel.name,
    mode: channel.mode,
    output_mode: channel.output_mode,
    session_mode: channel.session_mode || 'user',
    knowledge_base_id: channel.knowledge_base_id || '',
    credentials: { ...channel.credentials },
  };
  showCreateDialog.value = true;
}

async function editOverviewChannel(channel: IMChannelOverview) {
  try {
    const res = await listIMChannels(channel.agent_id);
    const full = (res.data || []).find((item) => item.id === channel.id);
    if (!full) {
      MessagePlugin.error('未找到该渠道详情');
      return;
    }
    editChannel(full);
  } catch {
    MessagePlugin.error(t('common.operationFailed'));
  }
}

function resetForm() {
  editingChannel.value = null;
  formData.value = {
    platform: 'wecom',
    name: '',
    mode: 'websocket',
    output_mode: 'stream',
    session_mode: 'user',
    knowledge_base_id: '',
    credentials: defaultCredentials(),
  };
}

async function handleSave() {
  try {
    if (editingChannel.value) {
      await updateIMChannel(editingChannel.value.id, {
        name: formData.value.name,
        mode: formData.value.mode,
        output_mode: formData.value.output_mode,
        session_mode: formData.value.session_mode,
        knowledge_base_id: formData.value.knowledge_base_id,
        credentials: formData.value.credentials,
      });
      MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createIMChannel(props.agentId, {
        platform: formData.value.platform,
        name: formData.value.name,
        mode: formData.value.mode,
        output_mode: formData.value.output_mode,
        session_mode: formData.value.session_mode,
        knowledge_base_id: formData.value.knowledge_base_id,
        credentials: formData.value.credentials,
      });
      MessagePlugin.success(t('common.createSuccess'));
    }
    showCreateDialog.value = false;
    resetForm();
    await loadChannels();
  } catch (e: any) {
    const msg = e?.message || (typeof e?.error === 'string' ? e.error : null) || t('common.operationFailed');
    MessagePlugin.error(msg);
  }
}

async function handleToggle(channel: { id: string }) {
  try {
    await toggleIMChannel(channel.id);
    await loadChannels();
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('common.operationFailed'));
  }
}

async function handleDelete(id: string) {
  try {
    await deleteIMChannel(id);
    MessagePlugin.success(t('common.deleteSuccess'));
    await loadChannels();
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('common.operationFailed'));
  }
}

onMounted(() => {
  loadChannels();
});
</script>

<style scoped lang="less">
.section-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.panel-toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 4px 0 2px;
}

.toolbar-copy {
  min-width: 0;
}

.toolbar-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}

.toolbar-subtitle {
  margin-top: 4px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  line-height: 1.5;
}

.toolbar-controls {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.toolbar-input {
  width: 220px;
}

.toolbar-select {
  width: 130px;
}

.command-hint {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid var(--td-component-stroke);
}

.command-hint-header {
  display: flex;
  align-items: center;
  gap: 8px;
  justify-content: space-between;
}

.command-hint-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.command-hint-subtitle {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.command-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.command-tag {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  color: var(--td-text-color-primary);
  font-size: 12px;
  line-height: 18px;
}

// --- Channel list section (matches AgentShareSettings pattern) ---
.channels-section {
  margin-bottom: 8px;
}

.channels-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;

  .channels-title {
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }

  .channels-count {
    padding: 2px 8px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 10px;
    font-size: 12px;
    color: var(--td-text-color-disabled);
  }
}

.channels-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px;
  color: var(--td-text-color-disabled);
  font-size: 14px;
}

.channels-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 40px 20px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 8px;
  color: var(--td-text-color-disabled);

  .empty-icon {
    font-size: 32px;
    opacity: 0.5;
  }
}

.channels-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 400px;
  overflow-y: auto;
}

.channel-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  transition: background 0.2s ease, border-color 0.2s ease;

  &:hover {
    border-color: var(--td-brand-color-focus);
  }
}

.overview-item {
  flex-direction: column;
  align-items: stretch;
}

.channel-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.channel-info-top {
  display: flex;
  align-items: center;
  gap: 12px;
}

.overview-top {
  justify-content: space-between;
  align-items: flex-start;
}

.channel-main {
  display: flex;
  align-items: center;
  gap: 8px;
}

.source-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(7, 193, 96, 0.08);
  color: var(--td-brand-color);
  font-size: 12px;
  line-height: 18px;
  white-space: nowrap;
}

.platform-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  line-height: 18px;

  &.wecom {
    background: rgba(7, 193, 96, 0.08);
    color: #07c160;
  }

  &.feishu {
    background: rgba(51, 112, 255, 0.08);
    color: #3370ff;
  }

  &.slack {
    background: rgba(224, 30, 90, 0.08);
    color: #e01e5a;
  }

  &.telegram {
    background: rgba(38, 166, 219, 0.08);
    color: #26a6db;
  }

  &.dingtalk {
    background: rgba(23, 126, 251, 0.08);
    color: #177efb;
  }

  &.mattermost {
    background: rgba(25, 42, 77, 0.08);
    color: #192a4d;
  }

  &.wechat {
    background: rgba(7, 193, 96, 0.08);
    color: #07c160;
  }
}

.channel-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.channel-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);

  .meta-tag {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 2px 6px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 4px;
  }

  .meta-icon {
    font-size: 12px;
    flex-shrink: 0;
  }
}

.callback-url-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  padding-top: 4px;
  border-top: 1px dashed var(--td-component-stroke);

  .url-label {
    color: var(--td-text-color-secondary);
    white-space: nowrap;
  }

  .url-value {
    background: var(--td-bg-color-container);
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
    min-width: 0;
  }
}

.channel-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.overview-section {
  .refresh-btn {
    margin-left: auto;
  }
}

.add-btn {
  margin-top: 4px;

  :deep(.t-button__text) {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
}

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-item {
  .form-label {
    display: block;
    margin-bottom: 8px;
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }
}

.mattermost-post-main-row {
  .mattermost-post-main-label {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;

    .form-label {
      margin-bottom: 0;
      flex: 1;
    }
  }
}

.form-divider {
  height: 1px;
  background: var(--td-component-stroke);
  margin: 4px 0;
}

.form-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  line-height: 1.4;
}

.platform-link-hint {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--td-text-color-placeholder);

  .hint-link-icon {
    font-size: 12px;
    color: var(--td-brand-color);
    flex-shrink: 0;
  }

  .hint-link {
    color: var(--td-brand-color);
    text-decoration: none;
    font-weight: 500;
    white-space: nowrap;

    &:hover {
      text-decoration: underline;
    }
  }

  .hint-text {
    color: var(--td-text-color-placeholder);
  }
}
</style>

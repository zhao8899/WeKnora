<template>
  <div class="langfuse-settings">
    <div class="section-header">
      <h2>Langfuse 可观测性</h2>
      <p>通过环境变量接入 Langfuse，用于追踪聊天、Agent 和异步任务。此页只读当前状态并提供连通性测试，不会保存密钥。</p>
    </div>

    <t-loading :loading="loading">
      <div class="status-grid">
        <div class="status-item">
          <span class="label">启用状态</span>
          <t-tag :theme="status?.enabled ? 'success' : 'default'" variant="light">
            {{ status?.enabled ? '已启用' : '未启用' }}
          </t-tag>
        </div>
        <div class="status-item">
          <span class="label">配置状态</span>
          <t-tag :theme="status?.configured ? 'success' : 'warning'" variant="light">
            {{ status?.configured ? '已配置密钥' : '未配置密钥' }}
          </t-tag>
        </div>
        <div class="status-item wide">
          <span class="label">Host</span>
          <span class="value">{{ status?.host || '-' }}</span>
        </div>
        <div class="status-item">
          <span class="label">Public Key</span>
          <span class="value">{{ status?.public_key_masked || '-' }}</span>
        </div>
        <div class="status-item">
          <span class="label">环境</span>
          <span class="value">{{ status?.environment || '-' }}</span>
        </div>
        <div class="status-item">
          <span class="label">版本</span>
          <span class="value">{{ status?.release || '-' }}</span>
        </div>
        <div class="status-item">
          <span class="label">采样率</span>
          <span class="value">{{ status?.sample_rate ?? '-' }}</span>
        </div>
        <div class="status-item">
          <span class="label">队列大小</span>
          <span class="value">{{ status?.queue_size ?? '-' }}</span>
        </div>
        <div class="status-item">
          <span class="label">请求超时</span>
          <span class="value">{{ status?.request_timeout || '-' }}</span>
        </div>
        <div class="status-item">
          <span class="label">调试模式</span>
          <span class="value">{{ status?.debug ? '开启' : '关闭' }}</span>
        </div>
      </div>
    </t-loading>

    <div class="settings-group">
      <div class="group-header">
        <h3>连通性测试</h3>
        <t-button variant="outline" :loading="loading" @click="loadStatus">刷新状态</t-button>
      </div>
      <t-form :data="testForm" label-align="top" @submit="runCheck">
        <t-form-item label="Host" name="host" :rules="[{ required: true, message: '请输入 Langfuse Host' }]">
          <t-input v-model="testForm.host" placeholder="https://cloud.langfuse.com" />
        </t-form-item>
        <t-form-item label="Public Key" name="public_key" :rules="[{ required: true, message: '请输入 Public Key' }]">
          <t-input v-model="testForm.public_key" placeholder="pk-lf-..." />
        </t-form-item>
        <t-form-item label="Secret Key" name="secret_key" :rules="[{ required: true, message: '请输入 Secret Key' }]">
          <t-input v-model="testForm.secret_key" type="password" placeholder="sk-lf-..." />
        </t-form-item>
        <t-button theme="primary" type="submit" :loading="checking">测试连接</t-button>
      </t-form>
    </div>

    <div class="settings-group">
      <h3>环境变量</h3>
      <div class="env-list">
        <code>LANGFUSE_ENABLED=true</code>
        <code>LANGFUSE_HOST=https://cloud.langfuse.com</code>
        <code>LANGFUSE_PUBLIC_KEY=pk-lf-...</code>
        <code>LANGFUSE_SECRET_KEY=sk-lf-...</code>
        <code>LANGFUSE_ENVIRONMENT=production</code>
        <code>LANGFUSE_RELEASE=v0.5.1</code>
      </div>
      <p class="hint">
        修改环境变量后需要重启后端服务。自建 Langfuse 时建议把 Host 指向内网地址，并通过后端 SSRF 白名单控制允许访问的域名。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { checkLangfuseConfig, getLangfuseStatus, type LangfuseStatus } from '@/api/system'

const loading = ref(false)
const checking = ref(false)
const status = ref<LangfuseStatus | null>(null)
const testForm = reactive({
  host: 'https://cloud.langfuse.com',
  public_key: '',
  secret_key: '',
})

const loadStatus = async () => {
  loading.value = true
  try {
    const result = await getLangfuseStatus()
    status.value = result.data
    if (status.value?.host) {
      testForm.host = status.value.host
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || '加载 Langfuse 状态失败')
  } finally {
    loading.value = false
  }
}

const runCheck = async ({ validateResult, firstError }: any) => {
  if (validateResult !== true && validateResult !== undefined) {
    MessagePlugin.warning(firstError || '请检查表单')
    return
  }

  checking.value = true
  try {
    const result = await checkLangfuseConfig({ ...testForm })
    if (result.data?.ok) {
      MessagePlugin.success(result.data.message || '连接成功')
      await loadStatus()
    } else {
      MessagePlugin.error(result.data?.message || '连接失败')
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || '连接失败')
  } finally {
    checking.value = false
  }
}

onMounted(loadStatus)
</script>

<style scoped lang="less">
.langfuse-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 24px;

  h2 {
    margin: 0 0 8px;
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  p {
    margin: 0;
    color: var(--td-text-color-secondary);
    line-height: 1.5;
  }
}

.status-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 24px;
}

.status-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;

  &.wide {
    grid-column: 1 / -1;
  }
}

.label {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.value {
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family-mono);
  font-size: 13px;
  word-break: break-all;
  text-align: right;
}

.settings-group {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid var(--td-component-stroke);

  h3 {
    margin: 0 0 16px;
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;

  h3 {
    margin: 0;
  }
}

.env-list {
  display: grid;
  gap: 8px;

  code {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-primary);
    font-size: 13px;
  }
}

.hint {
  margin: 12px 0 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
}
</style>

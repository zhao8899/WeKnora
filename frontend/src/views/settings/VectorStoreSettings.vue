<template>
  <div class="vector-store-settings">
    <div class="page-head">
      <div>
        <h2>向量库管理</h2>
        <p>统一管理租户可用的向量库连接、索引配置和知识库绑定关系。</p>
      </div>
      <div class="page-actions">
        <t-input
          v-model="searchText"
          clearable
          placeholder="搜索名称、引擎或地址"
          class="search-input"
        />
        <t-button theme="primary" @click="openCreate">
          <template #icon><AddIcon /></template>
          新增向量库
        </t-button>
        <t-button variant="outline" :loading="loading" @click="loadStores">刷新</t-button>
      </div>
    </div>

    <t-loading :loading="loading">
      <div class="workspace">
        <aside class="store-panel">
          <div class="panel-title">向量库列表</div>
          <div v-if="filteredStores.length" class="store-list">
            <button
              v-for="store in filteredStores"
              :key="store.id"
              type="button"
              class="store-item"
              :class="{ active: selectedStoreId === store.id }"
              @click="selectStore(store)"
            >
              <div class="store-item-head">
                <div class="store-name">{{ store.name }}</div>
                <div class="store-tags">
                  <t-tag size="small" variant="outline">{{ store.engine_type }}</t-tag>
                  <t-tag v-if="store.source === 'env'" size="small" theme="warning" variant="light">环境</t-tag>
                </div>
              </div>
              <div class="store-item-meta">
                <span>{{ formatEndpoint(store) || '未配置地址' }}</span>
                <span>{{ store.knowledge_base_count || 0 }} 个知识库</span>
              </div>
            </button>
          </div>
          <t-empty v-else description="暂无向量库配置" />
        </aside>

        <section class="detail-panel">
          <template v-if="selectedStore">
            <div class="detail-head">
              <div>
                <div class="detail-title-row">
                  <h3>{{ selectedStore.name }}</h3>
                  <t-tag size="small" variant="outline">{{ selectedStore.engine_type }}</t-tag>
                  <t-tag v-if="selectedStore.source === 'env'" size="small" theme="warning" variant="light">环境</t-tag>
                  <t-tag v-if="selectedStore.readonly" size="small" theme="default" variant="light">只读</t-tag>
                </div>
                <p class="detail-desc">
                  {{ formatEndpoint(selectedStore) || '未显示连接地址' }}
                </p>
              </div>
              <div class="detail-actions">
                <t-button size="small" variant="outline" :loading="testingId === selectedStore.id" @click="testExisting(selectedStore)">
                  测试连接
                </t-button>
                <t-button
                  v-if="selectedStore.source !== 'env'"
                  size="small"
                  variant="outline"
                  @click="openEdit(selectedStore)"
                >
                  编辑
                </t-button>
                <t-popconfirm
                  v-if="selectedStore.source !== 'env'"
                  content="确认删除该向量库配置？"
                  theme="danger"
                  @confirm="removeStore(selectedStore)"
                >
                  <t-button size="small" theme="danger" variant="text">删除</t-button>
                </t-popconfirm>
              </div>
            </div>

            <div class="stats-grid">
              <div class="stat-item">
                <div class="stat-label">知识库绑定</div>
                <div class="stat-value">{{ selectedStore.knowledge_base_count || 0 }}</div>
              </div>
              <div class="stat-item">
                <div class="stat-label">连接来源</div>
                <div class="stat-value">{{ selectedStore.source === 'env' ? '环境变量' : '手工配置' }}</div>
              </div>
              <div class="stat-item">
                <div class="stat-label">索引字段</div>
                <div class="stat-value">{{ selectedStoreType?.index_fields?.length || 0 }}</div>
              </div>
              <div class="stat-item">
                <div class="stat-label">测试状态</div>
                <div class="stat-value">{{ testingId === selectedStore.id ? '测试中' : '可测试' }}</div>
              </div>
            </div>

            <div class="section-block">
              <div class="section-headline">连接配置</div>
              <div class="kv-list">
                <div
                  v-for="field in selectedStoreType?.connection_fields || []"
                  :key="field.name"
                  class="kv-row"
                >
                  <div class="kv-label">{{ fieldLabel(field) }}</div>
                  <div class="kv-value">
                    {{ renderStoreValue(selectedStore.connection_config?.[field.name], field) }}
                  </div>
                </div>
              </div>
            </div>

            <div class="section-block">
              <div class="section-headline">索引策略</div>
              <div class="kv-list">
                <div
                  v-for="field in selectedStoreType?.index_fields || []"
                  :key="field.name"
                  class="kv-row"
                >
                  <div class="kv-label">{{ fieldLabel(field) }}</div>
                  <div class="kv-value">
                    {{ renderStoreValue(selectedStore.index_config?.[field.name], field) }}
                  </div>
                </div>
                <div v-if="!(selectedStoreType?.index_fields || []).length" class="kv-row">
                  <div class="kv-label">默认策略</div>
                  <div class="kv-value">使用引擎默认配置</div>
                </div>
              </div>
            </div>

            <div class="section-block">
              <div class="section-headline">
                知识库绑定
                <t-tag size="small" variant="outline">{{ selectedBindings.length }}</t-tag>
              </div>
              <div v-if="bindingLoading" class="binding-loading">
                <t-loading size="small" />
              </div>
              <div v-else-if="selectedBindings.length" class="binding-list">
                <button
                  v-for="kb in selectedBindings"
                  :key="kb.id"
                  type="button"
                  class="binding-item"
                  @click="openKnowledgeBase(kb.id)"
                >
                  <div class="binding-item-head">
                    <div class="binding-name">{{ kb.name }}</div>
                    <t-tag size="small" variant="outline">{{ kb.type }}</t-tag>
                  </div>
                  <div class="binding-meta">
                    <span>{{ kb.knowledge_count || 0 }} 条知识</span>
                    <span>{{ kb.chunk_count || 0 }} 个块</span>
                    <span>{{ formatDate(kb.updated_at) }}</span>
                  </div>
                </button>
              </div>
              <t-empty v-else description="暂无绑定知识库" />
            </div>
          </template>

          <div v-else class="empty-detail">
            <t-empty description="请选择一个向量库" />
          </div>
        </section>
      </div>
    </t-loading>

    <t-dialog
      v-model:visible="dialogVisible"
      :header="editingStore ? '编辑向量库' : '新增向量库'"
      width="720px"
      :footer="false"
      destroy-on-close
    >
      <t-form :data="form" label-align="top" @submit="saveStore">
        <t-form-item label="名称" name="name" :rules="[{ required: true, message: '请输入名称' }]">
          <t-input v-model="form.name" placeholder="例如：生产 Qdrant" />
        </t-form-item>

        <t-form-item v-if="!editingStore" label="引擎类型" name="engine_type" :rules="[{ required: true, message: '请选择引擎类型' }]">
          <t-select v-model="form.engine_type" @change="handleTypeChange">
            <t-option v-for="type in storeTypes" :key="type.type" :value="type.type" :label="type.display_name" />
          </t-select>
        </t-form-item>

        <template v-if="!editingStore && selectedType">
          <div class="dialog-subtitle">连接配置</div>
          <t-form-item
            v-for="field in selectedType.connection_fields"
            :key="field.name"
            :label="fieldLabel(field)"
            :name="`connection_config.${field.name}`"
            :rules="field.required ? [{ required: true, message: `请输入${fieldLabel(field)}` }] : []"
          >
            <t-switch v-if="field.type === 'boolean'" v-model="form.connection_config[field.name]" />
            <t-input-number
              v-else-if="field.type === 'number'"
              v-model="form.connection_config[field.name]"
              theme="normal"
              style="width: 100%"
              :placeholder="field.default !== undefined ? String(field.default) : ''"
            />
            <t-input
              v-else
              v-model="form.connection_config[field.name]"
              :type="field.sensitive ? 'password' : 'text'"
              :placeholder="field.default !== undefined ? String(field.default) : ''"
            />
          </t-form-item>

          <template v-if="selectedType.index_fields?.length">
            <div class="dialog-subtitle">索引配置</div>
            <t-form-item v-for="field in selectedType.index_fields" :key="field.name" :label="fieldLabel(field)">
              <t-input-number
                v-if="field.type === 'number'"
                v-model="form.index_config[field.name]"
                theme="normal"
                style="width: 100%"
                :placeholder="field.default !== undefined ? String(field.default) : ''"
              />
              <t-input
                v-else
                v-model="form.index_config[field.name]"
                :placeholder="field.default !== undefined ? String(field.default) : ''"
              />
            </t-form-item>
          </template>
        </template>

        <div class="dialog-footer">
          <t-button v-if="!editingStore" variant="outline" :loading="testingDialog" @click="testDialog">
            测试连接
          </t-button>
          <span v-else />
          <div class="dialog-actions">
            <t-button variant="outline" @click="dialogVisible = false">取消</t-button>
            <t-button theme="primary" type="submit" :loading="saving">保存</t-button>
          </div>
        </div>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { AddIcon } from 'tdesign-icons-vue-next'
import {
  createVectorStore,
  deleteVectorStore,
  getVectorStore,
  listKnowledgeBasesForVectorStore,
  listVectorStoreTypes,
  listVectorStores,
  testVectorStoreById,
  testVectorStoreRaw,
  updateVectorStore,
  type VectorStoreEntity,
  type VectorStoreFieldInfo,
  type VectorStoreKnowledgeBaseBinding,
  type VectorStoreTypeInfo,
} from '@/api/vectorstore'

const router = useRouter()

const loading = ref(false)
const saving = ref(false)
const testingDialog = ref(false)
const testingId = ref<string>('')
const dialogVisible = ref(false)
const editingStore = ref<VectorStoreEntity | null>(null)
const stores = ref<VectorStoreEntity[]>([])
const storeTypes = ref<VectorStoreTypeInfo[]>([])
const selectedStoreId = ref<string>('')
const selectedBindings = ref<VectorStoreKnowledgeBaseBinding[]>([])
const bindingLoading = ref(false)
const searchText = ref('')
const form = ref({
  name: '',
  engine_type: '',
  connection_config: {} as Record<string, any>,
  index_config: {} as Record<string, any>,
})

const selectedStore = computed(() => stores.value.find(item => item.id === selectedStoreId.value) || null)
const selectedType = computed(() => storeTypes.value.find(item => item.type === form.value.engine_type))
const selectedStoreType = computed(() => {
  if (!selectedStore.value) return undefined
  return storeTypes.value.find(item => item.type === selectedStore.value?.engine_type)
})

const filteredStores = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  if (!query) return stores.value
  return stores.value.filter(store => {
    const endpoint = formatEndpoint(store).toLowerCase()
    return [
      store.name,
      store.engine_type,
      store.source || '',
      endpoint,
    ].some(value => String(value).toLowerCase().includes(query))
  })
})

const fieldLabel = (field: VectorStoreFieldInfo) => field.description || field.name

const defaultValueForField = (field: VectorStoreFieldInfo) => {
  if (field.default === undefined || field.default === null) return field.type === 'boolean' ? false : ''
  if (field.type === 'number') return Number(field.default) || 0
  if (field.type === 'boolean') return Boolean(field.default)
  return field.default
}

const applyTypeDefaults = (engineType: string) => {
  const typeInfo = storeTypes.value.find(item => item.type === engineType)
  if (!typeInfo) {
    form.value.connection_config = {}
    form.value.index_config = {}
    return
  }
  const connectionConfig: Record<string, any> = {}
  const indexConfig: Record<string, any> = {}
  typeInfo.connection_fields.forEach(field => {
    if (field.default !== undefined) connectionConfig[field.name] = defaultValueForField(field)
  })
  typeInfo.index_fields?.forEach(field => {
    if (field.default !== undefined) indexConfig[field.name] = defaultValueForField(field)
  })
  form.value.connection_config = connectionConfig
  form.value.index_config = indexConfig
}

const handleTypeChange = (value: string) => {
  form.value.engine_type = value
  applyTypeDefaults(value)
}

const formatEndpoint = (store: VectorStoreEntity | null) => {
  if (!store) return ''
  const config = store.connection_config || {}
  return config.addr || config.host || config.url || config.endpoint || ''
}

const formatDate = (value?: string) => {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return date.toLocaleString()
}

const renderStoreValue = (value: any, field: VectorStoreFieldInfo) => {
  if (field.type === 'boolean') return value ? '是' : '否'
  if (field.type === 'number') return value === undefined || value === null || value === '' ? '--' : String(value)
  if (value === undefined || value === null || value === '') return '--'
  return String(value)
}

const loadSelectedBindings = async () => {
  if (!selectedStoreId.value) {
    selectedBindings.value = []
    return
  }
  bindingLoading.value = true
  try {
    selectedBindings.value = await listKnowledgeBasesForVectorStore(selectedStoreId.value)
  } catch (error: any) {
    selectedBindings.value = []
    MessagePlugin.error(error?.message || '加载知识库绑定失败')
  } finally {
    bindingLoading.value = false
  }
}

const loadStores = async () => {
  loading.value = true
  try {
    const [types, result] = await Promise.all([listVectorStoreTypes(), listVectorStores()])
    storeTypes.value = types
    stores.value = Array.isArray(result?.data) ? result.data : []
    if (!selectedStoreId.value || !stores.value.some(store => store.id === selectedStoreId.value)) {
      selectedStoreId.value = stores.value[0]?.id || ''
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || '加载向量库失败')
  } finally {
    loading.value = false
  }
}

const refreshSelectedStore = async () => {
  if (!selectedStoreId.value) return
  try {
    const result = await getVectorStore(selectedStoreId.value)
    const updated = result?.data
    if (updated?.id) {
      const index = stores.value.findIndex(item => item.id === updated.id)
      if (index >= 0) {
        stores.value[index] = { ...stores.value[index], ...updated }
      }
    }
  } catch (error) {
    console.error('refresh selected store failed', error)
  }
  await loadSelectedBindings()
}

const selectStore = (store: VectorStoreEntity) => {
  if (!store.id) return
  selectedStoreId.value = store.id
}

const openCreate = () => {
  editingStore.value = null
  const firstType = storeTypes.value[0]?.type || ''
  form.value = {
    name: '',
    engine_type: firstType,
    connection_config: {},
    index_config: {},
  }
  if (firstType) applyTypeDefaults(firstType)
  dialogVisible.value = true
}

const openEdit = (store: VectorStoreEntity) => {
  editingStore.value = store
  form.value = {
    name: store.name,
    engine_type: store.engine_type,
    connection_config: { ...(store.connection_config || {}) },
    index_config: { ...(store.index_config || {}) },
  }
  dialogVisible.value = true
}

const saveStore = async ({ validateResult, firstError }: any) => {
  if (validateResult !== true && validateResult !== undefined) {
    MessagePlugin.warning(firstError || '请检查表单')
    return
  }
  saving.value = true
  try {
    if (editingStore.value?.id) {
      await updateVectorStore(editingStore.value.id, { name: form.value.name.trim() })
      MessagePlugin.success('向量库已更新')
      selectedStoreId.value = editingStore.value.id
    } else {
      const response: any = await createVectorStore({
        name: form.value.name.trim(),
        engine_type: form.value.engine_type,
        connection_config: { ...form.value.connection_config },
        index_config: { ...form.value.index_config },
      })
      const newId = response?.data?.id || response?.data?.vector_store?.id
      if (newId) {
        selectedStoreId.value = newId
      }
      MessagePlugin.success('向量库已创建')
    }
    dialogVisible.value = false
    await loadStores()
    await loadSelectedBindings()
  } catch (error: any) {
    MessagePlugin.error(error?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const testDialog = async () => {
  testingDialog.value = true
  try {
    const result: any = await testVectorStoreRaw({
      engine_type: form.value.engine_type,
      connection_config: { ...form.value.connection_config },
    })
    if (result?.success) {
      MessagePlugin.success(result?.version ? `连接成功，版本 ${result.version}` : '连接测试成功')
    } else {
      MessagePlugin.error(result?.error || '连接测试失败')
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || '连接测试失败')
  } finally {
    testingDialog.value = false
  }
}

const testExisting = async (store: VectorStoreEntity) => {
  if (!store.id) return
  testingId.value = store.id
  try {
    const result: any = await testVectorStoreById(store.id)
    if (result?.success) {
      MessagePlugin.success(result?.version ? `连接成功，版本 ${result.version}` : '连接测试成功')
      await refreshSelectedStore()
    } else {
      MessagePlugin.error(result?.error || '连接测试失败')
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || '连接测试失败')
  } finally {
    testingId.value = ''
  }
}

const removeStore = async (store: VectorStoreEntity) => {
  if (!store.id) return
  try {
    await deleteVectorStore(store.id)
    MessagePlugin.success('向量库已删除')
    if (selectedStoreId.value === store.id) {
      selectedStoreId.value = ''
    }
    await loadStores()
    await loadSelectedBindings()
  } catch (error: any) {
    MessagePlugin.error(error?.message || '删除失败')
  }
}

const openKnowledgeBase = (kbId: string) => {
  if (!kbId) return
  router.push(`/platform/knowledge-bases/${kbId}`)
}

watch(selectedStoreId, async () => {
  await loadSelectedBindings()
}, { immediate: false })

onMounted(async () => {
  await loadStores()
  await loadSelectedBindings()
})
</script>

<style scoped lang="less">
.vector-store-settings {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;

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

.page-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.search-input {
  width: 260px;
}

.workspace {
  display: grid;
  grid-template-columns: 360px minmax(0, 1fr);
  gap: 16px;
  min-height: 640px;
}

.store-panel,
.detail-panel {
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  overflow: hidden;
}

.store-panel {
  display: flex;
  flex-direction: column;
}

.panel-title {
  padding: 14px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.store-list {
  display: flex;
  flex-direction: column;
  padding: 10px;
  gap: 8px;
}

.store-item {
  width: 100%;
  text-align: left;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  padding: 12px 14px;
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color);
  }

  &.active {
    border-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }
}

.store-item-head,
.detail-title-row,
.detail-actions,
.binding-item-head,
.store-tags {
  display: flex;
  align-items: center;
  gap: 8px;
}

.store-item-head {
  justify-content: space-between;
  gap: 12px;
}

.store-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}

.store-item-meta,
.binding-meta {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.detail-panel {
  padding: 16px 18px 18px;
}

.detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.detail-title-row h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.detail-desc {
  margin: 8px 0 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.detail-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin: 16px 0;
}

.stat-item {
  padding: 12px 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
}

.stat-label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.stat-value {
  margin-top: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  word-break: break-all;
}

.section-block {
  padding-top: 14px;
  margin-top: 14px;
  border-top: 1px solid var(--td-component-stroke);
}

.section-headline {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.kv-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.kv-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
}

.kv-label {
  min-width: 160px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.kv-value {
  flex: 1;
  text-align: right;
  font-size: 13px;
  color: var(--td-text-color-primary);
  word-break: break-all;
}

.binding-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px 0;
}

.binding-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.binding-item {
  width: 100%;
  text-align: left;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  padding: 12px 14px;
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }
}

.binding-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.empty-detail {
  min-height: 560px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.dialog-subtitle {
  margin: 18px 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.dialog-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid var(--td-component-stroke);
}

.dialog-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (max-width: 1100px) {
  .workspace {
    grid-template-columns: 1fr;
  }

  .stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .page-head,
  .detail-head,
  .dialog-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .page-actions {
    justify-content: flex-start;
  }

  .search-input {
    width: 100%;
  }

  .stats-grid {
    grid-template-columns: 1fr;
  }

  .kv-row {
    flex-direction: column;
  }

  .kv-label,
  .kv-value {
    text-align: left;
  }
}
</style>

<template>
  <section class="tab-content">
    <div class="content-header">
      <div>
        <h2>数据库连接列表</h2>
        <p class="description">管理监控的 MySQL、PostgreSQL、SQL Server 数据库实例，并配置各库的数据表黑白名单规则</p>
      </div>
      <button type="button" class="btn btn-primary" @click="openAddConnectionModal">
        + 新建连接
      </button>
    </div>

    <div v-if="loading" class="loading-box">
      <div class="spinner"></div> 正在加载连接...
    </div>

    <div v-else-if="connections.length === 0" class="empty-card">
      <div class="empty-icon">📭</div>
      <h3>暂无数据库连接</h3>
      <p>点击右上角“+ 新建连接”添加您的第一个数据库连接配置</p>
      <button type="button" class="btn btn-primary" @click="openAddConnectionModal">立即添加</button>
    </div>

    <div v-else class="connection-grid">
      <div v-for="conn in connections" :key="conn.id" class="connection-card">
        <div class="card-header">
          <div class="card-title">
            <span class="db-status-dot"></span>
            <h3>{{ conn.name || conn.id }}</h3>
            <span class="badge badge-vendor">{{ getVendorLabel(vendors, conn.vendor) }}</span>
            <span class="badge badge-id">{{ conn.id }}</span>
          </div>
          <div class="card-actions">
            <button type="button" class="btn-icon" title="编辑" @click="editConnection(conn)">✏️</button>
            <button type="button" class="btn-icon btn-icon-danger" title="删除" @click="deleteConn(conn.id)">🗑️</button>
          </div>
        </div>

        <div class="card-body">
          <div class="info-row">
            <span class="label">主机端口:</span>
            <span class="value">{{ conn.host }}:{{ conn.port }}</span>
          </div>
          <div class="info-row">
            <span class="label">目标数据库:</span>
            <span class="value font-bold">{{ conn.database }}</span>
          </div>
          <div v-if="conn.vendor && conn.vendor !== 'mysql'" class="info-row">
            <span class="label">Schema 模式:</span>
            <span class="value font-bold">{{ conn.schema || getDefaultSchema(conn.vendor) }}</span>
          </div>
          <div class="info-row">
            <span class="label">监控模式:</span>
            <span :class="['badge', conn.filter?.monitor_mode === 'schema_only' ? 'badge-warning' : 'badge-success']">
              {{ conn.filter?.monitor_mode === 'schema_only' ? '仅结构 (DDL)' : '结构与数据 (DDL+DML)' }}
            </span>
          </div>
          <div class="info-row">
            <span class="label">表白名单:</span>
            <span class="value filter-tag-list">
              <template v-if="conn.filter?.include_tables && conn.filter.include_tables.length > 0">
                <span v-for="t in conn.filter.include_tables" :key="t" class="tag tag-include">{{ t }}</span>
              </template>
              <span v-else class="text-muted">全量表 (无限制)</span>
            </span>
          </div>
          <div class="info-row">
            <span class="label">表黑名单:</span>
            <span class="value filter-tag-list">
              <template v-if="conn.filter?.exclude_tables && conn.filter.exclude_tables.length > 0">
                <span v-for="t in conn.filter.exclude_tables" :key="t" class="tag tag-exclude">{{ t }}</span>
              </template>
              <span v-else class="text-muted">无</span>
            </span>
          </div>
        </div>

        <div class="card-footer">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="testingConnId === conn.id"
            @click="testConn(conn)"
          >
            {{ testingConnId === conn.id ? '测试中...' : '测试连通性' }}
          </button>
          <button type="button" class="btn btn-outline btn-sm" @click="$emit('go-snapshots', conn.id)">
            查看快照 →
          </button>
        </div>

        <!-- 连通性测试结果展示 -->
        <div
          v-if="connTestResults[conn.id]"
          :class="['test-result-box', connTestResults[conn.id].success ? 'success' : 'error']"
        >
          <div class="test-title">
            {{ connTestResults[conn.id].success ? '✓ ' + connTestResults[conn.id].message : '✗ ' + connTestResults[conn.id].message }}
          </div>
          <div v-if="connTestResults[conn.id].success" class="test-details">
            <span>版本: {{ connTestResults[conn.id].server_version }}</span>
            <span>延迟: {{ connTestResults[conn.id].latency_ms }}ms</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 新增 / 编辑连接弹窗 -->
    <ConnectionModal
      :visible="showConnModal"
      :editing-connection="editingConnection"
      :vendors="vendors"
      @close="showConnModal = false"
      @saved="$emit('refresh')"
    />
  </section>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { deleteConnection, testConnection } from '../api/connection.js'
import ConnectionModal from '../components/ConnectionModal.vue'
import { useToast } from '../composables/useToast.js'
import { formatError } from '../utils/format.js'
import { getDefaultSchema, getVendorLabel } from '../utils/vendor.js'
import '../styles/connections.css'

// 数据库连接管理视图：负责连接列表展示、增删改、连通性测试及跳转快照
defineProps({
  connections: {
    type: Array,
    default: () => [],
  },
  vendors: {
    type: Array,
    default: () => [],
  },
  loading: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['refresh', 'go-snapshots', 'connection-deleted'])
const { showToast } = useToast()

const showConnModal = ref(false)
const editingConnection = ref(null)
const testingConnId = ref('')
const connTestResults = reactive({})

const openAddConnectionModal = () => {
  editingConnection.value = null
  showConnModal.value = true
}

const editConnection = (conn) => {
  editingConnection.value = conn
  showConnModal.value = true
}

const deleteConn = async (id) => {
  if (!confirm(`确定要删除连接 "${id}" 吗？`)) return
  try {
    await deleteConnection(id)
    delete connTestResults[id]
    showToast('删除成功', 'success')
    emit('connection-deleted', id)
    emit('refresh')
  } catch (err) {
    showToast('删除失败: ' + formatError(err), 'error')
  }
}

const testConn = async (conn) => {
  testingConnId.value = conn.id
  try {
    const res = await testConnection(conn)
    connTestResults[conn.id] = res
    if (res.success) {
      showToast(`连接测试通过 (延迟: ${res.latency_ms}ms)`, 'success')
    } else {
      showToast('连接测试失败: ' + res.message, 'error')
    }
  } catch (err) {
    showToast('测试异常: ' + formatError(err), 'error')
  } finally {
    testingConnId.value = ''
  }
}
</script>

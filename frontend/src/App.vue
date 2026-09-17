<template>
  <div class="app-container">
    <!-- 顶部导航栏 -->
    <header class="app-header">
      <div class="header-brand">
        <div class="logo-icon">🐬</div>
        <div class="brand-text">
          <h1>MySQL Monitor</h1>
          <span class="sub-title">DDL & DML 监控与版本比对工具</span>
        </div>
      </div>
      <nav class="nav-tabs">
        <button 
          :class="['nav-tab', activeTab === 'connections' ? 'active' : '']"
          @click="activeTab = 'connections'"
        >
          <span class="tab-icon">🔌</span> 数据库连接管理
        </button>
        <button 
          :class="['nav-tab', activeTab === 'snapshots' ? 'active' : '']"
          @click="activeTab = 'snapshots'"
        >
          <span class="tab-icon">📸</span> 快照与基准版本
        </button>
        <button 
          :class="['nav-tab', activeTab === 'diff' ? 'active' : '']"
          @click="activeTab = 'diff'"
        >
          <span class="tab-icon">⚖️</span> 历史版本差异比对
        </button>
      </nav>
    </header>

    <!-- 主体内容区 -->
    <main class="app-main">
      <!-- 提示消息通知框 -->
      <div v-if="toast.visible" :class="['toast-notification', toast.type]">
        <span>{{ toast.message }}</span>
      </div>

      <!-- ================= 1. 数据库连接管理 ================= -->
      <section v-if="activeTab === 'connections'" class="tab-content">
        <div class="content-header">
          <div>
            <h2>数据库连接列表</h2>
            <p class="description">管理监控的 MySQL 数据库实例，并配置各库的数据表黑白名单规则</p>
          </div>
          <button class="btn btn-primary" @click="openAddConnectionModal">
            + 新建连接
          </button>
        </div>

        <div v-if="loadingConnections" class="loading-box">
          <div class="spinner"></div> 正在加载连接...
        </div>

        <div v-else-if="connections.length === 0" class="empty-card">
          <div class="empty-icon">📭</div>
          <h3>暂无数据库连接</h3>
          <p>点击右上角“+ 新建连接”添加您的第一个 MySQL 数据库连接配置</p>
          <button class="btn btn-primary" @click="openAddConnectionModal">立即添加</button>
        </div>

        <div v-else class="connection-grid">
          <div v-for="conn in connections" :key="conn.id" class="connection-card">
            <div class="card-header">
              <div class="card-title">
                <span class="db-status-dot"></span>
                <h3>{{ conn.name || conn.id }}</h3>
                <span class="badge badge-id">{{ conn.id }}</span>
              </div>
              <div class="card-actions">
                <button class="btn-icon" title="编辑" @click="editConnection(conn)">✏️</button>
                <button class="btn-icon btn-icon-danger" title="删除" @click="deleteConn(conn.id)">🗑️</button>
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
              <div class="info-row">
                <span class="label">监控模式:</span>
                <span :class="['badge', conn.filter.monitor_mode === 'schema_only' ? 'badge-warning' : 'badge-success']">
                  {{ conn.filter.monitor_mode === 'schema_only' ? '仅结构 (DDL)' : '结构与数据 (DDL+DML)' }}
                </span>
              </div>
              <div class="info-row">
                <span class="label">表白名单:</span>
                <span class="value filter-tag-list">
                  <template v-if="conn.filter.include_tables && conn.filter.include_tables.length > 0">
                    <span v-for="t in conn.filter.include_tables" :key="t" class="tag tag-include">{{ t }}</span>
                  </template>
                  <span v-else class="text-muted">全量表 (无限制)</span>
                </span>
              </div>
              <div class="info-row">
                <span class="label">表黑名单:</span>
                <span class="value filter-tag-list">
                  <template v-if="conn.filter.exclude_tables && conn.filter.exclude_tables.length > 0">
                    <span v-for="t in conn.filter.exclude_tables" :key="t" class="tag tag-exclude">{{ t }}</span>
                  </template>
                  <span v-else class="text-muted">无</span>
                </span>
              </div>
            </div>

            <div class="card-footer">
              <button 
                class="btn btn-secondary btn-sm" 
                :disabled="testingConnId === conn.id"
                @click="testConn(conn)"
              >
                {{ testingConnId === conn.id ? '测试中...' : '测试连通性' }}
              </button>
              <button class="btn btn-outline btn-sm" @click="goToSnapshots(conn.id)">
                查看快照 →
              </button>
            </div>

            <!-- 测试结果展示 -->
            <div v-if="connTestResults[conn.id]" :class="['test-result-box', connTestResults[conn.id].success ? 'success' : 'error']">
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
      </section>

      <!-- ================= 2. 快照与基准版本管理 ================= -->
      <section v-if="activeTab === 'snapshots'" class="tab-content">
        <div class="content-header">
          <div>
            <h2>快照与基准版本管理</h2>
            <p class="description">提取并查看指定数据库的结构及数据历史快照，管理版本基线 (Baseline)</p>
          </div>
          <div class="header-controls">
            <select v-model="selectedConnId" class="select-input" @change="loadSnapshots">
              <option value="" disabled>-- 请选择数据库连接 --</option>
              <option v-for="c in connections" :key="c.id" :value="c.id">
                {{ c.name }} ({{ c.database }} @ {{ c.host }})
              </option>
            </select>
            <button 
              class="btn btn-primary" 
              :disabled="!selectedConnId || capturing"
              @click="openCaptureModal"
            >
              {{ capturing ? '正在抓取快照...' : '📸 抓取当前快照' }}
            </button>
          </div>
        </div>

        <div v-if="!selectedConnId" class="notice-card">
          <span>👈 请在右上角下拉选择一个数据库连接以管理其历史快照版本</span>
        </div>

        <div v-else>
          <!-- 基准版本提示条 -->
          <div v-if="!snapshots || snapshots.length === 0" class="baseline-banner">
            <div class="banner-icon">💡</div>
            <div class="banner-text">
              <strong>基准版本提示：</strong> 当前连接尚无任何历史版本记录。首次点击“抓取当前快照”时，系统将自动将其标记为<strong>【基准版本 (Baseline)】</strong>，作为后续所有版本比对的起点。
            </div>
          </div>

          <div v-if="loadingSnapshots" class="loading-box">
            <div class="spinner"></div> 正在加载快照列表...
          </div>

          <div v-else-if="!snapshots || snapshots.length === 0" class="empty-card">
            <div class="empty-icon">📂</div>
            <h3>暂无快照数据</h3>
            <p>点击上方“📸 抓取当前快照”以拉取首个基准版本</p>
          </div>

          <div v-else class="table-container">
            <table class="data-table">
              <thead>
                <tr>
                  <th>版本号 (Version ID)</th>
                  <th>类型</th>
                  <th>描述备注</th>
                  <th>表数量</th>
                  <th>记录总数</th>
                  <th>抓取时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="snap in snapshots" :key="snap.version_id">
                  <td class="font-mono font-bold">{{ snap.version_id }}</td>
                  <td>
                    <span :class="['badge', snap.is_baseline ? 'badge-baseline' : 'badge-snapshot']">
                      {{ snap.is_baseline ? '★ 基准版本 (Baseline)' : '增量快照' }}
                    </span>
                  </td>
                  <td>{{ snap.description || '-' }}</td>
                  <td>{{ snap.table_count }} 张表</td>
                  <td>{{ snap.row_count }} 行</td>
                  <td>{{ formatDate(snap.created_at) }}</td>
                  <td>
                    <div class="action-buttons">
                      <button class="btn btn-xs btn-outline" @click="selectForDiff(snap.version_id)">
                        比对
                      </button>
                      <button class="btn btn-xs btn-danger" @click="delSnapshot(snap.version_id)">
                        删除
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <!-- ================= 3. 历史版本差异比对 ================= -->
      <section v-if="activeTab === 'diff'" class="tab-content">
        <div class="content-header">
          <div>
            <h2>历史版本差异比对与 SQL 生成</h2>
            <p class="description">任意选择起始基准版本与目标版本，自动分析结构 (DDL) 与数据 (DML) 变动并导出 SQL</p>
          </div>
        </div>

        <!-- 比对控制栏 -->
        <div class="diff-control-panel">
          <div class="control-col">
            <label>目标数据库连接</label>
            <select v-model="diffConnId" class="select-input" @change="loadDiffSnapshots">
              <option value="" disabled>-- 请选择数据库连接 --</option>
              <option v-for="c in connections" :key="c.id" :value="c.id">
                {{ c.name }} ({{ c.database }})
              </option>
            </select>
          </div>

          <div class="control-col">
            <label>起始 / 基准版本 (From)</label>
            <select v-model="fromVersion" class="select-input" :disabled="!diffSnapshots || diffSnapshots.length === 0">
              <option value="" disabled>-- 请选择起始版本 --</option>
              <option v-for="s in diffSnapshots" :key="'from_'+s.version_id" :value="s.version_id">
                {{ s.version_id }} {{ s.is_baseline ? '[基准]' : '' }} - {{ s.description }}
              </option>
            </select>
          </div>

          <div class="diff-arrow">➔</div>

          <div class="control-col">
            <label>目标 / 新版本 (To)</label>
            <select v-model="toVersion" class="select-input" :disabled="!diffSnapshots || diffSnapshots.length === 0">
              <option value="" disabled>-- 请选择目标版本 --</option>
              <option v-for="s in diffSnapshots" :key="'to_'+s.version_id" :value="s.version_id">
                {{ s.version_id }} {{ s.is_baseline ? '[基准]' : '' }} - {{ s.description }}
              </option>
            </select>
          </div>

          <div class="control-action">
            <button 
              class="btn btn-primary btn-diff" 
              :disabled="!diffConnId || !fromVersion || !toVersion || comparing"
              @click="runDiff"
            >
              {{ comparing ? '比对中...' : '⚡ 开始版本比对' }}
            </button>
          </div>
        </div>

        <!-- 比对结果展示区 -->
        <div v-if="diffResult" class="diff-result-section">
          <!-- 变动摘要统计卡片 -->
          <div class="stats-grid">
            <div class="stat-card">
              <div class="stat-icon">🏗️</div>
              <div class="stat-content">
                <div class="stat-title">表结构变动 (DDL)</div>
                <div class="stat-numbers">
                  <span class="stat-tag add">+{{ diffResult.summary.ddl.added_tables }} 表</span>
                  <span class="stat-tag del">-{{ diffResult.summary.ddl.dropped_tables }} 表</span>
                  <span class="stat-tag mod">~{{ diffResult.summary.ddl.modified_tables }} 表</span>
                </div>
                <div class="stat-sub">
                  字段: +{{ diffResult.summary.ddl.added_columns }} / -{{ diffResult.summary.ddl.dropped_columns }} / ~{{ diffResult.summary.ddl.modified_columns }} | 
                  索引: +{{ diffResult.summary.ddl.added_indexes }} / -{{ diffResult.summary.ddl.dropped_indexes }}
                </div>
              </div>
            </div>

            <div class="stat-card">
              <div class="stat-icon">📝</div>
              <div class="stat-content">
                <div class="stat-title">数据记录变动 (DML)</div>
                <div class="stat-numbers">
                  <span class="stat-tag add">+{{ diffResult.summary.dml.insert_count }} INSERT</span>
                  <span class="stat-tag mod">~{{ diffResult.summary.dml.update_count }} UPDATE</span>
                  <span class="stat-tag del">-{{ diffResult.summary.dml.delete_count }} DELETE</span>
                </div>
                <div class="stat-sub">
                  总计变动 {{ diffResult.summary.dml.insert_count + diffResult.summary.dml.update_count + diffResult.summary.dml.delete_count }} 行记录
                </div>
              </div>
            </div>
          </div>

          <!-- SQL 代码展示与导出栏 -->
          <div class="sql-viewer-card">
            <div class="sql-viewer-header">
              <div class="script-tabs">
                <button 
                  :class="['script-tab', sqlTab === 'full' ? 'active' : '']"
                  @click="sqlTab = 'full'"
                >
                  完整同步脚本 (带事务保护)
                </button>
                <button 
                  :class="['script-tab', sqlTab === 'ddl' ? 'active' : '']"
                  @click="sqlTab = 'ddl'"
                >
                  仅 DDL 结构变动
                </button>
                <button 
                  :class="['script-tab', sqlTab === 'dml' ? 'active' : '']"
                  @click="sqlTab = 'dml'"
                >
                  仅 DML 数据变动
                </button>
              </div>

              <div class="script-actions">
                <button class="btn btn-secondary btn-sm" @click="copyActiveSQL">
                  📋 复制当前 SQL
                </button>
                <button class="btn btn-success btn-sm" @click="exportSQL">
                  💾 导出 .sql 文件
                </button>
              </div>
            </div>

            <div class="sql-code-container">
              <pre class="sql-code"><code>{{ activeSQLContent || '-- 经比对，选定的两个版本之间未检测到变动差异 --' }}</code></pre>
            </div>
          </div>
        </div>
      </section>
    </main>

    <!-- ================= 连接新增 / 编辑弹窗 ================= -->
    <div v-if="showConnModal" class="modal-overlay" @click.self="closeConnModal">
      <div class="modal-dialog">
        <div class="modal-header">
          <h3>{{ editingConn ? '编辑数据库连接' : '新建数据库连接' }}</h3>
          <button class="close-btn" @click="closeConnModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-grid">
            <div class="form-group">
              <label>连接标识 ID *</label>
              <input v-model="connForm.id" :disabled="editingConn" type="text" placeholder="如 dev_order_db" />
            </div>
            <div class="form-group">
              <label>显示名称 *</label>
              <input v-model="connForm.name" type="text" placeholder="如 订单系统开发库" />
            </div>
            <div class="form-group">
              <label>主机地址 *</label>
              <input v-model="connForm.host" type="text" placeholder="127.0.0.1 或 remote.host" />
            </div>
            <div class="form-group">
              <label>端口 *</label>
              <input v-model.number="connForm.port" type="number" placeholder="3306" />
            </div>
            <div class="form-group">
              <label>用户名 *</label>
              <input v-model="connForm.user" type="text" placeholder="root" />
            </div>
            <div class="form-group">
              <label>密码</label>
              <input 
                v-model="connForm.password" 
                type="password" 
                :placeholder="editingConn ? '•••••••• (留空保留原密码)' : '请输入数据库密码'" 
              />
            </div>
            <div class="form-group">
              <label>目标数据库 *</label>
              <input v-model="connForm.database" type="text" placeholder="mall" />
            </div>
            <div class="form-group">
              <label>字符集</label>
              <input v-model="connForm.charset" type="text" placeholder="utf8mb4" />
            </div>
          </div>

          <div class="form-divider">表监控黑白名单与高级配置</div>

          <div class="form-group">
            <label>监控模式</label>
            <select v-model="connForm.filter.monitor_mode" class="select-input">
              <option value="schema_and_data">结构与数据全量监控 (DDL + DML)</option>
              <option value="schema_only">仅监控表结构 (DDL)</option>
            </select>
          </div>

          <div class="form-group">
            <label>表白名单列表 (每行或英文逗号分隔，留空表示监控所有表)</label>
            <textarea 
              v-model="includeTablesText" 
              rows="2" 
              placeholder="支持通配符与正则，例如: t_user, t_order_*, sys_*"
            ></textarea>
          </div>

          <div class="form-group">
            <label>表黑名单列表 (命中黑名单将坚决排除)</label>
            <textarea 
              v-model="excludeTablesText" 
              rows="2" 
              placeholder="例如: *_bak, tmp_*, sys_log"
            ></textarea>
          </div>

          <div class="form-group">
            <label>单表提取最大数据行数 (0 表示不限制)</label>
            <input v-model.number="connForm.filter.max_rows_per_table" type="number" placeholder="0" />
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeConnModal">取消</button>
          <button class="btn btn-primary" @click="submitConnection">保存配置</button>
        </div>
      </div>
    </div>

    <!-- ================= 快照抓取参数弹窗 ================= -->
    <div v-if="showCaptureModal" class="modal-overlay" @click.self="showCaptureModal = false">
      <div class="modal-dialog modal-dialog-sm">
        <div class="modal-header">
          <h3>抓取当前数据库快照</h3>
          <button class="close-btn" @click="showCaptureModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>快照备注说明</label>
            <input 
              v-model="captureDesc" 
              type="text" 
              placeholder="如：v1.2.0 发版前基准快照 / 数据库变更记录" 
            />
          </div>
          <div class="form-group-checkbox">
            <label>
              <input type="checkbox" v-model="captureForceBaseline" />
              <span>标记为新的基准版本 (Baseline)</span>
            </label>
            <p class="help-text">选中后，后续版本比对可直接将此快照作为新基准起点</p>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="showCaptureModal = false">取消</button>
          <button class="btn btn-primary" :disabled="capturing" @click="doCapture">
            {{ capturing ? '正在抓取中...' : '确认提取' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import * as api from './api.js'

// 标签页状态
const activeTab = ref('connections')

// 提示消息
const toast = reactive({
  visible: false,
  message: '',
  type: 'info',
})

const showToast = (message, type = 'info') => {
  toast.message = message
  toast.type = type
  toast.visible = true
  setTimeout(() => {
    toast.visible = false
  }, 3000)
}

// ================= 连接管理逻辑 =================
const connections = ref([])
const loadingConnections = ref(false)
const showConnModal = ref(false)
const editingConn = ref(false)
const testingConnId = ref('')
const connTestResults = reactive({})

const defaultConnForm = () => ({
  id: '',
  name: '',
  host: '127.0.0.1',
  port: 3306,
  user: 'root',
  password: '',
  database: '',
  charset: 'utf8mb4',
  timeout_seconds: 10,
  filter: {
    include_tables: [],
    exclude_tables: [],
    monitor_mode: 'schema_and_data',
    max_rows_per_table: 0,
  }
})

const connForm = reactive(defaultConnForm())
const includeTablesText = ref('')
const excludeTablesText = ref('')

const loadConnections = async () => {
  loadingConnections.value = true
  try {
    const list = await api.getConnections()
    connections.value = list || []
  } catch (err) {
    showToast('获取连接列表失败: ' + err, 'error')
  } finally {
    loadingConnections.value = false
  }
}

const openAddConnectionModal = () => {
  editingConn.value = false
  Object.assign(connForm, defaultConnForm())
  includeTablesText.value = ''
  excludeTablesText.value = ''
  showConnModal.value = true
}

const editConnection = (c) => {
  editingConn.value = true
  Object.assign(connForm, JSON.parse(JSON.stringify(c)))
  connForm.password = '' // 留空表示不修改已有密码
  includeTablesText.value = (c.filter.include_tables || []).join(', ')
  excludeTablesText.value = (c.filter.exclude_tables || []).join(', ')
  showConnModal.value = true
}

const closeConnModal = () => {
  showConnModal.value = false
}

const parseTableList = (str) => {
  if (!str) return []
  return str
    .split(/[\n,]+/)
    .map(s => s.trim())
    .filter(s => s.length > 0)
}

const submitConnection = async () => {
  if (!connForm.id || !connForm.host || !connForm.user || !connForm.database) {
    showToast('请完整填写必填字段', 'error')
    return
  }

  connForm.filter.include_tables = parseTableList(includeTablesText.value)
  connForm.filter.exclude_tables = parseTableList(excludeTablesText.value)

  try {
    await api.saveConnection(JSON.parse(JSON.stringify(connForm)))
    showToast('保存连接配置成功', 'success')
    closeConnModal()
    await loadConnections()
  } catch (err) {
    showToast('保存失败: ' + err, 'error')
  }
}

const deleteConn = async (id) => {
  if (!confirm(`确定要删除连接 "${id}" 吗？`)) return
  try {
    await api.deleteConnection(id)
    showToast('删除成功', 'success')
    await loadConnections()
  } catch (err) {
    showToast('删除失败: ' + err, 'error')
  }
}

const testConn = async (conn) => {
  testingConnId.value = conn.id
  try {
    const res = await api.testConnection(conn)
    connTestResults[conn.id] = res
    if (res.success) {
      showToast(`连接测试通过 (延迟: ${res.latency_ms}ms)`, 'success')
    } else {
      showToast('连接测试失败: ' + res.message, 'error')
    }
  } catch (err) {
    showToast('测试异常: ' + err, 'error')
  } finally {
    testingConnId.value = ''
  }
}

const goToSnapshots = (connId) => {
  selectedConnId.value = connId
  activeTab.value = 'snapshots'
  loadSnapshots()
}

// ================= 快照管理逻辑 =================
const selectedConnId = ref('')
const snapshots = ref([])
const loadingSnapshots = ref(false)
const capturing = ref(false)
const showCaptureModal = ref(false)
const captureDesc = ref('')
const captureForceBaseline = ref(false)

const loadSnapshots = async () => {
  if (!selectedConnId.value) return
  loadingSnapshots.value = true
  try {
    const list = await api.listSnapshots(selectedConnId.value)
    snapshots.value = list || []
  } catch (err) {
    snapshots.value = []
    showToast('获取快照失败: ' + err, 'error')
  } finally {
    loadingSnapshots.value = false
  }
}

const openCaptureModal = () => {
  captureDesc.value = ''
  // 若当前无快照，默认勾选基准
  captureForceBaseline.value = !snapshots.value || snapshots.value.length === 0
  showCaptureModal.value = true
}

const doCapture = async () => {
  capturing.value = true
  try {
    const meta = await api.captureSnapshot(selectedConnId.value, captureForceBaseline.value, captureDesc.value)
    showToast(`快照抓取成功 (${meta.version_id})`, 'success')
    showCaptureModal.value = false
    await loadSnapshots()
    if (diffConnId.value === selectedConnId.value) {
      await loadDiffSnapshots()
    }
  } catch (err) {
    showToast('抓取失败: ' + err, 'error')
  } finally {
    capturing.value = false
  }
}

const delSnapshot = async (verId) => {
  if (!confirm(`确定要删除快照版本 "${verId}" 吗？`)) return
  try {
    await api.deleteSnapshot(selectedConnId.value, verId)
    showToast('删除快照成功', 'success')
    // 如果比对页选中了被删除的版本，清理比对缓存与选中项
    if (fromVersion.value === verId) fromVersion.value = ''
    if (toVersion.value === verId) toVersion.value = ''
    diffResult.value = null
    await loadSnapshots()
    if (diffConnId.value === selectedConnId.value) {
      await loadDiffSnapshots()
    }
  } catch (err) {
    showToast('删除失败: ' + err, 'error')
  }
}

const selectForDiff = (verId) => {
  diffConnId.value = selectedConnId.value
  fromVersion.value = verId
  activeTab.value = 'diff'
  loadDiffSnapshots()
}

// ================= 版本比对逻辑 =================
const diffConnId = ref('')
const diffSnapshots = ref([])
const fromVersion = ref('')
const toVersion = ref('')
const comparing = ref(false)
const diffResult = ref(null)
const sqlTab = ref('full')

const loadDiffSnapshots = async () => {
  if (!diffConnId.value) return
  try {
    const list = await api.listSnapshots(diffConnId.value)
    diffSnapshots.value = list || []
    if (!list || list.length < 2) {
      fromVersion.value = ''
      toVersion.value = ''
      diffResult.value = null
    } else {
      if (!fromVersion.value || !list.find(s => s.version_id === fromVersion.value)) {
        fromVersion.value = list[list.length - 1].version_id
      }
      if (!toVersion.value || !list.find(s => s.version_id === toVersion.value)) {
        toVersion.value = list[0].version_id
      }
    }
  } catch (err) {
    diffSnapshots.value = []
    showToast('加载比对快照列表失败: ' + err, 'error')
  }
}

const runDiff = async () => {
  if (!diffConnId.value || !fromVersion.value || !toVersion.value) return
  if (fromVersion.value === toVersion.value) {
    showToast('起始版本和目标版本不能相同', 'warning')
    return
  }

  comparing.value = true
  try {
    diffResult.value = await api.compareVersions(diffConnId.value, fromVersion.value, toVersion.value)
    showToast('版本比对完成', 'success')
  } catch (err) {
    showToast('比对失败: ' + err, 'error')
  } finally {
    comparing.value = false
  }
}

const activeSQLContent = computed(() => {
  if (!diffResult.value) return ''
  if (sqlTab.value === 'ddl') return diffResult.value.ddl_script
  if (sqlTab.value === 'dml') return diffResult.value.dml_script
  return diffResult.value.full_script
})

const copyActiveSQL = async () => {
  if (!activeSQLContent.value) return
  try {
    await navigator.clipboard.writeText(activeSQLContent.value)
    showToast('SQL 语句已复制到剪贴板', 'success')
  } catch (err) {
    showToast('复制失败，请手动选择复制', 'error')
  }
}

const exportSQL = async () => {
  if (!activeSQLContent.value) return
  const defaultName = `diff_${diffConnId.value}_${fromVersion.value}_to_${toVersion.value}.sql`
  try {
    const savedPath = await api.exportSQLFile(defaultName, activeSQLContent.value)
    if (savedPath) {
      showToast(`已成功导出至: ${savedPath}`, 'success')
    }
  } catch (err) {
    showToast('导出文件失败: ' + err, 'error')
  }
}

const formatDate = (dateStr) => {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return d.toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  loadConnections()
})
</script>

<style scoped>
.app-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background-color: var(--bg-main);
  overflow: hidden;
}

/* 顶部导航 */
.app-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 24px;
  height: 64px;
  background-color: #ffffff;
  border-bottom: 1px solid var(--border);
  box-shadow: var(--shadow-sm);
  z-index: 10;
}

.header-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-icon {
  font-size: 28px;
}

.brand-text h1 {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-main);
}

.brand-text .sub-title {
  font-size: 11px;
  color: var(--text-muted);
}

.nav-tabs {
  display: flex;
  gap: 8px;
}

.nav-tab {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-muted);
  font-size: 14px;
  font-weight: 500;
}

.nav-tab:hover {
  background-color: #f1f5f9;
  color: var(--text-main);
}

.nav-tab.active {
  background-color: var(--primary-light);
  color: var(--primary);
  font-weight: 600;
}

/* 主内容区域 */
.app-main {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
  position: relative;
}

.content-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.content-header h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-main);
}

.content-header .description {
  font-size: 13px;
  color: var(--text-muted);
  margin-top: 2px;
}

.header-controls {
  display: flex;
  gap: 12px;
  align-items: center;
}

/* 按钮规范 */
.btn {
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.btn-primary {
  background-color: var(--primary);
  color: #ffffff;
}

.btn-primary:hover:not(:disabled) {
  background-color: var(--primary-hover);
}

.btn-secondary {
  background-color: #f1f5f9;
  color: #334155;
}

.btn-secondary:hover:not(:disabled) {
  background-color: #e2e8f0;
}

.btn-success {
  background-color: var(--success);
  color: #ffffff;
}

.btn-outline {
  border: 1px solid var(--border);
  background-color: #ffffff;
  color: #334155;
}

.btn-outline:hover:not(:disabled) {
  background-color: #f8fafc;
}

.btn-danger {
  background-color: var(--danger-light);
  color: var(--danger);
}

.btn-sm {
  padding: 5px 12px;
  font-size: 12px;
}

.btn-xs {
  padding: 3px 8px;
  font-size: 11px;
  border-radius: 4px;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-icon {
  background: transparent;
  padding: 4px 6px;
  border-radius: 4px;
  font-size: 14px;
}

.btn-icon:hover {
  background-color: #f1f5f9;
}

/* 卡片与网格 */
.connection-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 20px;
}

.connection-card {
  background: #ffffff;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 18px;
  box-shadow: var(--shadow-sm);
  display: flex;
  flex-direction: column;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 14px;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-title h3 {
  font-size: 16px;
  font-weight: 600;
}

.db-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: var(--success);
}

.card-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
}

.info-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.info-row .label {
  color: var(--text-muted);
  width: 80px;
  flex-shrink: 0;
}

.info-row .value {
  color: var(--text-main);
  word-break: break-all;
}

.font-bold {
  font-weight: 600;
}

.filter-tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.tag {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 4px;
  font-family: monospace;
}

.tag-include {
  background: #dbeafe;
  color: #1e40af;
}

.tag-exclude {
  background: #fee2e2;
  color: #991b1b;
}

.card-footer {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--border);
  display: flex;
  justify-content: space-between;
}

.test-result-box {
  margin-top: 10px;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12px;
}

.test-result-box.success {
  background-color: var(--success-light);
  color: #065f46;
}

.test-result-box.error {
  background-color: var(--danger-light);
  color: #991b1b;
}

.test-details {
  display: flex;
  gap: 12px;
  margin-top: 4px;
  font-size: 11px;
  opacity: 0.85;
}

/* 徽标 */
.badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 9999px;
  font-weight: 500;
}

.badge-id {
  background: #f1f5f9;
  color: #475569;
}

.badge-success {
  background: var(--success-light);
  color: #065f46;
}

.badge-warning {
  background: var(--warning-light);
  color: #92400e;
}

.badge-baseline {
  background: #fef3c7;
  color: #92400e;
  font-weight: 600;
}

.badge-snapshot {
  background: #e0e7ff;
  color: #3730a3;
}

/* 提示与空状态 */
.baseline-banner {
  background: #fffbeb;
  border: 1px solid #fde68a;
  border-radius: var(--radius);
  padding: 12px 16px;
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 20px;
  color: #92400e;
  font-size: 13px;
}

.empty-card, .notice-card {
  background: #ffffff;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
  padding: 40px 20px;
  text-align: center;
  color: var(--text-muted);
}

.empty-icon {
  font-size: 36px;
  margin-bottom: 8px;
}

.empty-card h3 {
  font-size: 16px;
  color: var(--text-main);
  margin-bottom: 6px;
}

.empty-card p {
  font-size: 13px;
  margin-bottom: 16px;
}

.table-container {
  background: #ffffff;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
  font-size: 13px;
}

.data-table th {
  background: #f8fafc;
  padding: 12px 16px;
  color: var(--text-muted);
  font-weight: 600;
  border-bottom: 1px solid var(--border);
}

.data-table td {
  padding: 12px 16px;
  border-bottom: 1px solid #f1f5f9;
}

.data-table tr:last-child td {
  border-bottom: none;
}

.action-buttons {
  display: flex;
  gap: 6px;
}

/* 比对控制面板 */
.diff-control-panel {
  background: #ffffff;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 20px;
  display: flex;
  align-items: flex-end;
  gap: 16px;
  box-shadow: var(--shadow-sm);
  margin-bottom: 24px;
}

.control-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.control-col label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
}

.select-input {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: #ffffff;
  font-size: 13px;
  color: var(--text-main);
}

.diff-arrow {
  font-size: 20px;
  color: var(--text-muted);
  padding-bottom: 8px;
}

.btn-diff {
  height: 38px;
  padding: 0 20px;
}

/* 比对统计卡片 */
.stats-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 20px;
}

.stat-card {
  background: #ffffff;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
  display: flex;
  gap: 14px;
  align-items: center;
  box-shadow: var(--shadow-sm);
}

.stat-icon {
  font-size: 32px;
}

.stat-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-main);
}

.stat-numbers {
  display: flex;
  gap: 8px;
  margin: 6px 0;
}

.stat-tag {
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 4px;
}

.stat-tag.add {
  background: #dcfce7;
  color: #15803d;
}

.stat-tag.mod {
  background: #fef3c7;
  color: #b45309;
}

.stat-tag.del {
  background: #fee2e2;
  color: #b91c1c;
}

.stat-sub {
  font-size: 11px;
  color: var(--text-muted);
}

/* SQL 代码查看卡片 */
.sql-viewer-card {
  background: #ffffff;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}

.sql-viewer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: #f8fafc;
  border-bottom: 1px solid var(--border);
}

.script-tabs {
  display: flex;
  gap: 6px;
}

.script-tab {
  padding: 6px 12px;
  border-radius: 4px;
  font-size: 12px;
  background: transparent;
  color: var(--text-muted);
}

.script-tab.active {
  background: #ffffff;
  color: var(--primary);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0,0,0,0.05);
}

.script-actions {
  display: flex;
  gap: 8px;
}

.sql-code-container {
  padding: 16px;
  max-height: 480px;
  overflow-y: auto;
  background: #1e293b;
}

.sql-code {
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

/* 模态弹窗 */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  backdrop-filter: blur(2px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.modal-dialog {
  background: #ffffff;
  border-radius: var(--radius);
  width: 600px;
  max-width: 90vw;
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}

.modal-dialog-sm {
  width: 440px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
}

.modal-header h3 {
  font-size: 16px;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  font-size: 18px;
  color: var(--text-muted);
}

.modal-body {
  padding: 20px;
  max-height: 70vh;
  overflow-y: auto;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
}

.form-group label {
  font-size: 12px;
  font-weight: 500;
  color: #475569;
}

.form-group input, .form-group textarea {
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  font-size: 13px;
}

.form-group input:focus, .form-group textarea:focus {
  border-color: var(--primary);
}

.form-divider {
  font-size: 12px;
  font-weight: 600;
  color: var(--primary);
  margin: 16px 0 10px;
  padding-bottom: 4px;
  border-bottom: 1px solid #e0e7ff;
}

.form-group-checkbox {
  margin: 12px 0;
}

.form-group-checkbox label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
}

.help-text {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 4px;
  padding-left: 20px;
}

.modal-footer {
  padding: 12px 20px;
  background: #f8fafc;
  border-top: 1px solid var(--border);
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

/* 消息 Toast */
.toast-notification {
  position: fixed;
  top: 20px;
  right: 24px;
  padding: 10px 18px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  z-index: 999;
  box-shadow: var(--shadow);
}

.toast-notification.info {
  background: #1e293b;
  color: #ffffff;
}

.toast-notification.success {
  background: #059669;
  color: #ffffff;
}

.toast-notification.error {
  background: #dc2626;
  color: #ffffff;
}

.toast-notification.warning {
  background: #d97706;
  color: #ffffff;
}

.loading-box {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 40px;
  color: var(--text-muted);
  font-size: 13px;
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid #cbd5e1;
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>

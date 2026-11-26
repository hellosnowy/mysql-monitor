<template>
  <section class="tab-content home-view">
    <!-- 顶部欢迎概览横幅 -->
    <div class="home-hero">
      <div class="home-hero-text">
        <h2>多数据库结构与数据变更监控中心</h2>
        <p>
          支持 MySQL、PostgreSQL、SQL Server 的结构 (DDL) 与数据 (DML) 快照捕获、基准版本 (Baseline) 管理，以及升级/回滚 SQL 脚本双向生成。
        </p>
      </div>
      <div class="home-hero-actions">
        <button type="button" class="btn btn-primary" @click="$emit('navigate', 'connections')">
          🔌 管理数据库连接
        </button>
        <button type="button" class="btn btn-outline" @click="$emit('navigate', 'diff')">
          ⚖️ 开始版本比对
        </button>
      </div>
    </div>

    <!-- 核心指标统计卡片 -->
    <div class="home-overview-grid">
      <div class="overview-card">
        <div class="overview-icon">🔌</div>
        <div>
          <div class="overview-label">已配置数据库连接</div>
          <div class="overview-value">{{ connections.length }}</div>
          <div class="overview-sub">{{ vendorBreakdownText }}</div>
        </div>
      </div>

      <div class="overview-card">
        <div class="overview-icon">📸</div>
        <div>
          <div class="overview-label">已捕获历史快照</div>
          <div class="overview-value">{{ loadingSnapshots ? '...' : totalSnapshotCount }}</div>
          <div class="overview-sub">包含 {{ loadingSnapshots ? '...' : totalBaselineCount }} 个基准版本 (Baseline)</div>
        </div>
      </div>

      <div class="overview-card">
        <div class="overview-icon">🧩</div>
        <div>
          <div class="overview-label">内置数据库方言</div>
          <div class="overview-value">{{ vendors.length }}</div>
          <div class="overview-sub">{{ driverNamesText }}</div>
        </div>
      </div>
    </div>

    <!-- 三步标准使用流程入口 -->
    <div class="home-section">
      <div class="home-section-header">
        <h3>核心工作流</h3>
      </div>
      <div class="workflow-grid">
        <div class="workflow-card">
          <div>
            <span class="workflow-step">步骤 1</span>
            <h4>配置与测试数据库连接</h4>
            <p>添加目标数据库实例，配置表级黑白名单与监控模式，实时探测服务端版本和网络延迟。</p>
          </div>
          <button type="button" class="btn btn-outline btn-sm" @click="$emit('navigate', 'connections')">
            进入连接管理 →
          </button>
        </div>

        <div class="workflow-card">
          <div>
            <span class="workflow-step">步骤 2</span>
            <h4>抓取基准与增量快照</h4>
            <p>自动确立首个基准版本 (Baseline)，按时间线记录每次发布前后的表结构与行数据状态。</p>
          </div>
          <button type="button" class="btn btn-outline btn-sm" @click="$emit('navigate', 'snapshots')">
            进入快照管理 →
          </button>
        </div>

        <div class="workflow-card">
          <div>
            <span class="workflow-step">步骤 3</span>
            <h4>双向比对并导出 SQL</h4>
            <p>任意选择两个历史版本，生成带事务保护的升级或回滚 SQL 脚本，支持复制与本地导出。</p>
          </div>
          <button type="button" class="btn btn-outline btn-sm" @click="$emit('navigate', 'diff')">
            进入差异比对 →
          </button>
        </div>
      </div>
    </div>

    <!-- 快捷连接列表与内置驱动概览 -->
    <div class="home-two-col">
      <div class="home-panel">
        <div class="home-section-header">
          <h3>数据库连接快捷入口</h3>
          <button type="button" class="btn btn-outline btn-xs" @click="$emit('navigate', 'connections')">
            查看全部
          </button>
        </div>

        <div v-if="loadingConnections" class="loading-box">
          <div class="spinner"></div> 正在加载连接概览...
        </div>

        <div v-else-if="connections.length === 0" class="empty-card">
          <div class="empty-icon">📭</div>
          <h3>暂无数据库连接</h3>
          <p>请先前往“数据库连接管理”添加您的第一个数据库实例</p>
          <button type="button" class="btn btn-primary btn-sm" @click="$emit('navigate', 'connections')">
            去添加连接
          </button>
        </div>

        <div v-else class="quick-conn-list">
          <div v-for="conn in connections" :key="conn.id" class="quick-conn-item">
            <div class="quick-conn-info">
              <div class="quick-conn-title">
                <span>{{ conn.name || conn.id }}</span>
                <span class="badge badge-vendor">{{ getVendorLabel(vendors, conn.vendor) }}</span>
              </div>
              <div class="quick-conn-meta">
                {{ conn.host }}:{{ conn.port }} / {{ conn.database }}
                <template v-if="conn.vendor && conn.vendor !== 'mysql'">
                  ({{ conn.schema || getDefaultSchema(conn.vendor) }})
                </template>
                · 快照 {{ snapshotStats[conn.id]?.total ?? 0 }} 个
              </div>
            </div>
            <div class="quick-conn-actions">
              <button type="button" class="btn btn-outline btn-xs" @click="$emit('go-snapshots', conn.id)">
                快照
              </button>
              <button type="button" class="btn btn-primary btn-xs" @click="$emit('go-diff', conn.id)">
                比对
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="home-panel">
        <div class="home-section-header">
          <h3>内置数据库方言与驱动</h3>
        </div>
        <div class="vendor-grid">
          <div v-for="v in vendors" :key="v.vendor" class="vendor-card">
            <div class="vendor-card-top">
              <span class="vendor-card-name">{{ v.display_name }}</span>
              <span class="badge badge-success">默认端口 {{ v.default_port }}</span>
            </div>
            <div class="vendor-card-pkg font-mono">
              {{ v.driver_package }} {{ v.driver_version }}
            </div>
            <div class="vendor-card-pkg">
              {{ v.supports_schemas ? `支持 Schema（默认 ${v.default_schema || '-'}）` : '以数据库名作为默认 Schema' }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { listSnapshots } from '../api/snapshot.js'
import { getDefaultSchema, getVendorLabel } from '../utils/vendor.js'
import '../styles/home.css'

// 首页视图：汇总连接、快照与方言概览并提供快捷工作流跳转
const props = defineProps({
  connections: {
    type: Array,
    default: () => [],
  },
  vendors: {
    type: Array,
    default: () => [],
  },
  loadingConnections: {
    type: Boolean,
    default: false,
  },
  snapshotRevision: {
    type: Number,
    default: 0,
  },
})

defineEmits(['navigate', 'go-snapshots', 'go-diff'])

const snapshotStats = reactive({})
const loadingSnapshots = ref(false)
let loadSeq = 0

// 异步聚合所有连接的快照数量与基准版本数量
const refreshSnapshotStats = async () => {
  const currentSeq = ++loadSeq
  const list = props.connections || []
  if (list.length === 0) {
    Object.keys(snapshotStats).forEach((key) => delete snapshotStats[key])
    loadingSnapshots.value = false
    return
  }

  loadingSnapshots.value = true
  try {
    const entries = await Promise.all(
      list.map(async (conn) => {
        try {
          const snaps = (await listSnapshots(conn.id)) || []
          const baselines = snaps.filter((s) => s.is_baseline).length
          return [conn.id, { total: snaps.length, baselines }]
        } catch {
          return [conn.id, { total: 0, baselines: 0 }]
        }
      }),
    )

    if (currentSeq !== loadSeq) return
    Object.keys(snapshotStats).forEach((key) => delete snapshotStats[key])
    for (const [id, stats] of entries) {
      snapshotStats[id] = stats
    }
  } finally {
    if (currentSeq === loadSeq) {
      loadingSnapshots.value = false
    }
  }
}

watch(
  () => [props.connections, props.snapshotRevision],
  () => {
    refreshSnapshotStats()
  },
  { immediate: true },
)

const totalSnapshotCount = computed(() =>
  Object.values(snapshotStats).reduce((sum, item) => sum + item.total, 0),
)

const totalBaselineCount = computed(() =>
  Object.values(snapshotStats).reduce((sum, item) => sum + item.baselines, 0),
)

const vendorBreakdownText = computed(() => {
  if (!props.connections.length) return '暂未配置数据库实例'
  const counts = { mysql: 0, postgres: 0, sqlserver: 0 }
  for (const conn of props.connections) {
    const vendor = conn.vendor || 'mysql'
    counts[vendor] = (counts[vendor] || 0) + 1
  }
  const parts = []
  if (counts.mysql) parts.push(`MySQL ${counts.mysql}`)
  if (counts.postgres) parts.push(`PostgreSQL ${counts.postgres}`)
  if (counts.sqlserver) parts.push(`SQL Server ${counts.sqlserver}`)
  return parts.join(' / ') || '已就绪'
})

const driverNamesText = computed(() =>
  props.vendors.length ? props.vendors.map((v) => v.display_name).join(' · ') : '加载中...',
)
</script>

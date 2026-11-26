<template>
  <section class="tab-content">
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
        <select v-model="diffConnId" class="select-input" @change="handleConnChange">
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
          <option v-for="s in diffSnapshots" :key="'from_' + s.version_id" :value="s.version_id">
            {{ s.version_id }} {{ s.is_baseline ? '[基准]' : '' }} - {{ s.description }}
          </option>
        </select>
      </div>

      <div class="diff-arrow">➔</div>

      <div class="control-col">
        <label>目标 / 新版本 (To)</label>
        <select v-model="toVersion" class="select-input" :disabled="!diffSnapshots || diffSnapshots.length === 0">
          <option value="" disabled>-- 请选择目标版本 --</option>
          <option v-for="s in diffSnapshots" :key="'to_' + s.version_id" :value="s.version_id">
            {{ s.version_id }} {{ s.is_baseline ? '[基准]' : '' }} - {{ s.description }}
          </option>
        </select>
      </div>

      <div class="control-col direction-col">
        <label>SQL 方向</label>
        <div class="direction-toggle">
          <button
            type="button"
            :class="['direction-btn', 'direction-upgrade', diffDirection === 'upgrade' ? 'active' : '']"
            @click="setDiffDirection('upgrade')"
          >
            <span :class="['direction-checkbox', diffDirection === 'upgrade' ? 'checked' : '']">
              <svg v-if="diffDirection === 'upgrade'" class="check-icon" viewBox="0 0 16 16" fill="currentColor">
                <path d="M13.485 3.515a1 1 0 0 1 0 1.414l-7 7a1 1 0 0 1-1.414 0l-3.5-3.5a1 1 0 1 1 1.414-1.414L5.778 10.515l6.293-6.293a1 1 0 0 1 1.414 0z"/>
              </svg>
            </span>
            <span class="direction-text">升级 From → To</span>
          </button>
          <button
            type="button"
            :class="['direction-btn', 'direction-rollback', diffDirection === 'rollback' ? 'active' : '']"
            @click="setDiffDirection('rollback')"
          >
            <span :class="['direction-checkbox', diffDirection === 'rollback' ? 'checked' : '']">
              <svg v-if="diffDirection === 'rollback'" class="check-icon" viewBox="0 0 16 16" fill="currentColor">
                <path d="M13.485 3.515a1 1 0 0 1 0 1.414l-7 7a1 1 0 0 1-1.414 0l-3.5-3.5a1 1 0 1 1 1.414-1.414L5.778 10.515l6.293-6.293a1 1 0 0 1 1.414 0z"/>
              </svg>
            </span>
            <span class="direction-text">回滚 To → From</span>
          </button>
        </div>
      </div>

      <div class="control-action">
        <button
          type="button"
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
      <div class="diff-result-meta">
        <span class="badge badge-vendor">{{ getVendorLabel(vendors, diffResult.vendor) }}</span>
        <span :class="['badge', diffResult.direction === 'rollback' ? 'badge-warning' : 'badge-success']">
          {{ diffResult.direction === 'rollback' ? '回滚脚本 (To → From)' : '升级脚本 (From → To)' }}
        </span>
      </div>

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
              type="button"
              :class="['script-tab', sqlTab === 'full' ? 'active' : '']"
              @click="sqlTab = 'full'"
            >
              完整同步脚本 (带事务保护)
            </button>
            <button
              type="button"
              :class="['script-tab', sqlTab === 'ddl' ? 'active' : '']"
              @click="sqlTab = 'ddl'"
            >
              仅 DDL 结构变动
            </button>
            <button
              type="button"
              :class="['script-tab', sqlTab === 'dml' ? 'active' : '']"
              @click="sqlTab = 'dml'"
            >
              仅 DML 数据变动
            </button>
          </div>

          <div class="script-actions">
            <button type="button" class="btn btn-secondary btn-sm" @click="copyActiveSQL">
              📋 复制当前 SQL
            </button>
            <button type="button" class="btn btn-success btn-sm" @click="exportSQL">
              💾 导出 .sql 文件
            </button>
          </div>
        </div>

        <div class="sql-code-container">
          <pre class="sql-code" contenteditable="false"><code contenteditable="false">{{ activeSQLContent || '-- 经比对，选定的两个版本之间未检测到变动差异 --' }}</code></pre>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { compareVersions, exportSQLFile } from '../api/diff.js'
import { listSnapshots } from '../api/snapshot.js'
import { useToast } from '../composables/useToast.js'
import { formatError } from '../utils/format.js'
import { getVendorLabel } from '../utils/vendor.js'
import '../styles/diff.css'

// 历史版本差异比对视图：负责版本选择、双向比对、SQL 预览与导出
const props = defineProps({
  connections: {
    type: Array,
    default: () => [],
  },
  vendors: {
    type: Array,
    default: () => [],
  },
  target: {
    type: Object,
    default: null,
  },
  snapshotRevision: {
    type: Number,
    default: 0,
  },
  lastDeletedSnapshot: {
    type: Object,
    default: null,
  },
})

const { showToast } = useToast()

const diffConnId = ref('')
const diffSnapshots = ref([])
const fromVersion = ref('')
const toVersion = ref('')
const comparing = ref(false)
const diffResult = ref(null)
const sqlTab = ref('full')
const diffDirection = ref('upgrade')

const setDiffDirection = (dir) => {
  if (diffDirection.value === dir) return
  diffDirection.value = dir
  diffResult.value = null
}

// 加载指定连接的可比对快照版本并初始化起止默认值
const loadDiffSnapshots = async () => {
  if (!diffConnId.value) return
  try {
    const list = await listSnapshots(diffConnId.value)
    diffSnapshots.value = list || []
    if (!list || list.length < 2) {
      fromVersion.value = ''
      toVersion.value = ''
      diffResult.value = null
    } else {
      if (!fromVersion.value || !list.find((s) => s.version_id === fromVersion.value)) {
        fromVersion.value = list[list.length - 1].version_id
      }
      if (!toVersion.value || !list.find((s) => s.version_id === toVersion.value)) {
        toVersion.value = list[0].version_id
      }
    }
  } catch (err) {
    diffSnapshots.value = []
    showToast('加载比对快照列表失败: ' + formatError(err), 'error')
  }
}

const handleConnChange = () => {
  fromVersion.value = ''
  toVersion.value = ''
  diffResult.value = null
  loadDiffSnapshots()
}

// 接收外部跳转目标（如从首页或快照列表带入连接和起始版本）
watch(
  () => props.target,
  (target) => {
    if (!target || !target.connId) return
    diffConnId.value = target.connId
    fromVersion.value = target.fromVersionId || ''
    toVersion.value = ''
    diffResult.value = null
    loadDiffSnapshots()
  },
  { immediate: true },
)

// 快照增删后同步刷新比对页可用版本列表
watch(
  () => props.snapshotRevision,
  () => {
    if (!diffConnId.value) return
    loadDiffSnapshots()
  },
)

// 若被删除的快照正好处于当前比对选择中，清理选择状态与结果
watch(
  () => props.lastDeletedSnapshot,
  (deleted) => {
    if (!deleted || deleted.connId !== diffConnId.value || !deleted.versionId) return
    if (fromVersion.value === deleted.versionId) fromVersion.value = ''
    if (toVersion.value === deleted.versionId) toVersion.value = ''
    diffResult.value = null
  },
)

// 若当前比对连接已被删除，重置整个比对面板
watch(
  () => props.connections,
  (list) => {
    if (!diffConnId.value) return
    if (!list.find((c) => c.id === diffConnId.value)) {
      diffConnId.value = ''
      diffSnapshots.value = []
      fromVersion.value = ''
      toVersion.value = ''
      diffResult.value = null
    }
  },
)

const runDiff = async () => {
  if (!diffConnId.value || !fromVersion.value || !toVersion.value) return
  if (fromVersion.value === toVersion.value) {
    showToast('起始版本和目标版本不能相同', 'warning')
    return
  }

  comparing.value = true
  try {
    diffResult.value = await compareVersions(
      diffConnId.value,
      fromVersion.value,
      toVersion.value,
      diffDirection.value,
    )
    showToast(diffDirection.value === 'rollback' ? '回滚脚本生成完成' : '升级脚本生成完成', 'success')
  } catch (err) {
    showToast('比对失败: ' + formatError(err), 'error')
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
  } catch {
    showToast('复制失败，请手动选择复制', 'error')
  }
}

const exportSQL = async () => {
  if (!activeSQLContent.value) return
  const defaultName = `diff_${diffConnId.value}_${diffDirection.value}_${fromVersion.value}_to_${toVersion.value}.sql`
  try {
    const savedPath = await exportSQLFile(defaultName, activeSQLContent.value)
    if (savedPath) {
      showToast(`已成功导出至: ${savedPath}`, 'success')
    }
  } catch (err) {
    showToast('导出文件失败: ' + formatError(err), 'error')
  }
}
</script>

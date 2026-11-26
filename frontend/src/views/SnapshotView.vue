<template>
  <section class="tab-content">
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
          type="button"
          class="btn btn-primary"
          :disabled="!selectedConnId"
          @click="openCaptureModal"
        >
          📸 抓取当前快照
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
                  <button type="button" class="btn btn-xs btn-outline" @click="selectForDiff(snap.version_id)">
                    比对
                  </button>
                  <button type="button" class="btn btn-xs btn-danger" @click="delSnapshot(snap.version_id)">
                    删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 快照抓取参数弹窗 -->
    <CaptureSnapshotModal
      :visible="showCaptureModal"
      :conn-id="selectedConnId"
      :default-force-baseline="defaultForceBaseline"
      @close="showCaptureModal = false"
      @captured="handleCaptured"
    />
  </section>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { deleteSnapshot, listSnapshots } from '../api/snapshot.js'
import CaptureSnapshotModal from '../components/CaptureSnapshotModal.vue'
import { useToast } from '../composables/useToast.js'
import { formatDate, formatError } from '../utils/format.js'
import '../styles/snapshots.css'

// 快照与基准版本管理视图：负责历史版本查看、抓取与删除
const props = defineProps({
  connections: {
    type: Array,
    default: () => [],
  },
  initialConnId: {
    type: String,
    default: '',
  },
})

const emit = defineEmits(['go-diff', 'snapshots-changed'])
const { showToast } = useToast()

const selectedConnId = ref('')
const snapshots = ref([])
const loadingSnapshots = ref(false)
const showCaptureModal = ref(false)

// 当前连接尚无历史快照时，弹窗默认勾选基准版本
const defaultForceBaseline = computed(() => !snapshots.value || snapshots.value.length === 0)

const loadSnapshots = async () => {
  if (!selectedConnId.value) return
  loadingSnapshots.value = true
  try {
    const list = await listSnapshots(selectedConnId.value)
    snapshots.value = list || []
  } catch (err) {
    snapshots.value = []
    showToast('获取快照失败: ' + formatError(err), 'error')
  } finally {
    loadingSnapshots.value = false
  }
}

// 接收外部模块传入的目标连接 ID 并自动拉取快照
watch(
  () => props.initialConnId,
  (connId) => {
    if (!connId) return
    selectedConnId.value = connId
    loadSnapshots()
  },
  { immediate: true },
)

// 若当前选中的连接被删除，清理本地状态
watch(
  () => props.connections,
  (list) => {
    if (!selectedConnId.value) return
    if (!list.find((c) => c.id === selectedConnId.value)) {
      selectedConnId.value = ''
      snapshots.value = []
    }
  },
)

const openCaptureModal = () => {
  showCaptureModal.value = true
}

const handleCaptured = async () => {
  await loadSnapshots()
  emit('snapshots-changed', { connId: selectedConnId.value, deletedVersionId: '' })
}

const delSnapshot = async (verId) => {
  if (!confirm(`确定要删除快照版本 "${verId}" 吗？`)) return
  try {
    await deleteSnapshot(selectedConnId.value, verId)
    showToast('删除快照成功', 'success')
    await loadSnapshots()
    emit('snapshots-changed', { connId: selectedConnId.value, deletedVersionId: verId })
  } catch (err) {
    showToast('删除失败: ' + formatError(err), 'error')
  }
}

const selectForDiff = (verId) => {
  emit('go-diff', {
    connId: selectedConnId.value,
    fromVersionId: verId,
  })
}
</script>

<template>
  <div class="app-container">
    <!-- 顶部导航栏 -->
    <AppHeader v-model:active-tab="activeTab" />

    <!-- 主体内容区 -->
    <main class="app-main">
      <!-- 全局提示消息通知框 -->
      <ToastNotification />

      <!-- 1. 首页 -->
      <HomeView
        v-if="activeTab === 'home'"
        :connections="connections"
        :vendors="vendors"
        :loading-connections="loadingConnections"
        :snapshot-revision="snapshotRevision"
        @navigate="activeTab = $event"
        @go-snapshots="goToSnapshots"
        @go-diff="goToDiff($event)"
      />

      <!-- 2. 数据库连接管理 -->
      <ConnectionView
        v-if="activeTab === 'connections'"
        :connections="connections"
        :vendors="vendors"
        :loading="loadingConnections"
        @refresh="loadConnections"
        @go-snapshots="goToSnapshots"
        @connection-deleted="handleConnectionDeleted"
      />

      <!-- 3. 快照与基准版本管理 -->
      <SnapshotView
        v-if="activeTab === 'snapshots'"
        :connections="connections"
        :initial-conn-id="snapshotConnId"
        @go-diff="goToDiff($event.connId, $event.fromVersionId)"
        @snapshots-changed="handleSnapshotsChanged"
      />

      <!-- 4. 历史版本差异比对 -->
      <DiffView
        v-if="activeTab === 'diff'"
        :connections="connections"
        :vendors="vendors"
        :target="diffTarget"
        :snapshot-revision="snapshotRevision"
        :last-deleted-snapshot="lastDeletedSnapshot"
      />
    </main>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { getConnections } from './api/connection.js'
import { getVendors } from './api/vendor.js'
import AppHeader from './components/AppHeader.vue'
import ToastNotification from './components/ToastNotification.vue'
import { useToast } from './composables/useToast.js'
import { formatError } from './utils/format.js'
import ConnectionView from './views/ConnectionView.vue'
import DiffView from './views/DiffView.vue'
import HomeView from './views/HomeView.vue'
import SnapshotView from './views/SnapshotView.vue'

// 主应用壳：管理全局共享的连接/方言数据与四模块页面路由
const { showToast } = useToast()

const activeTab = ref('home')
const connections = ref([])
const vendors = ref([])
const loadingConnections = ref(false)

const snapshotConnId = ref('')
const snapshotRevision = ref(0)
const lastDeletedSnapshot = ref(null)
const diffTarget = ref(null)
let actionSeq = 0

// 加载内置数据库方言列表
const loadVendors = async () => {
  try {
    vendors.value = (await getVendors()) || []
  } catch (err) {
    showToast('获取内置驱动信息失败: ' + formatError(err), 'error')
  }
}

// 加载已保存的数据库连接列表
const loadConnections = async () => {
  loadingConnections.value = true
  try {
    connections.value = (await getConnections()) || []
  } catch (err) {
    showToast('获取连接列表失败: ' + formatError(err), 'error')
  } finally {
    loadingConnections.value = false
  }
}

// 跳转到指定连接的快照管理页
const goToSnapshots = (connId) => {
  snapshotConnId.value = connId
  activeTab.value = 'snapshots'
}

// 跳转到历史版本差异比对页（可选预设起始版本）
const goToDiff = (connId, fromVersionId = '') => {
  diffTarget.value = {
    connId,
    fromVersionId,
    seq: ++actionSeq,
  }
  activeTab.value = 'diff'
}

// 同步快照变更事件至首页统计与比对视图
const handleSnapshotsChanged = ({ connId, deletedVersionId }) => {
  snapshotRevision.value += 1
  if (deletedVersionId) {
    lastDeletedSnapshot.value = {
      connId,
      versionId: deletedVersionId,
      seq: ++actionSeq,
    }
  }
}

// 清理被删除连接关联的跳转缓存
const handleConnectionDeleted = (connId) => {
  if (snapshotConnId.value === connId) {
    snapshotConnId.value = ''
  }
  if (diffTarget.value && diffTarget.value.connId === connId) {
    diffTarget.value = null
  }
  snapshotRevision.value += 1
}

onMounted(() => {
  loadVendors()
  loadConnections()
})
</script>

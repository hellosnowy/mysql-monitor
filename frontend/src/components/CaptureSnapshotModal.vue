<template>
  <div v-if="visible" class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-dialog modal-dialog-sm">
      <div class="modal-header">
        <h3>抓取当前数据库快照</h3>
        <button type="button" class="close-btn" @click="$emit('close')">✕</button>
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
            <input v-model="captureForceBaseline" type="checkbox" />
            <span>标记为新的基准版本 (Baseline)</span>
          </label>
          <p class="help-text">选中后，后续版本比对可直接将此快照作为新基准起点</p>
        </div>
      </div>

      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" @click="$emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="capturing" @click="doCapture">
          {{ capturing ? '正在抓取中...' : '确认提取' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { captureSnapshot } from '../api/snapshot.js'
import { useToast } from '../composables/useToast.js'
import { formatError } from '../utils/format.js'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  connId: {
    type: String,
    default: '',
  },
  defaultForceBaseline: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['close', 'captured'])
const { showToast } = useToast()

const captureDesc = ref('')
const captureForceBaseline = ref(false)
const capturing = ref(false)

// 每次打开弹窗时重置备注并根据当前连接是否已有快照决定默认基线勾选状态
watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    captureDesc.value = ''
    captureForceBaseline.value = props.defaultForceBaseline
  },
  { immediate: true },
)

// 执行后端快照提取并通知父视图刷新列表
const doCapture = async () => {
  if (!props.connId) return
  capturing.value = true
  try {
    const meta = await captureSnapshot(
      props.connId,
      captureForceBaseline.value,
      captureDesc.value,
    )
    showToast(`快照抓取成功 (${meta.version_id})`, 'success')
    emit('captured', meta)
    emit('close')
  } catch (err) {
    showToast('抓取失败: ' + formatError(err), 'error')
  } finally {
    capturing.value = false
  }
}
</script>

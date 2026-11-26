<template>
  <div v-if="visible" class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-dialog">
      <div class="modal-header">
        <h3>{{ isEditing ? '编辑数据库连接' : '新建数据库连接' }}</h3>
        <button type="button" class="close-btn" @click="$emit('close')">✕</button>
      </div>

      <div class="modal-body">
        <div class="form-grid">
          <div class="form-group">
            <label>数据库厂商 *</label>
            <select
              v-model="connForm.vendor"
              class="select-input"
              :disabled="vendorLocked"
              @change="applyVendorDefaults"
            >
              <option v-for="v in vendors" :key="v.vendor" :value="v.vendor">
                {{ v.display_name }}
              </option>
            </select>
            <div v-if="currentVendorInfo" class="driver-hint">
              <span class="badge badge-success">内置驱动</span>
              <span>{{ currentVendorInfo.driver_package }} {{ currentVendorInfo.driver_version }}</span>
            </div>
            <div v-if="vendorLocked" class="driver-hint">已有历史快照，厂商不可修改</div>
          </div>

          <div class="form-group">
            <label>连接标识 ID *</label>
            <input
              v-model="connForm.id"
              :disabled="isEditing"
              type="text"
              placeholder="如 dev_order_db"
            />
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
              :placeholder="isEditing ? '•••••••• (留空保留原密码)' : '请输入数据库密码'"
            />
          </div>

          <div class="form-group">
            <label>目标数据库 *</label>
            <input v-model="connForm.database" type="text" placeholder="mall" />
          </div>

          <div v-if="connForm.vendor === 'mysql'" class="form-group">
            <label>字符集</label>
            <input v-model="connForm.charset" type="text" placeholder="utf8mb4" />
          </div>

          <div v-else class="form-group">
            <label>Schema / 模式 *</label>
            <div class="input-with-action">
              <input
                v-model="connForm.schema"
                type="text"
                :placeholder="getDefaultSchema(connForm.vendor)"
              />
              <button
                class="btn btn-outline btn-sm"
                type="button"
                :disabled="loadingSchemas"
                @click="loadSchemas"
              >
                {{ loadingSchemas ? '加载中...' : '加载列表' }}
              </button>
            </div>
            <select v-if="availableSchemas.length" v-model="connForm.schema" class="select-input schema-select">
              <option v-for="s in availableSchemas" :key="s" :value="s">{{ s }}</option>
            </select>
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
        <button type="button" class="btn btn-secondary" @click="$emit('close')">取消</button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="submitConnection">
          {{ saving ? '保存中...' : '保存配置' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { saveConnection } from '../api/connection.js'
import { listSnapshots } from '../api/snapshot.js'
import { listSchemas } from '../api/vendor.js'
import { useToast } from '../composables/useToast.js'
import { formatError, parseTableList } from '../utils/format.js'
import { VENDOR_DEFAULTS, getDefaultSchema } from '../utils/vendor.js'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  editingConnection: {
    type: Object,
    default: null,
  },
  vendors: {
    type: Array,
    default: () => [],
  },
})

const emit = defineEmits(['close', 'saved'])
const { showToast } = useToast()

// 构造空白连接配置默认值
const createDefaultConnForm = () => ({
  id: '',
  name: '',
  vendor: 'mysql',
  host: '127.0.0.1',
  port: 3306,
  user: 'root',
  password: '',
  database: '',
  schema: '',
  charset: 'utf8mb4',
  ssl_mode: '',
  timeout_seconds: 10,
  filter: {
    include_tables: [],
    exclude_tables: [],
    monitor_mode: 'schema_and_data',
    max_rows_per_table: 0,
  },
})

const connForm = reactive(createDefaultConnForm())
const includeTablesText = ref('')
const excludeTablesText = ref('')
const availableSchemas = ref([])
const loadingSchemas = ref(false)
const vendorLocked = ref(false)
const saving = ref(false)

const isEditing = computed(() => Boolean(props.editingConnection))
const currentVendorInfo = computed(
  () => props.vendors.find((v) => v.vendor === connForm.vendor) || null,
)

// 根据弹窗打开状态与编辑对象初始化表单内容
watch(
  () => [props.visible, props.editingConnection],
  ([visible]) => {
    if (!visible) return

    availableSchemas.value = []
    vendorLocked.value = false

    if (props.editingConnection) {
      const cloned = JSON.parse(JSON.stringify(props.editingConnection))
      Object.assign(connForm, createDefaultConnForm(), cloned)
      if (!connForm.filter) {
        connForm.filter = createDefaultConnForm().filter
      }
      if (!connForm.vendor) connForm.vendor = 'mysql'
      if (!connForm.schema) connForm.schema = getDefaultSchema(connForm.vendor)
      connForm.password = '' // 留空表示保留后端已加密存储的原密码
      includeTablesText.value = (connForm.filter.include_tables || []).join(', ')
      excludeTablesText.value = (connForm.filter.exclude_tables || []).join(', ')

      // 已有历史快照的连接禁止修改数据库厂商，避免跨方言快照无法比对
      listSnapshots(connForm.id)
        .then((list) => {
          vendorLocked.value = (list || []).length > 0
        })
        .catch(() => {})
      return
    }

    Object.assign(connForm, createDefaultConnForm())
    includeTablesText.value = ''
    excludeTablesText.value = ''
  },
  { immediate: true },
)

// 切换数据库厂商时自动回填默认端口、Schema 与字符集
const applyVendorDefaults = () => {
  const defaults = VENDOR_DEFAULTS[connForm.vendor] || VENDOR_DEFAULTS.mysql
  connForm.port = defaults.port
  connForm.schema = defaults.schema
  connForm.charset = defaults.charset
  availableSchemas.value = []
}

// 从目标数据库拉取可用 Schema 列表
const loadSchemas = async () => {
  if (!connForm.host || !connForm.user || !connForm.database) {
    showToast('请先填写主机、用户名和目标数据库', 'warning')
    return
  }
  loadingSchemas.value = true
  try {
    const list = await listSchemas(JSON.parse(JSON.stringify(connForm)))
    availableSchemas.value = list || []
    if (!connForm.schema && availableSchemas.value.length > 0) {
      connForm.schema = availableSchemas.value[0]
    }
    showToast(`已加载 ${availableSchemas.value.length} 个 Schema`, 'success')
  } catch (err) {
    showToast('加载 Schema 失败: ' + formatError(err), 'error')
  } finally {
    loadingSchemas.value = false
  }
}

// 校验并保存数据库连接配置
const submitConnection = async () => {
  if (!connForm.id || !connForm.host || !connForm.user || !connForm.database) {
    showToast('请完整填写必填字段', 'error')
    return
  }
  if (connForm.vendor !== 'mysql' && !connForm.schema) {
    showToast('请填写 Schema / 模式', 'error')
    return
  }

  connForm.filter.include_tables = parseTableList(includeTablesText.value)
  connForm.filter.exclude_tables = parseTableList(excludeTablesText.value)

  saving.value = true
  try {
    await saveConnection(JSON.parse(JSON.stringify(connForm)))
    showToast('保存连接配置成功', 'success')
    emit('saved')
    emit('close')
  } catch (err) {
    showToast('保存失败: ' + formatError(err), 'error')
  } finally {
    saving.value = false
  }
}
</script>

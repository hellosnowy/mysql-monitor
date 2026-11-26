<template>
  <header class="app-header">
    <div class="header-brand">
      <img class="logo-image" src="/icon.svg" alt="SQL Monitor" />
      <div class="brand-text">
        <h1>SQL Monitor</h1>
        <span class="sub-title">多数据库 DDL &amp; DML 监控与版本比对工具</span>
      </div>
    </div>

    <nav class="nav-tabs">
      <button
        v-for="tab in navItems"
        :key="tab.key"
        type="button"
        :class="['nav-tab', activeTab === tab.key ? 'active' : '']"
        @click="$emit('update:activeTab', tab.key)"
      >
        <span class="tab-icon">{{ tab.icon }}</span>
        {{ tab.label }}
      </button>
    </nav>
  </header>
</template>

<script setup>
// 顶部导航栏：仅负责品牌展示与四个主功能模块切换
defineProps({
  activeTab: {
    type: String,
    required: true,
  },
})

defineEmits(['update:activeTab'])

const navItems = [
  { key: 'home', icon: '🏠', label: '首页' },
  { key: 'connections', icon: '🔌', label: '数据库连接管理' },
  { key: 'snapshots', icon: '📸', label: '快照与基准版本' },
  { key: 'diff', icon: '⚖️', label: '历史版本差异比对' },
]
</script>

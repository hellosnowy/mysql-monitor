<template>
  <div v-if="toast.visible" :class="['toast-notification', toast.type]">
    <span>{{ toast.message }}</span>
  </div>
</template>

<script setup>
import { useToast } from '../composables/useToast.js'

// 全局消息提示组件
const { toast } = useToast()
</script>

<template>
  <div class="space-y-2">
    <p class="text-sm text-gray-500">隔离预览：脚本仅在无来源权限的 sandbox 内运行，外部网络、表单、弹窗和顶层导航均禁止。请人工比较图形、指令遵循和动画。</p>
    <iframe v-if="source" title="观察实验产物" sandbox="allow-scripts" referrerpolicy="no-referrer" :srcdoc="document" class="h-[520px] w-full rounded border bg-white" />
    <details><summary class="cursor-pointer">原始产物</summary><pre class="max-h-96 overflow-auto whitespace-pre-wrap break-all text-xs">{{ source }}</pre></details>
    <button class="btn btn-secondary" @click="download">下载原始文本</button>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { buildOperationsPreview } from '@/utils/operationsPreview'
const props = defineProps<{ source: string; id: string }>()
// The app's own CSP nonce is also required by srcdoc's inherited policy.
const nonce = window.document.head.querySelector<HTMLScriptElement>('script[nonce]')?.nonce || ''
const document = computed(() => buildOperationsPreview(props.source, nonce))
function download() { const url = URL.createObjectURL(new Blob([props.source], { type: 'text/plain;charset=utf-8' })); const a = window.document.createElement('a'); a.href = url; a.download = `${props.id}.txt`; a.click(); URL.revokeObjectURL(url) }
</script>

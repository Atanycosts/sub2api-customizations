<template>
  <iframe
    v-if="preview.hasStaticVisual"
    :title="title"
    sandbox=""
    referrerpolicy="no-referrer"
    :srcdoc="preview.document"
    class="h-full w-full border-0 bg-white"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { buildOperationsStaticPreview } from '@/utils/operationsPreview'

const props = defineProps<{
  source: string
  title: string
}>()

const preview = computed(() => buildOperationsStaticPreview(props.source))
</script>

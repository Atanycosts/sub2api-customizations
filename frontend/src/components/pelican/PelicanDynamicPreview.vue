<template>
  <div class="space-y-2">
    <p v-if="showNotice" class="text-xs leading-5 text-gray-500 dark:text-dark-300">
      {{ t('pelican.preview.isolationNotice') }}
    </p>
    <div class="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700">
      <iframe
        :title="title"
        sandbox="allow-scripts"
        referrerpolicy="no-referrer"
        :srcdoc="document"
        class="h-[min(68vh,720px)] min-h-[360px] w-full border-0 bg-white"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { buildOperationsPreview } from '@/utils/operationsPreview'

const props = defineProps<{
  source: string
  title: string
  showNotice?: boolean
}>()

const { t } = useI18n()
const showNotice = computed(() => props.showNotice !== false)
const nonce = window.document.head.querySelector<HTMLScriptElement>('script[nonce]')?.nonce || ''
const document = computed(() => buildOperationsPreview(props.source, nonce))
</script>

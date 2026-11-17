<template>
  <BaseDialog
    :show="show"
    :title="t('pelican.detail.title')"
    width="extra-wide"
    close-on-click-outside
    @close="$emit('close')"
  >
    <div v-if="loading" class="flex min-h-72 items-center justify-center">
      <LoadingSpinner size="lg" />
    </div>
    <p v-else-if="error" role="alert" class="rounded-2xl bg-rose-50 p-4 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
      {{ error }}
    </p>
    <div v-else-if="run" class="space-y-5">
      <div class="flex flex-wrap items-center gap-2">
        <span
          class="inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-bold"
          :class="pelicanStateClass(run.state)"
        >
          {{ t(`pelican.state.${run.state}`) }}
        </span>
        <span class="text-xs text-gray-500 dark:text-dark-300">{{ formatDate(run.created_at) }}</span>
        <span class="text-xs text-gray-500 dark:text-dark-300">·</span>
        <span class="text-xs tabular-nums text-gray-500 dark:text-dark-300">{{ formatPelicanDuration(run.duration_ms) }}</span>
      </div>

      <PelicanRunFacts :run="run" />

      <PelicanDynamicPreview
        v-if="run.output"
        :source="run.output"
        :title="t('pelican.preview.detailTitle', { model: run.model })"
      />
      <div v-else class="rounded-2xl border border-dashed border-gray-300 bg-gray-50 p-8 text-center dark:border-dark-600 dark:bg-dark-900/40">
        <p class="font-semibold text-gray-800 dark:text-dark-100">{{ t('pelican.preview.noOutput') }}</p>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
          {{ run.error_kind ? t(`pelican.errorKind.${run.error_kind}`) : t('pelican.preview.dynamicOnly') }}
        </p>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PelicanDynamicPreview from './PelicanDynamicPreview.vue'
import PelicanRunFacts from './PelicanRunFacts.vue'
import { getPelicanResult } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatPelicanDuration, pelicanStateClass } from './pelicanFormat'
import type { PelicanPublicRunDetail } from '@/types/pelican'

const props = defineProps<{
  show: boolean
  runId: string | null
}>()

defineEmits<{ close: [] }>()

const { t } = useI18n()
const run = ref<PelicanPublicRunDetail | null>(null)
const loading = ref(false)
const error = ref('')
let controller: AbortController | null = null
let requestSequence = 0

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function load() {
  const requestID = ++requestSequence
  controller?.abort()
  const current = new AbortController()
  controller = current
  run.value = null
  error.value = ''
  loading.value = false
  if (!props.show || !props.runId) return
  loading.value = true
  try {
    const result = await getPelicanResult(props.runId, { signal: current.signal })
    if (requestID === requestSequence) run.value = result
  } catch (cause) {
    if (requestID === requestSequence && !current.signal.aborted) {
      error.value = extractApiErrorMessage(cause, t('pelican.detail.loadFailed'))
    }
  } finally {
    if (requestID === requestSequence) loading.value = false
  }
}

watch(() => [props.show, props.runId] as const, () => void load(), { immediate: true })
onBeforeUnmount(() => {
  requestSequence += 1
  controller?.abort()
})
</script>

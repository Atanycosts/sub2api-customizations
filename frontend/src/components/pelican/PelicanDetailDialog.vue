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
    <p
      v-else-if="error"
      role="alert"
      class="rounded-2xl bg-rose-50 p-4 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300"
    >
      {{ error }}
    </p>
    <div v-else-if="run" class="space-y-3">
      <div class="flex flex-wrap items-center gap-2">
        <span
          class="inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-bold"
          :class="pelicanStateClass(run.state)"
        >
          {{ t(`pelican.state.${run.state}`) }}
        </span>
        <span class="text-xs text-gray-500 dark:text-dark-300">
          {{ formatDate(run.created_at) }}
        </span>
        <span class="text-xs text-gray-500 dark:text-dark-300">·</span>
        <span class="text-xs tabular-nums text-gray-500 dark:text-dark-300">
          {{
            run.state === 'running'
              ? t('pelican.durationPending')
              : formatPelicanDuration(run.duration_ms)
          }}
        </span>
      </div>

      <div class="min-w-0">
        <p
          class="truncate text-sm font-semibold text-gray-900 dark:text-white"
          :title="run.group_name"
        >
          {{ run.group_name }}
        </p>
        <p class="mt-1 break-words font-mono text-xs text-gray-600 dark:text-dark-200">
          {{ run.model }}
        </p>
        <dl class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-dark-300">
          <div class="flex gap-1">
            <dt>{{ t('pelican.fields.effort') }}</dt>
            <dd class="font-medium text-gray-800 dark:text-dark-100">
              {{ run.effort || t('pelican.effort.unspecified') }}
            </dd>
          </div>
          <div class="flex gap-1">
            <dt>{{ t('pelican.fields.protocol') }}</dt>
            <dd class="font-medium text-gray-800 dark:text-dark-100">
              {{ t(`pelican.protocol.${run.protocol}`) }}
            </dd>
          </div>
        </dl>
      </div>

      <PelicanDynamicPreview
        v-if="run.output"
        :show-notice="false"
        :source="run.output"
        :title="t('pelican.preview.detailTitle', { model: run.model })"
      />
      <div
        v-else
        class="rounded-2xl border border-dashed border-gray-300 bg-gray-50 p-8 text-center dark:border-dark-600 dark:bg-dark-900/40"
      >
        <p class="font-semibold text-gray-800 dark:text-dark-100">
          {{ t('pelican.preview.noOutput') }}
        </p>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">
          {{
            run.error_kind
              ? t(`pelican.errorKind.${run.error_kind}`)
              : t('pelican.preview.dynamicOnly')
          }}
        </p>
      </div>
      <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-dark-300">
        <span>{{ t('pelican.page.promptVersion', { version: run.prompt_version }) }}</span>
        <span>
          {{ t('pelican.fields.outputCap') }} ·
          {{ t('pelican.fields.tokens', { count: run.max_output_tokens }) }}
        </span>
      </div>
      <p v-if="run.output" class="text-xs leading-5 text-gray-500 dark:text-dark-300">
        {{ t('pelican.preview.isolationNotice') }}
      </p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PelicanDynamicPreview from './PelicanDynamicPreview.vue'
import { usePelicanRefresh } from './usePelicanRefresh'
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
const refresh = usePelicanRefresh(
  () => void load(true),
  () => props.show && run.value?.state === 'running' ? 10_000 : null,
)

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function load(background = false) {
  refresh.clear()
  controller?.abort()
  if (!background) {
    run.value = null
    loading.value = false
  }
  error.value = ''
  if (!props.show || !props.runId) return
  const current = new AbortController()
  controller = current
  if (!background) loading.value = true
  try {
    const result = await getPelicanResult(props.runId, { signal: current.signal })
    if (!current.signal.aborted) run.value = result
  } catch (cause) {
    if (!current.signal.aborted) {
      error.value = extractApiErrorMessage(cause, t('pelican.detail.loadFailed'))
    }
  } finally {
    if (!current.signal.aborted) {
      loading.value = false
      refresh.schedule()
    }
  }
}

watch(
  () => [props.show, props.runId] as const,
  () => void load(),
  { immediate: true }
)
onBeforeUnmount(() => controller?.abort())
</script>

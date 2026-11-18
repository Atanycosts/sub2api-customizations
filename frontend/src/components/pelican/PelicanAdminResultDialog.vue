<template>
  <BaseDialog
    :show="show"
    :title="t('pelican.admin.results.detailTitle')"
    width="extra-wide"
    close-on-click-outside
    @close="$emit('close')"
  >
    <div v-if="loading" class="flex min-h-80 items-center justify-center">
      <LoadingSpinner size="lg" />
    </div>
    <p v-else-if="error" role="alert" class="rounded-2xl bg-rose-50 p-4 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
      {{ error }}
    </p>
    <div v-else-if="run" class="space-y-5">
      <div class="flex flex-wrap items-center gap-2">
        <span
          class="rounded-full border px-2.5 py-1 text-xs font-bold"
          :class="pelicanStateClass(run.state)"
        >
          {{ t(`pelican.state.${run.state}`) }}
        </span>
        <span class="font-mono text-xs text-gray-500 dark:text-dark-300">{{ run.id }}</span>
      </div>

      <PelicanRunFacts :run="run" />

      <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900/40">
          <p class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.plan') }}</p>
          <p class="mt-1 font-semibold text-gray-900 dark:text-white">#{{ run.plan_id }}</p>
        </div>
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900/40">
          <p class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.account') }}</p>
          <p class="mt-1 font-semibold text-gray-900 dark:text-white">#{{ run.account_id }}</p>
        </div>
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900/40">
          <p class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.reserved') }}</p>
          <p class="mt-1 font-semibold tabular-nums text-gray-900 dark:text-white">${{ run.reserved_usd.toFixed(6) }}</p>
        </div>
        <div class="rounded-2xl bg-gray-50 p-3 dark:bg-dark-900/40">
          <p class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.cost') }}</p>
          <p class="mt-1 font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ run.cost_usd == null ? t('pelican.admin.results.unknownCost') : `$${run.cost_usd.toFixed(6)}` }}
          </p>
        </div>
      </div>

      <div class="grid gap-3 text-sm sm:grid-cols-2">
        <p><span class="text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.upstreamModel') }}:</span> <span class="font-mono">{{ run.upstream_model }}</span></p>
        <p><span class="text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.endpoint') }}:</span> <span class="break-all font-mono">{{ run.endpoint }}</span></p>
        <p><span class="text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.terminal') }}:</span> <span class="font-mono">{{ run.terminal || '—' }}</span></p>
        <p><span class="text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.requestedBy') }}:</span> {{ run.requested_by === 0 ? t('pelican.admin.results.scheduled') : `#${run.requested_by}` }}</p>
      </div>

      <p v-if="run.error" class="rounded-2xl bg-rose-50 p-4 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
        {{ run.error }}
      </p>

      <PelicanDynamicPreview
        v-if="run.output"
        :source="run.output"
        :title="t('pelican.preview.adminTitle', { model: run.model })"
      />

      <details class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
        <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('pelican.admin.results.frozenInput') }}
        </summary>
        <pre class="mt-3 whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-dark-200">{{ pretty(run.frozen) }}</pre>
      </details>
      <details class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
        <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('pelican.admin.results.usage') }}
        </summary>
        <pre class="mt-3 whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-dark-200">{{ pretty(run.usage) }}</pre>
      </details>
      <details class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
        <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('pelican.admin.results.rawResponse') }}
        </summary>
        <pre class="mt-3 whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-dark-200">{{ pretty(run.raw_response) }}</pre>
      </details>
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
import { usePelicanRefresh } from './usePelicanRefresh'
import { getPelicanAdminResult } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import { pelicanStateClass } from './pelicanFormat'
import type { PelicanAdminRun } from '@/types/pelican'

const props = defineProps<{
  show: boolean
  runId: string | null
}>()

defineEmits<{ close: [] }>()

const { t } = useI18n()
const run = ref<PelicanAdminRun | null>(null)
const loading = ref(false)
const error = ref('')
let controller: AbortController | null = null
const refresh = usePelicanRefresh(
  () => void load(true),
  () => props.show && run.value?.state === 'running' ? 10_000 : null,
)

function pretty(value: unknown): string {
  if (value == null || value === '') return '—'
  try {
    return typeof value === 'string' ? value : JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
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
    const result = await getPelicanAdminResult(props.runId, { signal: current.signal })
    if (!current.signal.aborted) run.value = result
  } catch (cause) {
    if (!current.signal.aborted) {
      error.value = extractApiErrorMessage(cause, t('pelican.admin.results.detailLoadFailed'))
    }
  } finally {
    if (!current.signal.aborted) {
      loading.value = false
      refresh.schedule()
    }
  }
}

watch(() => [props.show, props.runId] as const, () => void load(), { immediate: true })
onBeforeUnmount(() => controller?.abort())
</script>

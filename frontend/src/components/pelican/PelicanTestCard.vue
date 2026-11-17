<template>
  <article
    ref="root"
    class="group flex min-w-0 flex-col overflow-hidden rounded-3xl border border-gray-200 bg-white shadow-sm transition duration-200 hover:-translate-y-0.5 hover:shadow-xl dark:border-dark-700 dark:bg-dark-800"
  >
    <div class="relative aspect-[4/3] overflow-hidden bg-gradient-to-br from-slate-100 via-white to-cyan-50 dark:from-dark-900 dark:via-dark-800 dark:to-cyan-950/30">
      <PelicanStaticPreview
        v-if="run.state === 'completed' && output && staticPreview?.hasStaticVisual"
        :source="output"
        :title="t('pelican.preview.thumbnailTitle', { model: run.model })"
      />
      <div
        v-else
        class="flex h-full flex-col items-center justify-center px-6 text-center"
      >
        <LoadingSpinner v-if="loadingOutput" size="md" />
        <template v-else>
          <span
            class="mb-3 inline-flex h-11 w-11 items-center justify-center rounded-2xl border border-white/70 bg-white/80 text-slate-500 shadow-sm backdrop-blur dark:border-dark-600 dark:bg-dark-700/80 dark:text-dark-200"
          >
            <Icon :name="run.state === 'completed' ? 'eye' : statusIcon" size="lg" />
          </span>
          <p class="max-w-xs text-sm font-semibold text-slate-700 dark:text-dark-100">
            {{ placeholderTitle }}
          </p>
          <p class="mt-1 max-w-xs text-xs leading-5 text-slate-500 dark:text-dark-300">
            {{ placeholderDetail }}
          </p>
          <button
            v-if="run.state === 'completed' && !loadingOutput"
            type="button"
            class="btn btn-secondary btn-sm mt-4"
            @click="$emit('detail', run.id)"
          >
            {{ t('pelican.actions.openAnimation') }}
          </button>
        </template>
      </div>

      <span
        class="absolute left-3 top-3 inline-flex items-center rounded-full border px-2.5 py-1 text-[11px] font-bold uppercase tracking-wide shadow-sm backdrop-blur"
        :class="pelicanStateClass(run.state)"
      >
        {{ t(`pelican.state.${run.state}`) }}
      </span>
      <button
        v-if="run.state === 'completed'"
        type="button"
        class="absolute right-3 top-3 inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[11px] font-semibold shadow-sm backdrop-blur transition disabled:cursor-not-allowed disabled:opacity-50"
        :class="
          selected
            ? 'border-primary-300 bg-primary-600 text-white'
            : 'border-white/80 bg-white/90 text-slate-700 hover:bg-white dark:border-dark-600 dark:bg-dark-800/90 dark:text-dark-100'
        "
        :disabled="comparisonFull && !selected"
        :aria-pressed="selected"
        :title="comparisonFull && !selected ? t('pelican.compare.limit') : undefined"
        @click="$emit('toggle-compare', run.id)"
      >
        <Icon name="check" size="xs" v-if="selected" />
        {{ selected ? t('pelican.compare.selected') : t('pelican.compare.select') }}
      </button>
    </div>

    <div class="flex flex-1 flex-col p-4 sm:p-5">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ run.group_name }}</p>
          <h2 class="mt-1 break-words font-mono text-base font-bold leading-6 text-slate-950 dark:text-white">
            {{ run.model }}
          </h2>
        </div>
        <span class="shrink-0 rounded-full bg-gray-100 px-2 py-1 text-[11px] font-semibold text-gray-600 dark:bg-dark-700 dark:text-dark-200">
          {{ t(`pelican.protocol.${run.protocol}`) }}
        </span>
      </div>

      <div class="mt-3 flex flex-wrap gap-1.5 text-[11px]">
        <span class="rounded-full border border-gray-200 px-2 py-1 text-gray-600 dark:border-dark-600 dark:text-dark-200">
          {{ run.effort || t('pelican.effort.unspecified') }}
        </span>
        <span class="rounded-full border border-gray-200 px-2 py-1 font-mono text-gray-600 dark:border-dark-600 dark:text-dark-200">
          {{ run.prompt_version }}
        </span>
        <span class="rounded-full border border-gray-200 px-2 py-1 tabular-nums text-gray-600 dark:border-dark-600 dark:text-dark-200">
          {{ t('pelican.fields.tokens', { count: run.max_output_tokens }) }}
        </span>
      </div>

      <div class="mt-4 flex items-center justify-between gap-3 border-t border-gray-100 pt-3 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-300">
        <span :title="formatDate(run.created_at)">
          {{ t('pelican.freshnessAgo', { value: freshness }) }}
        </span>
        <span class="tabular-nums">
          {{ run.state === 'running' ? t('pelican.durationPending') : formatPelicanDuration(run.duration_ms) }}
        </span>
      </div>

      <div class="mt-4 flex gap-2">
        <button type="button" class="btn btn-secondary flex-1" @click="$emit('detail', run.id)">
          <Icon name="eye" size="sm" />
          {{ t('pelican.actions.details') }}
        </button>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PelicanStaticPreview from './PelicanStaticPreview.vue'
import { getPelicanResult } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import { buildOperationsStaticPreview } from '@/utils/operationsPreview'
import { formatPelicanDuration, formatPelicanFreshness, pelicanStateClass } from './pelicanFormat'
import type { PelicanPublicRun } from '@/types/pelican'

const props = defineProps<{
  run: PelicanPublicRun
  selected: boolean
  comparisonFull: boolean
}>()

defineEmits<{
  detail: [id: string]
  'toggle-compare': [id: string]
}>()

const { t } = useI18n()
const root = ref<HTMLElement | null>(null)
const output = ref('')
const loadingOutput = ref(false)
const outputError = ref('')
let observer: IntersectionObserver | null = null
let controller: AbortController | null = null

const freshness = computed(() => formatPelicanFreshness(props.run.created_at))
const staticPreview = computed(() => (output.value ? buildOperationsStaticPreview(output.value) : null))
const statusIcon = computed(() => {
  if (props.run.state === 'failed') return 'xCircle'
  if (props.run.state === 'unknown') return 'questionCircle'
  return 'clock'
})
const placeholderTitle = computed(() => {
  if (props.run.state === 'running') return t('pelican.preview.running')
  if (props.run.state === 'failed') return t('pelican.preview.failed')
  if (props.run.state === 'unknown') return t('pelican.preview.unknown')
  if (outputError.value) return t('pelican.preview.loadFailed')
  return t('pelican.preview.dynamicOnly')
})
const placeholderDetail = computed(() => {
  if (props.run.state === 'completed') return t('pelican.preview.openAnimationDetail')
  if (props.run.error_kind) return t(`pelican.errorKind.${props.run.error_kind}`)
  return t('pelican.preview.noOutput')
})

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function loadOutput() {
  if (output.value || loadingOutput.value || props.run.state !== 'completed') return
  loadingOutput.value = true
  outputError.value = ''
  controller = new AbortController()
  try {
    const detail = await getPelicanResult(props.run.id, { signal: controller.signal })
    output.value = detail.output || ''
  } catch (error) {
    if (!controller.signal.aborted) outputError.value = extractApiErrorMessage(error, t('pelican.preview.loadFailed'))
  } finally {
    loadingOutput.value = false
  }
}

onMounted(() => {
  if (props.run.state !== 'completed') return
  if (!('IntersectionObserver' in window)) return
  observer = new IntersectionObserver((entries) => {
    if (!entries.some((entry) => entry.isIntersecting)) return
    observer?.disconnect()
    observer = null
    void loadOutput()
  }, { rootMargin: '80px 0px' })
  if (root.value) observer.observe(root.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  controller?.abort()
})
</script>

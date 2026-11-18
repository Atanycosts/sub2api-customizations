<template>
  <article
    ref="root"
    class="pelican-card overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
    :class="{ 'pelican-card-compact': compact }"
  >
    <header class="pelican-card-heading min-w-0 px-3 py-2">
      <div class="flex items-center justify-between gap-2">
        <h2
          class="truncate text-sm font-bold text-gray-950 dark:text-white"
          :title="run.group_name"
        >
          {{ run.group_name }}
        </h2>
        <span class="shrink-0 text-[10px] font-semibold" :class="stateTextClass">
          {{ t(`pelican.state.${run.state}`) }}
        </span>
      </div>
      <p
        class="mt-0.5 truncate text-[11px] text-gray-500 dark:text-dark-300"
        :title="`${run.model} · ${run.effort || t('pelican.effort.unspecified')} · ${t(`pelican.protocol.${run.protocol}`)}`"
      >
        <span class="font-mono">{{ run.model }}</span>
        · {{ run.effort || t('pelican.effort.unspecified') }} ·
        {{ t(`pelican.protocol.${run.protocol}`) }}
      </p>
    </header>
    <div
      class="pelican-card-preview relative min-h-0 overflow-hidden bg-gray-50 dark:bg-dark-900/50"
    >
      <PelicanStaticPreview
        v-if="run.state === 'completed' && output && staticPreview?.hasStaticVisual"
        :source="output"
        :title="t('pelican.preview.thumbnailTitle', { model: run.model })"
      />
      <div
        v-else
        class="flex h-full flex-col items-center justify-center gap-1 px-3 py-2 text-center"
      >
        <LoadingSpinner v-if="loadingOutput" size="sm" />
        <template v-else>
          <Icon
            :name="run.state === 'completed' ? 'eye' : statusIcon"
            size="md"
            class="text-gray-400 dark:text-dark-400"
          />
          <p class="text-xs font-semibold text-gray-700 dark:text-dark-100">
            {{ placeholderTitle }}
          </p>
          <p class="pelican-card-problem text-[11px] leading-4 text-gray-500 dark:text-dark-300">
            {{ placeholderDetail }}
          </p>
        </template>
      </div>
      <button
        v-if="run.state === 'completed'"
        type="button"
        class="absolute inset-0 focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary-500"
        :aria-label="t('pelican.actions.openAnimation')"
        @click="$emit('detail', run.id)"
      ></button>
    </div>
    <footer class="pelican-card-footer min-w-0 px-3 py-2">
      <div
        class="flex items-center justify-between gap-2 text-[11px] text-gray-500 dark:text-dark-300"
      >
        <span class="truncate" :title="formatDate(run.created_at)">
          {{ t('pelican.freshnessAgo', { value: freshness }) }}
        </span>
        <span class="shrink-0 tabular-nums">
          {{
            run.state === 'running'
              ? t('pelican.durationPending')
              : formatPelicanDuration(run.duration_ms)
          }}
        </span>
      </div>
      <div class="mt-2 flex items-center justify-between gap-1 text-xs">
        <label
          v-if="run.state === 'completed'"
          class="inline-flex min-h-6 cursor-pointer items-center gap-1 text-gray-600 dark:text-dark-200"
          :title="comparisonFull && !selected ? t('pelican.compare.limit') : undefined"
        >
          <input
            type="checkbox"
            :checked="selected"
            :disabled="comparisonFull && !selected"
            class="rounded border-gray-300 text-primary-600"
            @change="$emit('toggle-compare', run.id)"
          />
          {{ t('pelican.compare.select') }}
        </label>
        <button
          v-else
          type="button"
          class="min-h-6 text-gray-600 hover:text-primary-600 dark:text-dark-200"
          @click="$emit('detail', run.id)"
        >
          {{ t('pelican.actions.details') }}
        </button>
        <button
          type="button"
          class="min-h-6 text-gray-500 hover:text-primary-600 dark:text-dark-300"
          @click="$emit('history')"
        >
          {{
            run.history_count
              ? t('pelican.history.count', { count: run.history_count })
              : t('pelican.history.action')
          }}
        </button>
      </div>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PelicanStaticPreview from './PelicanStaticPreview.vue'
import { getPelicanResult } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import { buildOperationsStaticPreview } from '@/utils/operationsPreview'
import { formatPelicanDuration, formatPelicanFreshness } from './pelicanFormat'
import type { PelicanPublicRun } from '@/types/pelican'

const props = defineProps<{
  run: PelicanPublicRun
  selected: boolean
  comparisonFull: boolean
  compact?: boolean
}>()

defineEmits<{
  detail: [id: string]
  'toggle-compare': [id: string]
  history: []
}>()

const { t } = useI18n()
const visible = ref(false)
const root = ref<HTMLElement | null>(null)
const output = ref('')
const loadingOutput = ref(false)
const outputError = ref('')
let observer: IntersectionObserver | null = null
let controller: AbortController | null = null

const freshness = computed(() => formatPelicanFreshness(props.run.created_at))
const staticPreview = computed(() =>
  output.value ? buildOperationsStaticPreview(output.value) : null
)
const stateTextClass = computed(
  () =>
    ({
      completed: 'text-emerald-600 dark:text-emerald-400',
      failed: 'text-rose-600 dark:text-rose-400',
      unknown: 'text-amber-600 dark:text-amber-400',
      running: 'text-sky-600 dark:text-sky-400'
    })[props.run.state]
)
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
  const current = new AbortController()
  controller = current
  try {
    const detail = await getPelicanResult(props.run.id, {
      signal: current.signal
    })
    if (!current.signal.aborted) output.value = detail.output || ''
  } catch (error) {
    if (!current.signal.aborted)
      outputError.value = extractApiErrorMessage(error, t('pelican.preview.loadFailed'))
  } finally {
    if (!current.signal.aborted) loadingOutput.value = false
  }
}

onMounted(() => {
  if (!('IntersectionObserver' in window)) {
    visible.value = true
    void loadOutput()
    return
  }
  observer = new IntersectionObserver(
    (entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return
      visible.value = true
      observer?.disconnect()
      observer = null
      void loadOutput()
    },
    { rootMargin: '80px 0px' }
  )
  if (root.value) observer.observe(root.value)
})

watch(
  () => [props.run.id, props.run.state],
  () => {
    controller?.abort()
    output.value = ''
    outputError.value = ''
    loadingOutput.value = false
    if (visible.value) void loadOutput()
  }
)

onBeforeUnmount(() => {
  observer?.disconnect()
  controller?.abort()
})
</script>

<style scoped>
.pelican-card {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
}
.pelican-card-preview {
  border-block: 1px solid rgb(229 231 235 / 0.55);
}
.pelican-card-compact {
  grid-template-columns: 32% minmax(0, 1fr);
  grid-template-rows: auto minmax(0, 1fr);
}
.pelican-card-compact .pelican-card-preview {
  grid-column: 1;
  grid-row: 1 / 3;
  border: 0;
  border-right: 1px solid rgb(229 231 235 / 0.55);
}
.pelican-card-compact .pelican-card-heading {
  grid-column: 2;
  padding: 8px 10px 0;
}
.pelican-card-compact .pelican-card-footer {
  grid-column: 2;
  padding: 4px 10px 6px;
  align-self: end;
}
.pelican-card-compact .pelican-card-heading h2 {
  font-size: 12px;
}
.pelican-card-compact .pelican-card-heading p {
  font-size: 10px;
}
.pelican-card-compact .pelican-card-problem {
  display: none;
}
.pelican-card-compact .pelican-card-footer > div {
  font-size: 10px;
}
</style>

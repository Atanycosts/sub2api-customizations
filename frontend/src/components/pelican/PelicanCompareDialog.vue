<template>
  <BaseDialog
    :show="show"
    :title="t('pelican.compare.title')"
    width="full"
    close-on-click-outside
    @close="$emit('close')"
  >
    <div v-if="loading" class="flex min-h-80 items-center justify-center">
      <LoadingSpinner size="lg" />
    </div>
    <p v-else-if="error" role="alert" class="rounded-2xl bg-rose-50 p-4 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
      {{ error }}
    </p>
    <div v-else class="space-y-5">
      <p class="rounded-2xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm leading-6 text-blue-900 dark:border-blue-900/60 dark:bg-blue-950/30 dark:text-blue-200">
        {{ t('pelican.compare.disclosure') }}
      </p>
      <div class="grid gap-5 lg:grid-cols-2">
        <section
          v-for="run in runs"
          :key="run.id"
          class="min-w-0 rounded-3xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800 sm:p-5"
        >
          <div class="mb-4 flex items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-300">{{ run.group_name }}</p>
              <h3 class="mt-1 break-words font-mono text-base font-bold text-gray-950 dark:text-white">
                {{ run.model }}
              </h3>
            </div>
            <span
              class="shrink-0 rounded-full border px-2 py-1 text-[11px] font-bold"
              :class="pelicanStateClass(run.state)"
            >
              {{ t(`pelican.state.${run.state}`) }}
            </span>
          </div>
          <PelicanRunFacts :run="run" />
          <div class="mt-4">
            <PelicanDynamicPreview
              v-if="run.output"
              :source="run.output"
              :title="t('pelican.preview.compareTitle', { model: run.model })"
              :show-notice="false"
            />
            <div v-else class="rounded-2xl border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-300">
              {{ t('pelican.preview.noOutput') }}
            </div>
          </div>
        </section>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PelicanDynamicPreview from './PelicanDynamicPreview.vue'
import PelicanRunFacts from './PelicanRunFacts.vue'
import { getPelicanResult } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import { pelicanStateClass } from './pelicanFormat'
import type { PelicanPublicRunDetail } from '@/types/pelican'

const props = defineProps<{
  show: boolean
  ids: string[]
}>()

defineEmits<{ close: [] }>()

const { t } = useI18n()
const runs = ref<PelicanPublicRunDetail[]>([])
const loading = ref(false)
const error = ref('')
const requestedIDs = computed(() => props.ids.slice(0, 2))
let controller: AbortController | null = null
let requestSequence = 0

async function load() {
  const requestID = ++requestSequence
  controller?.abort()
  const current = new AbortController()
  controller = current
  runs.value = []
  error.value = ''
  loading.value = false
  if (!props.show || requestedIDs.value.length !== 2) return
  loading.value = true
  try {
    const result = await Promise.all(
      requestedIDs.value.map((id) => getPelicanResult(id, { signal: current.signal })),
    )
    if (requestID === requestSequence) runs.value = result
  } catch (cause) {
    if (requestID === requestSequence && !current.signal.aborted) {
      error.value = extractApiErrorMessage(cause, t('pelican.compare.loadFailed'))
    }
  } finally {
    if (requestID === requestSequence) loading.value = false
  }
}

watch(() => [props.show, requestedIDs.value.join(':')] as const, () => void load(), { immediate: true })
onBeforeUnmount(() => {
  requestSequence += 1
  controller?.abort()
})
</script>

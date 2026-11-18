<template>
  <BaseDialog
    :show="show"
    :title="t('pelican.history.title', { group: groupName })"
    width="wide"
    close-on-click-outside
    @close="$emit('close')"
  >
    <p class="mb-3 text-xs text-gray-500 dark:text-dark-300">
      {{ t('pelican.history.scope') }}
      <span v-if="model">· {{ model }}</span>
      <span v-if="effort">· {{ effort }}</span>
    </p>
    <div v-if="loading" class="flex min-h-40 items-center justify-center">
      <LoadingSpinner />
    </div>
    <p v-else-if="error" role="alert" class="text-sm text-rose-600 dark:text-rose-300">
      {{ error }}
    </p>
    <template v-else-if="data">
      <ul class="divide-y divide-gray-100 dark:divide-dark-700">
        <li v-for="run in data.items" :key="run.id">
          <button
            type="button"
            class="flex w-full items-center justify-between gap-3 rounded-lg px-2 py-3 text-left hover:bg-gray-50 dark:hover:bg-dark-700"
            @click="$emit('detail', run.id)"
          >
            <div class="min-w-0">
              <p class="truncate font-mono text-sm font-semibold text-gray-900 dark:text-white">
                {{ run.model }}
              </p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-300">
                {{ formatDate(run.created_at) }} ·
                {{ run.effort || t('pelican.effort.unspecified') }} ·
                {{ t(`pelican.protocol.${run.protocol}`) }}
              </p>
            </div>
            <div class="shrink-0 text-right text-xs">
              <span class="rounded-full border px-2 py-0.5" :class="pelicanStateClass(run.state)">
                {{ t(`pelican.state.${run.state}`) }}
              </span>
              <p class="mt-2 text-gray-500 dark:text-dark-300">
                {{
                  run.state === 'running'
                    ? t('pelican.durationPending')
                    : formatPelicanDuration(run.duration_ms)
                }}
              </p>
            </div>
          </button>
        </li>
      </ul>
      <p v-if="!data.items.length" class="py-8 text-center text-sm text-gray-500">
        {{ t('pelican.empty.title') }}
      </p>
      <PelicanCompactPager
        :page="data.page"
        :total="data.total"
        :page-size="data.page_size"
        @update:page="changePage"
      />
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PelicanCompactPager from './PelicanCompactPager.vue'
import { formatPelicanDuration, pelicanStateClass } from './pelicanFormat'
import { usePelicanRefresh } from './usePelicanRefresh'
import { getPelicanResults } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { PelicanEffort, PelicanPublicPage } from '@/types/pelican'
const props = defineProps<{
  show: boolean
  groupId: number | null
  groupName: string
  model: string
  effort: PelicanEffort | null
}>()
defineEmits<{ close: []; detail: [id: string] }>()
const { t } = useI18n()
const data = ref<PelicanPublicPage | null>(null)
const page = ref(1)
const loading = ref(false)
const error = ref('')
let controller: AbortController | null = null
const refresh = usePelicanRefresh(
  () => void load(true),
  () => props.show ? (data.value?.stats.running ? 10_000 : 30_000) : null,
)
function formatDate(value: string) {
  return new Date(value).toLocaleString()
}
async function load(background = false) {
  refresh.clear()
  controller?.abort()
  if (!background) {
    data.value = null
    loading.value = false
  }
  error.value = ''
  if (!props.show || props.groupId === null) return
  const current = new AbortController()
  controller = current
  if (!background) loading.value = true
  try {
    const result = await getPelicanResults(
      {
        group_id: props.groupId,
        ...(props.model ? { model: props.model } : {}),
        ...(props.effort ? { effort: props.effort } : {}),
        page: page.value,
        page_size: 8
      },
      { signal: current.signal }
    )
    if (current.signal.aborted) return
    const lastPage = Math.max(1, result.pages)
    if (page.value > lastPage) {
      page.value = lastPage
      await load(background)
      return
    }
    data.value = result
  } catch (cause) {
    if (!current.signal.aborted)
      error.value = extractApiErrorMessage(cause, t('pelican.history.loadFailed'))
  } finally {
    if (!current.signal.aborted) {
      loading.value = false
      refresh.schedule()
    }
  }
}
function changePage(value: number) {
  page.value = value
  void load()
}
watch(
  () => [props.show, props.groupId, props.model, props.effort],
  () => {
    page.value = 1
    data.value = null
    void load()
  },
  { immediate: true }
)
onBeforeUnmount(() => controller?.abort())
</script>

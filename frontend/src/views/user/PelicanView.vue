<template>
  <AppLayout viewport>
    <div class="pelican-page flex min-h-0 w-full flex-1 flex-col gap-3">
      <header class="flex shrink-0 items-center justify-between gap-2">
        <h1 class="truncate text-xl font-bold text-gray-950 dark:text-white">
          {{ t('pelican.page.title') }}
        </h1>
        <div class="flex shrink-0 items-center gap-2">
          <button type="button" class="btn btn-secondary btn-sm" @click="rulesOpen = true">
            {{ t('pelican.rules.title') }}
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="selectedForCompare.length !== 2"
            @click="compareOpen = true"
          >
            {{ t('pelican.actions.compare') }} {{ selectedForCompare.length }}/2
          </button>
          <button
            v-if="selectedForCompare.length"
            type="button"
            class="text-xs text-gray-500"
            @click="selectedForCompare = []"
          >
            {{ t('pelican.actions.clear') }}
          </button>
        </div>
      </header>

      <form
        class="pelican-filters grid shrink-0 grid-cols-2 gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_minmax(0,1fr)_auto_auto]"
        @submit.prevent="applyFilters"
      >
        <Select
          v-model="groupFilter"
          :options="groupOptions"
          :placeholder="t('pelican.filters.allGroups')"
          searchable
          clearable
          :aria-label="t('pelican.filters.group')"
          @change="applyFilters"
        />
        <input
          v-model.trim="modelFilter"
          list="pelican-model-options"
          class="input min-w-0"
          :placeholder="t('pelican.filters.modelPlaceholder')"
          :aria-label="t('pelican.filters.model')"
          @change="applyFilters"
        />
        <datalist id="pelican-model-options">
          <option v-for="model in knownModels" :key="model" :value="model" />
        </datalist>
        <Select
          v-model="effortFilter"
          :options="effortOptions"
          :placeholder="t('pelican.filters.allEfforts')"
          clearable
          :aria-label="t('pelican.filters.effort')"
          @change="applyFilters"
        />
        <button type="button" class="btn btn-secondary btn-sm" @click="resetFilters">
          {{ t('pelican.filters.reset') }}
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm hidden md:inline-flex"
          :aria-pressed="listMode"
          @click="listMode = !listMode"
        >
          {{ t(listMode ? 'pelican.gallery.gridView' : 'pelican.gallery.listView') }}
        </button>
      </form>

      <div
        class="flex shrink-0 flex-wrap items-center justify-between gap-1 text-xs text-gray-500 dark:text-dark-300"
      >
        <span>{{ t('pelican.gallery.latestPerGroup') }}</span>
        <div class="flex gap-2" :aria-label="t('pelican.stats.title')">
          <span v-for="stat in statCards" :key="stat.state" class="inline-flex items-center gap-1">
            <i class="h-1.5 w-1.5 rounded-full" :class="stat.dotClass"></i>
            {{ stat.label }} {{ stat.value }}
          </span>
        </div>
      </div>
      <p v-if="error" role="alert" class="shrink-0 text-sm text-rose-600 dark:text-rose-300">
        {{ error }}
      </p>
      <section
        ref="gallery"
        class="pelican-gallery min-h-0 flex-1"
        :class="{ 'pelican-gallery-compact': layout.compact }"
        :style="{
          '--pelican-columns': layout.columns,
          '--pelican-rows': layout.rows
        }"
        :aria-label="t('pelican.gallery.title')"
        :aria-busy="loading"
      >
        <template v-if="loading && !data">
          <div
            v-for="index in layout.pageSize"
            :key="index"
            class="animate-pulse rounded-xl border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800"
          ></div>
        </template>
        <template v-else-if="data?.items.length">
          <PelicanTestCard
            v-for="run in data.items"
            :key="run.id"
            :run="run"
            :selected="selectedForCompare.includes(run.id)"
            :comparison-full="selectedForCompare.length >= 2"
            :compact="layout.compact"
            @detail="openDetail"
            @toggle-compare="toggleCompare"
            @history="historyRun = run"
          />
        </template>
        <div
          v-else-if="data"
          class="col-span-full flex flex-col items-center justify-center rounded-xl border border-dashed border-gray-300 p-4 text-center dark:border-dark-600"
        >
          <h2 class="font-semibold text-gray-900 dark:text-white">
            {{ t('pelican.empty.title') }}
          </h2>
          <p class="mt-2 text-sm text-gray-500 dark:text-dark-300">
            {{ t('pelican.empty.description') }}
          </p>
        </div>
      </section>
      <PelicanCompactPager
        v-if="data"
        class="shrink-0"
        :page="data.page"
        :total="data.total"
        :page-size="data.page_size"
        @update:page="changePage"
      />
      <PelicanRulesDialog
        :show="rulesOpen"
        :prompt="data?.prompt"
        :prompt-version="data?.prompt_version"
        @close="rulesOpen = false"
      />
      <PelicanHistoryDialog
        :show="!!historyRun"
        :group-id="historyRun?.group_id ?? null"
        :group-name="historyRun?.group_name || ''"
        :model="appliedModel"
        :effort="appliedEffort"
        @close="historyRun = null"
        @detail="openDetail"
      />
      <PelicanDetailDialog :show="detailOpen" :run-id="detailRunId" @close="detailOpen = false" />
      <PelicanCompareDialog
        :show="compareOpen"
        :ids="selectedForCompare"
        @close="compareOpen = false"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import PelicanCompactPager from '@/components/pelican/PelicanCompactPager.vue'
import PelicanRulesDialog from '@/components/pelican/PelicanRulesDialog.vue'
import PelicanHistoryDialog from '@/components/pelican/PelicanHistoryDialog.vue'
import { usePelicanGalleryLayout } from '@/components/pelican/usePelicanGalleryLayout'
import PelicanTestCard from '@/components/pelican/PelicanTestCard.vue'
import PelicanDetailDialog from '@/components/pelican/PelicanDetailDialog.vue'
import PelicanCompareDialog from '@/components/pelican/PelicanCompareDialog.vue'
import { togglePelicanComparison } from '@/components/pelican/compareSelection'
import { getPelicanResults } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { PelicanEffort, PelicanPublicPage, PelicanPublicRun } from '@/types/pelican'

const { t } = useI18n()
const gallery = ref<HTMLElement | null>(null)
const listMode = ref(false)
const layout = usePelicanGalleryLayout(gallery, listMode)
const rulesOpen = ref(false)
const historyRun = ref<PelicanPublicRun | null>(null)
const appliedModel = ref('')
const appliedEffort = ref<PelicanEffort | null>(null)
const data = ref<PelicanPublicPage | null>(null)
const availableGroups = ref<PelicanPublicPage['groups']>([])
const loading = ref(false)
const error = ref('')
const page = ref(1)
const groupFilter = ref<number | null>(null)
const modelFilter = ref('')
const effortFilter = ref<PelicanEffort | null>(null)
const knownModels = ref<string[]>([])
const selectedForCompare = ref<string[]>([])
const detailOpen = ref(false)
const detailRunId = ref<string | null>(null)
const compareOpen = ref(false)
let controller: AbortController | null = null
let pollTimer: ReturnType<typeof setTimeout> | null = null
let pollAttempts = 0

const groupOptions = computed(() => [
  ...availableGroups.value.map((group) => ({
    value: group.id,
    label: group.name
  }))
])
const effortOptions = [
  { value: 'none', label: 'none' },
  { value: 'minimal', label: 'minimal' },
  { value: 'low', label: 'low' },
  { value: 'medium', label: 'medium' },
  { value: 'high', label: 'high' },
  { value: 'xhigh', label: 'xhigh' },
  { value: 'max', label: 'max' }
]
const statCards = computed(() => [
  {
    state: 'completed' as const,
    label: t('pelican.state.completed'),
    value: data.value?.stats.completed || 0,
    dotClass: 'bg-emerald-500'
  },
  {
    state: 'failed' as const,
    label: t('pelican.state.failed'),
    value: data.value?.stats.failed || 0,
    dotClass: 'bg-rose-500'
  },
  {
    state: 'unknown' as const,
    label: t('pelican.state.unknown'),
    value: data.value?.stats.unknown || 0,
    dotClass: 'bg-amber-500'
  },
  {
    state: 'running' as const,
    label: t('pelican.state.running'),
    value: data.value?.stats.running || 0,
    dotClass: 'bg-sky-500'
  }
])

function recordModels(items: { model: string }[]) {
  const models = new Set(knownModels.value)
  for (const item of items) {
    if (item.model) models.add(item.model)
  }
  knownModels.value = [...models].sort((left, right) => left.localeCompare(right))
}

function clearPoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

function schedulePoll() {
  clearPoll()
  if (document.visibilityState !== 'visible' || !data.value?.stats.running || pollAttempts >= 60)
    return
  pollTimer = setTimeout(async () => {
    pollAttempts += 1
    await loadResults(true)
  }, 10_000)
}

async function loadResults(background = false) {
  clearPoll()
  controller?.abort()
  const current = new AbortController()
  controller = current
  if (!background) {
    data.value = null
    historyRun.value = null
    selectedForCompare.value = []
    compareOpen.value = false
    loading.value = true
    error.value = ''
  }
  try {
    const result = await getPelicanResults(
      {
        page: page.value,
        page_size: layout.value.pageSize,
        latest_per_group: true,
        ...(groupFilter.value ? { group_id: groupFilter.value } : {}),
        ...(appliedModel.value ? { model: appliedModel.value } : {}),
        ...(appliedEffort.value ? { effort: appliedEffort.value } : {})
      },
      { signal: current.signal }
    )
    if (current.signal.aborted) return
    const lastPage = Math.max(1, result.pages)
    if (page.value > lastPage) {
      page.value = lastPage
      await loadResults()
      return
    }
    data.value = result
    availableGroups.value = result.groups
    error.value = ''
    const completedIds = new Set(
      result.items.filter((run) => run.state === 'completed').map((run) => run.id)
    )
    selectedForCompare.value = selectedForCompare.value.filter((id) => completedIds.has(id))
    if (selectedForCompare.value.length !== 2) compareOpen.value = false
    recordModels(result.items)
  } catch (cause) {
    if (!current.signal.aborted) error.value = extractApiErrorMessage(cause, t('common.error'))
  } finally {
    if (!current.signal.aborted) {
      loading.value = false
      schedulePoll()
    }
  }
}

function applyFilters() {
  appliedModel.value = modelFilter.value.trim()
  appliedEffort.value = effortFilter.value
  page.value = 1
  pollAttempts = 0
  void loadResults()
}

function resetFilters() {
  groupFilter.value = null
  modelFilter.value = ''
  effortFilter.value = null
  applyFilters()
}

function changePage(nextPage: number) {
  page.value = nextPage
  pollAttempts = 0
  void loadResults()
}

watch(
  () => [layout.value.pageSize, listMode.value],
  () => {
    page.value = 1
    void loadResults()
  }
)

function openDetail(id: string) {
  detailRunId.value = id
  detailOpen.value = true
}

function toggleCompare(id: string) {
  if (!data.value?.items.some((run) => run.id === id && run.state === 'completed')) return
  selectedForCompare.value = togglePelicanComparison(selectedForCompare.value, id)
}

function handleVisibilityChange() {
  if (document.visibilityState === 'visible') schedulePoll()
  else clearPoll()
}

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  void loadResults()
})

onBeforeUnmount(() => {
  controller?.abort()
  clearPoll()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<style scoped>
.pelican-gallery {
  display: grid;
  grid-template-columns: repeat(var(--pelican-columns), minmax(0, 1fr));
  grid-template-rows: repeat(var(--pelican-rows), minmax(0, 1fr));
  gap: 12px;
}

.pelican-filters :deep(.input),
.pelican-filters > .input {
  min-height: 36px;
  padding-top: 6px;
  padding-bottom: 6px;
  font-size: 12px;
}
@media (max-width: 640px) {
  .pelican-page {
    gap: 8px;
  }
  .pelican-gallery {
    gap: 8px;
  }
  .pelican-filters > input {
    font-size: 16px;
  }
}
</style>

<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6 pb-10">
      <section class="relative overflow-hidden rounded-[2rem] border border-cyan-100 bg-gradient-to-br from-cyan-50 via-white to-amber-50 px-5 py-5 shadow-sm dark:border-cyan-900/40 dark:from-cyan-950/40 dark:via-dark-800 dark:to-amber-950/20 sm:px-7">
        <div class="pointer-events-none absolute -right-20 -top-24 h-64 w-64 rounded-full bg-cyan-200/40 blur-3xl dark:bg-cyan-700/20"></div>
        <div class="relative max-w-3xl">
          <div class="mb-2 inline-flex items-center gap-2 rounded-full border border-cyan-200 bg-white/80 px-3 py-1 text-xs font-bold text-cyan-800 shadow-sm backdrop-blur dark:border-cyan-800 dark:bg-dark-800/80 dark:text-cyan-200">
            <Icon name="beaker" size="sm" />
            {{ t('pelican.page.badge') }}
          </div>
          <h1 class="text-2xl font-black tracking-tight text-slate-950 dark:text-white sm:text-3xl">
            {{ t('pelican.page.title') }}
          </h1>
          <p class="mt-2 max-w-3xl text-sm leading-6 text-slate-600 dark:text-dark-200">
            {{ t('pelican.page.description') }}
          </p>
          <div class="mt-3 flex flex-wrap items-center gap-2 text-xs">
            <span class="rounded-full bg-white/85 px-3 py-1 font-mono font-semibold text-slate-700 ring-1 ring-slate-200 dark:bg-dark-800/85 dark:text-dark-100 dark:ring-dark-600">
              {{ t('pelican.page.promptVersion', { version: data?.prompt_version || 'v1' }) }}
            </span>
            <span class="rounded-full bg-white/85 px-3 py-1 font-semibold text-slate-700 ring-1 ring-slate-200 dark:bg-dark-800/85 dark:text-dark-100 dark:ring-dark-600">
              {{ t('pelican.page.readOnly') }}
            </span>
          </div>
        </div>
      </section>

      <section class="flex flex-wrap gap-2" :aria-label="t('pelican.stats.title')">
        <div
          v-for="stat in statCards"
          :key="stat.state"
          class="inline-flex items-center gap-2 rounded-full border border-gray-200 bg-white px-3 py-1.5 text-xs shadow-sm dark:border-dark-700 dark:bg-dark-800"
        >
          <span class="h-2 w-2 rounded-full" :class="stat.dotClass"></span>
          <span class="font-medium text-gray-500 dark:text-dark-300">{{ stat.label }}</span>
          <span class="font-black tabular-nums text-gray-950 dark:text-white">{{ stat.value }}</span>
        </div>
      </section>

      <section class="rounded-3xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:p-5">
        <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-[1fr_1.3fr_1fr_auto_auto]">
          <Select
            v-model="groupFilter"
            :options="groupOptions"
            :placeholder="t('pelican.filters.allGroups')"
            :searchable="true"
            clearable
            :aria-label="t('pelican.filters.group')"
            @change="applyFilters"
          />
          <div class="relative">
            <input
              v-model.trim="modelFilter"
              list="pelican-model-options"
              class="input pr-10"
              :placeholder="t('pelican.filters.modelPlaceholder')"
              :aria-label="t('pelican.filters.model')"
              @keyup.enter="applyFilters"
            />
            <datalist id="pelican-model-options">
              <option v-for="model in knownModels" :key="model" :value="model" />
            </datalist>
            <Icon name="search" size="sm" class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400" />
          </div>
          <Select
            v-model="effortFilter"
            :options="effortOptions"
            :placeholder="t('pelican.filters.allEfforts')"
            clearable
            :aria-label="t('pelican.filters.effort')"
            @change="applyFilters"
          />
          <button type="button" class="btn btn-primary" :disabled="loading" @click="applyFilters">
            {{ t('pelican.filters.apply') }}
          </button>
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="resetFilters">
            {{ t('pelican.filters.reset') }}
          </button>
        </div>
      </section>

      <p
        v-if="error"
        role="alert"
        class="rounded-2xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700 dark:border-rose-900/60 dark:bg-rose-950/30 dark:text-rose-300"
      >
        {{ error }}
      </p>

      <section
        v-if="loading && !data"
        class="grid gap-5 md:grid-cols-2 xl:grid-cols-3"
        :aria-label="t('common.loading')"
      >
        <div
          v-for="index in 6"
          :key="index"
          class="overflow-hidden rounded-3xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="aspect-[4/3] animate-pulse bg-gray-100 dark:bg-dark-700"></div>
          <div class="space-y-3 p-5">
            <div class="h-4 w-1/2 animate-pulse rounded bg-gray-100 dark:bg-dark-700"></div>
            <div class="h-6 w-4/5 animate-pulse rounded bg-gray-100 dark:bg-dark-700"></div>
            <div class="h-10 animate-pulse rounded bg-gray-100 dark:bg-dark-700"></div>
          </div>
        </div>
      </section>

      <section v-else-if="data?.items.length" class="space-y-5">
        <div class="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 class="text-xl font-bold text-gray-950 dark:text-white">{{ t('pelican.gallery.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('pelican.gallery.description') }}</p>
          </div>
          <p class="text-xs text-gray-500 dark:text-dark-300">
            {{ t('pelican.gallery.count', { count: data.total }) }}
          </p>
        </div>
        <div class="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
          <PelicanTestCard
            v-for="run in data.items"
            :key="run.id"
            :run="run"
            :selected="selectedForCompare.includes(run.id)"
            :comparison-full="selectedForCompare.length >= 2"
            @detail="openDetail"
            @toggle-compare="toggleCompare"
          />
        </div>
        <div class="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <Pagination
            :page="data.page"
            :total="data.total"
            :page-size="data.page_size"
            :show-page-size-selector="false"
            @update:page="changePage"
          />
        </div>
      </section>

      <section
        v-else-if="data"
        class="rounded-3xl border border-dashed border-gray-300 bg-white px-6 py-14 text-center dark:border-dark-600 dark:bg-dark-800"
      >
        <Icon name="inbox" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
        <h2 class="mt-3 text-lg font-bold text-gray-900 dark:text-white">{{ t('pelican.empty.title') }}</h2>
        <p class="mx-auto mt-2 max-w-lg text-sm leading-6 text-gray-500 dark:text-dark-300">
          {{ t('pelican.empty.description') }}
        </p>
      </section>

      <section class="rounded-3xl border border-amber-200 bg-amber-50/80 p-5 dark:border-amber-900/60 dark:bg-amber-950/20">
        <div class="flex items-start gap-3">
          <Icon name="infoCircle" size="md" class="mt-0.5 shrink-0 text-amber-600 dark:text-amber-300" />
          <div>
            <h2 class="text-sm font-bold text-amber-950 dark:text-amber-100">{{ t('pelican.disclosureTitle') }}</h2>
            <p class="mt-1 text-sm leading-6 text-amber-800 dark:text-amber-200">
              {{ t('pelican.disclosure') }}
            </p>
          </div>
        </div>
      </section>

      <div
        v-if="selectedForCompare.length"
        class="sticky bottom-4 z-20 mx-auto flex max-w-2xl flex-wrap items-center justify-between gap-3 rounded-2xl border border-primary-200 bg-white/95 p-3 shadow-xl backdrop-blur dark:border-primary-900 dark:bg-dark-800/95"
      >
        <p class="px-1 text-sm text-gray-700 dark:text-dark-100">
          {{ t('pelican.compare.selectedCount', { count: selectedForCompare.length }) }}
        </p>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" @click="selectedForCompare = []">
            {{ t('pelican.actions.clear') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="selectedForCompare.length !== 2"
            @click="compareOpen = true"
          >
            {{ t('pelican.actions.compare') }}
          </button>
        </div>
      </div>

      <PelicanDetailDialog
        :show="detailOpen"
        :run-id="detailRunId"
        @close="detailOpen = false"
      />
      <PelicanCompareDialog
        :show="compareOpen"
        :ids="selectedForCompare"
        @close="compareOpen = false"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import PelicanTestCard from '@/components/pelican/PelicanTestCard.vue'
import PelicanDetailDialog from '@/components/pelican/PelicanDetailDialog.vue'
import PelicanCompareDialog from '@/components/pelican/PelicanCompareDialog.vue'
import { togglePelicanComparison } from '@/components/pelican/compareSelection'
import { getPelicanResults } from '@/api/pelican'
import { extractApiErrorMessage } from '@/utils/apiError'
import type {
  PelicanEffort,
  PelicanPublicPage,
} from '@/types/pelican'

const { t } = useI18n()
const data = ref<PelicanPublicPage | null>(null)
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
  ...(data.value?.groups || []).map((group) => ({ value: group.id, label: group.name })),
])
const effortOptions = [
  { value: 'none', label: 'none' },
  { value: 'minimal', label: 'minimal' },
  { value: 'low', label: 'low' },
  { value: 'medium', label: 'medium' },
  { value: 'high', label: 'high' },
  { value: 'xhigh', label: 'xhigh' },
  { value: 'max', label: 'max' },
]
const statCards = computed(() => [
  { state: 'completed' as const, label: t('pelican.state.completed'), value: data.value?.stats.completed || 0, dotClass: 'bg-emerald-500' },
  { state: 'failed' as const, label: t('pelican.state.failed'), value: data.value?.stats.failed || 0, dotClass: 'bg-rose-500' },
  { state: 'unknown' as const, label: t('pelican.state.unknown'), value: data.value?.stats.unknown || 0, dotClass: 'bg-amber-500' },
  { state: 'running' as const, label: t('pelican.state.running'), value: data.value?.stats.running || 0, dotClass: 'bg-sky-500' },
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
  if (
    document.visibilityState !== 'visible'
    || !data.value?.stats.running
    || pollAttempts >= 60
  ) return
  pollTimer = setTimeout(async () => {
    pollAttempts += 1
    await loadResults(true)
  }, 10_000)
}

async function loadResults(background = false) {
  controller?.abort()
  const current = new AbortController()
  controller = current
  if (!background) {
    loading.value = true
    error.value = ''
  }
  try {
    const result = await getPelicanResults(
      {
        page: page.value,
        page_size: 12,
        ...(groupFilter.value ? { group_id: groupFilter.value } : {}),
        ...(modelFilter.value.trim() ? { model: modelFilter.value.trim() } : {}),
        ...(effortFilter.value ? { effort: effortFilter.value } : {}),
      },
      { signal: current.signal },
    )
    if (current.signal.aborted) return
    data.value = result
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
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function openDetail(id: string) {
  detailRunId.value = id
  detailOpen.value = true
}

function toggleCompare(id: string) {
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

<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6 pb-10">
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="mb-2 inline-flex items-center gap-2 rounded-full bg-cyan-50 px-3 py-1 text-xs font-bold text-cyan-700 dark:bg-cyan-950/40 dark:text-cyan-300">
            <Icon name="beaker" size="sm" />
            {{ t('pelican.admin.badge') }}
          </div>
          <h1 class="text-2xl font-black tracking-tight text-gray-950 dark:text-white sm:text-3xl">
            {{ t('pelican.admin.title') }}
          </h1>
          <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-300">
            {{ t('pelican.admin.description') }}
          </p>
        </div>
        <router-link to="/pelican" class="btn btn-secondary">
          <Icon name="eye" size="sm" />
          {{ t('pelican.admin.openUserPage') }}
        </router-link>
      </header>

      <p
        v-if="pageError"
        role="alert"
        class="rounded-2xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700 dark:border-rose-900/60 dark:bg-rose-950/30 dark:text-rose-300"
      >
        {{ pageError }}
      </p>

      <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:p-6">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-bold text-gray-950 dark:text-white">{{ t('pelican.admin.config.title') }}</h2>
            <p class="mt-1 max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-300">
              {{ t('pelican.admin.config.description') }}
            </p>
          </div>
          <span
            class="rounded-full px-3 py-1 text-xs font-bold"
            :class="config.enabled && config.daily_budget_usd > 0
              ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
              : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'"
          >
            {{ config.enabled && config.daily_budget_usd > 0 ? t('pelican.admin.config.active') : t('pelican.admin.config.paused') }}
          </span>
        </div>
        <div class="mt-5 grid gap-4 md:grid-cols-[auto_1fr_auto] md:items-end">
          <div class="flex items-center gap-3 rounded-2xl border border-gray-200 p-3 dark:border-dark-700">
            <Toggle v-model="config.enabled" />
            <div>
              <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('pelican.admin.config.enabled') }}</p>
              <p class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.config.enabledHint') }}</p>
            </div>
          </div>
          <label class="space-y-1.5">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.admin.config.dailyBudget') }}</span>
            <input v-model.number="config.daily_budget_usd" type="number" min="0" max="10000" step="0.01" class="input" />
            <span class="block text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.config.budgetHint') }}</span>
          </label>
          <button type="button" class="btn btn-primary" :disabled="savingConfig" @click="saveConfig">
            {{ savingConfig ? t('common.saving') : t('common.save') }}
          </button>
        </div>
        <p class="mt-4 rounded-2xl border border-amber-200 bg-amber-50 p-3 text-xs leading-5 text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
          {{ t('pelican.admin.budgetNotice') }}
        </p>
      </section>

      <section class="space-y-4">
        <div class="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 class="text-xl font-bold text-gray-950 dark:text-white">{{ t('pelican.admin.plans.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.description') }}</p>
          </div>
          <button type="button" class="btn btn-primary" @click="openCreatePlan">
            <Icon name="plus" size="sm" />
            {{ t('pelican.admin.plans.create') }}
          </button>
        </div>

        <div v-if="loadingPlans" class="grid gap-4 lg:grid-cols-2">
          <div v-for="index in 2" :key="index" class="h-64 animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800"></div>
        </div>
        <div v-else-if="plans.length" class="grid gap-4 lg:grid-cols-2">
          <article
            v-for="plan in plans"
            :key="plan.id"
            class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-mono text-xs font-semibold text-gray-500 dark:text-dark-300">#{{ plan.id }}</span>
                  <span
                    class="rounded-full px-2 py-1 text-[11px] font-bold"
                    :class="plan.enabled
                      ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
                      : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-200'"
                  >
                    {{ plan.enabled ? t('pelican.admin.plans.enabled') : t('pelican.admin.plans.disabled') }}
                  </span>
                  <span
                    class="rounded-full px-2 py-1 text-[11px] font-bold"
                    :class="plan.published
                      ? 'bg-blue-50 text-blue-700 dark:bg-blue-950/40 dark:text-blue-300'
                      : 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'"
                  >
                    {{ plan.published ? t('pelican.admin.plans.isPublished') : t('pelican.admin.plans.unpublished') }}
                  </span>
                </div>
                <h3 class="mt-2 break-words font-mono text-lg font-bold text-gray-950 dark:text-white">{{ plan.model }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ plan.group_name }}</p>
              </div>
              <div class="flex shrink-0 gap-1">
                <button type="button" class="btn btn-ghost btn-icon" :title="t('common.edit')" @click="openEditPlan(plan)">
                  <Icon name="edit" size="sm" />
                </button>
                <button type="button" class="btn btn-ghost btn-icon text-rose-600" :title="t('common.delete')" @click="removePlan(plan)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>

            <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
              <div><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.effort') }}</dt><dd class="mt-0.5 font-medium">{{ plan.effort || t('pelican.effort.unspecified') }}</dd></div>
              <div><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.protocol') }}</dt><dd class="mt-0.5 font-medium">{{ t(`pelican.protocol.${plan.protocol}`) }}</dd></div>
              <div><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.interval') }}</dt><dd class="mt-0.5 tabular-nums">{{ t('pelican.admin.plans.minutes', { count: plan.interval_minutes }) }}</dd></div>
              <div><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.outputCap') }}</dt><dd class="mt-0.5 tabular-nums">{{ t('pelican.fields.tokens', { count: plan.max_output_tokens }) }}</dd></div>
              <div><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.budget') }}</dt><dd class="mt-0.5 tabular-nums">${{ plan.budget_usd.toFixed(6) }}</dd></div>
              <div><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.lastRun') }}</dt><dd class="mt-0.5">{{ plan.last_run_at ? formatDate(plan.last_run_at) : '—' }}</dd></div>
            </dl>

            <div class="mt-4 rounded-2xl bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-dark-300">
              <p>{{ t('pelican.admin.plans.nextRun', { time: formatDate(plan.next_run_at) }) }}</p>
              <p v-if="plan.running_until" class="mt-1 font-semibold text-sky-700 dark:text-sky-300">
                {{ t('pelican.admin.plans.runningUntil', { time: formatDate(plan.running_until) }) }}
              </p>
            </div>

            <div class="mt-4 flex flex-wrap gap-2">
              <button
                type="button"
                class="btn btn-primary flex-1"
                :disabled="!canRun || runningPlanID != null"
                :title="!canRun ? t('pelican.admin.plans.enableBeforeRun') : undefined"
                @click="runNow(plan)"
              >
                <Icon name="play" size="sm" />
                {{ runningPlanID === plan.id ? t('pelican.admin.plans.submitting') : t('pelican.admin.plans.runNow') }}
              </button>
              <button type="button" class="btn btn-secondary" @click="openEditPlan(plan)">
                {{ t('common.edit') }}
              </button>
            </div>
          </article>
        </div>
        <div v-else class="rounded-3xl border border-dashed border-gray-300 bg-white p-10 text-center dark:border-dark-600 dark:bg-dark-800">
          <p class="font-semibold text-gray-800 dark:text-dark-100">{{ t('pelican.admin.plans.empty') }}</p>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.emptyHint') }}</p>
        </div>
      </section>

      <section class="space-y-4">
        <div class="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 class="text-xl font-bold text-gray-950 dark:text-white">{{ t('pelican.admin.results.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('pelican.admin.results.description') }}</p>
          </div>
          <button type="button" class="btn btn-secondary" :disabled="loadingResults" @click="loadResults(false)">
            <Icon name="refresh" size="sm" :class="loadingResults ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>

        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <div v-for="stat in resultStats" :key="stat.state" class="rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-dark-300">{{ stat.label }}</p>
            <p class="mt-2 text-2xl font-black tabular-nums text-gray-950 dark:text-white">{{ stat.value }}</p>
          </div>
        </div>

        <div class="grid gap-3 rounded-3xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_auto]">
          <Select
            v-model="resultPlanFilter"
            :options="planFilterOptions"
            :placeholder="t('pelican.admin.results.allPlans')"
            searchable
            clearable
          />
          <Select
            v-model="resultStateFilter"
            :options="stateFilterOptions"
            :placeholder="t('pelican.admin.results.allStates')"
            clearable
          />
          <div class="flex gap-2">
            <button type="button" class="btn btn-primary flex-1" @click="applyResultFilters">
              {{ t('pelican.filters.apply') }}
            </button>
            <button type="button" class="btn btn-secondary" @click="resetResultFilters">
              {{ t('pelican.filters.reset') }}
            </button>
          </div>
        </div>

        <p v-if="resultError" role="alert" class="rounded-2xl bg-rose-50 p-4 text-sm text-rose-700 dark:bg-rose-950/30 dark:text-rose-300">
          {{ resultError }}
        </p>

        <div v-if="loadingResults && !resultPage" class="grid gap-4 md:grid-cols-2">
          <div v-for="index in 4" :key="index" class="h-64 animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800"></div>
        </div>
        <div v-else-if="resultPage?.items.length" class="grid gap-4 md:grid-cols-2">
          <article
            v-for="run in resultPage.items"
            :key="run.id"
            class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span
                    class="rounded-full border px-2 py-1 text-[11px] font-bold"
                    :class="pelicanStateClass(run.state)"
                  >
                    {{ t(`pelican.state.${run.state}`) }}
                  </span>
                  <span class="text-xs text-gray-500 dark:text-dark-300">{{ formatDate(run.created_at) }}</span>
                </div>
                <h3 class="mt-2 break-words font-mono text-base font-bold text-gray-950 dark:text-white">{{ run.frozen.model }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ run.group_name }} · #{{ run.plan_id }}</p>
              </div>
              <span class="shrink-0 text-xs font-semibold tabular-nums text-gray-500 dark:text-dark-300">
                {{ formatPelicanDuration(run.duration_ms) }}
              </span>
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2 text-xs text-gray-600 dark:text-dark-200">
              <p>{{ t('pelican.fields.effort') }}: {{ run.frozen.effort || t('pelican.effort.unspecified') }}</p>
              <p>{{ t('pelican.fields.protocol') }}: {{ t(`pelican.protocol.${run.frozen.protocol}`) }}</p>
              <p>{{ t('pelican.admin.results.account') }}: #{{ run.account_id }}</p>
              <p>{{ t('pelican.admin.results.cost') }}: {{ run.cost_usd == null ? '—' : `$${run.cost_usd.toFixed(6)}` }}</p>
            </div>
            <p v-if="run.error_kind" class="mt-3 text-xs font-medium text-rose-600 dark:text-rose-300">
              {{ t(`pelican.errorKind.${run.error_kind}`) }}
            </p>
            <button type="button" class="btn btn-secondary mt-4 w-full" @click="openResult(run.id)">
              <Icon name="eye" size="sm" />
              {{ t('pelican.actions.details') }}
            </button>
          </article>
        </div>
        <div v-else-if="resultPage" class="rounded-3xl border border-dashed border-gray-300 bg-white p-10 text-center dark:border-dark-600 dark:bg-dark-800">
          <p class="font-semibold text-gray-800 dark:text-dark-100">{{ t('pelican.admin.results.empty') }}</p>
        </div>

        <div v-if="resultPage && resultPage.total > 0" class="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <Pagination
            :page="resultPage.page"
            :total="resultPage.total"
            :page-size="resultPage.page_size"
            :show-page-size-selector="false"
            @update:page="changeResultPage"
          />
        </div>
      </section>

      <PelicanPlanDialog
        :show="planDialogOpen"
        :plan="editingPlan"
        :groups="groups"
        :saving="savingPlan"
        @close="planDialogOpen = false"
        @save="savePlan"
      />
      <PelicanAdminResultDialog
        :show="resultDialogOpen"
        :run-id="selectedResultID"
        @close="resultDialogOpen = false"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import PelicanPlanDialog from '@/components/pelican/PelicanPlanDialog.vue'
import PelicanAdminResultDialog from '@/components/pelican/PelicanAdminResultDialog.vue'
import { formatPelicanDuration, pelicanStateClass } from '@/components/pelican/pelicanFormat'
import {
  deletePelicanPlan,
  getPelicanAdminResults,
  getPelicanConfig,
  getPelicanPlans,
  runPelicanPlan,
  savePelicanConfig,
  savePelicanPlan,
} from '@/api/pelican'
import { adminAPI } from '@/api/admin'
import { operationsUUID } from '@/utils/operationsUUID'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { AdminGroup } from '@/types'
import type {
  PelicanAdminPage,
  PelicanConfig,
  PelicanPlan,
  PelicanPlanInput,
  PelicanRunState,
} from '@/types/pelican'

const { t } = useI18n()
const pageError = ref('')
const resultError = ref('')
const config = reactive<PelicanConfig>({ enabled: false, daily_budget_usd: 0 })
const savingConfig = ref(false)
const plans = ref<PelicanPlan[]>([])
const groups = ref<AdminGroup[]>([])
const loadingPlans = ref(false)
const planDialogOpen = ref(false)
const editingPlan = ref<PelicanPlan | null>(null)
const savingPlan = ref(false)
const runningPlanID = ref<number | null>(null)
const submitIDs = reactive<Record<string, string>>({})
const resultPage = ref<PelicanAdminPage | null>(null)
const loadingResults = ref(false)
const resultPageIndex = ref(1)
const resultPlanFilter = ref<number | null>(null)
const resultStateFilter = ref<PelicanRunState | ''>('')
const resultDialogOpen = ref(false)
const selectedResultID = ref<string | null>(null)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let pollAttempts = 0

const canRun = computed(() => config.enabled && config.daily_budget_usd > 0)
const planFilterOptions = computed(() =>
  plans.value.map((plan) => ({ value: plan.id, label: `#${plan.id} · ${plan.model}` })),
)
const stateFilterOptions = [
  { value: 'completed', label: t('pelican.state.completed') },
  { value: 'failed', label: t('pelican.state.failed') },
  { value: 'unknown', label: t('pelican.state.unknown') },
  { value: 'running', label: t('pelican.state.running') },
]
const resultStats = computed(() => [
  { state: 'completed', label: t('pelican.state.completed'), value: resultPage.value?.stats.completed || 0 },
  { state: 'failed', label: t('pelican.state.failed'), value: resultPage.value?.stats.failed || 0 },
  { state: 'unknown', label: t('pelican.state.unknown'), value: resultPage.value?.stats.unknown || 0 },
  { state: 'running', label: t('pelican.state.running'), value: resultPage.value?.stats.running || 0 },
])

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function clearPoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

function schedulePoll() {
  clearPoll()
  if (
    document.visibilityState !== 'visible'
    || (!resultPage.value?.stats.running && !plans.value.some((plan) => plan.running_until))
    || pollAttempts >= 60
  ) return
  pollTimer = setTimeout(async () => {
    pollAttempts += 1
    await Promise.all([loadPlans(), loadResults(true)])
  }, 10_000)
}

async function loadConfig() {
  try {
    Object.assign(config, await getPelicanConfig())
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('common.error'))
  }
}

async function loadGroups() {
  try {
    const all = await adminAPI.groups.getAll()
    groups.value = all.filter((group) => group.platform === 'openai' && group.status === 'active')
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.groupsLoadFailed'))
  }
}

async function loadPlans() {
  loadingPlans.value = true
  try {
    plans.value = await getPelicanPlans()
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.loadFailed'))
  } finally {
    loadingPlans.value = false
    schedulePoll()
  }
}

async function loadResults(background: boolean) {
  if (!background) loadingResults.value = true
  resultError.value = ''
  try {
    resultPage.value = await getPelicanAdminResults({
      page: resultPageIndex.value,
      page_size: 12,
      ...(resultPlanFilter.value ? { plan_id: resultPlanFilter.value } : {}),
      ...(resultStateFilter.value ? { state: resultStateFilter.value } : {}),
    })
  } catch (error) {
    resultError.value = extractApiErrorMessage(error, t('pelican.admin.results.loadFailed'))
  } finally {
    loadingResults.value = false
    schedulePoll()
  }
}

async function saveConfig() {
  savingConfig.value = true
  pageError.value = ''
  try {
    Object.assign(config, await savePelicanConfig({ ...config }))
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.config.saveFailed'))
  } finally {
    savingConfig.value = false
  }
}

function openCreatePlan() {
  editingPlan.value = null
  planDialogOpen.value = true
}

function openEditPlan(plan: PelicanPlan) {
  editingPlan.value = plan
  planDialogOpen.value = true
}

async function savePlan(input: PelicanPlanInput) {
  savingPlan.value = true
  pageError.value = ''
  try {
    await savePelicanPlan(editingPlan.value?.id ?? null, input)
    planDialogOpen.value = false
    await loadPlans()
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.saveFailed'))
  } finally {
    savingPlan.value = false
  }
}

async function removePlan(plan: PelicanPlan) {
  if (!window.confirm(t('pelican.admin.plans.deleteConfirm', { model: plan.model }))) return
  pageError.value = ''
  try {
    await deletePelicanPlan(plan.id)
    delete submitIDs[String(plan.id)]
    await loadPlans()
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.deleteFailed'))
  }
}

async function runNow(plan: PelicanPlan) {
  if (!canRun.value || runningPlanID.value != null) return
  const key = String(plan.id)
  if (!submitIDs[key]) submitIDs[key] = operationsUUID()
  runningPlanID.value = plan.id
  pageError.value = ''
  try {
    await runPelicanPlan(plan.id, submitIDs[key])
    delete submitIDs[key]
    resultPageIndex.value = 1
    await Promise.all([loadPlans(), loadResults(false)])
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.runFailed'))
  } finally {
    runningPlanID.value = null
  }
}

function applyResultFilters() {
  resultPageIndex.value = 1
  pollAttempts = 0
  void loadResults(false)
}

function resetResultFilters() {
  resultPlanFilter.value = null
  resultStateFilter.value = ''
  applyResultFilters()
}

function changeResultPage(page: number) {
  resultPageIndex.value = page
  pollAttempts = 0
  void loadResults(false)
}

function openResult(id: string) {
  selectedResultID.value = id
  resultDialogOpen.value = true
}

function handleVisibilityChange() {
  if (document.visibilityState === 'visible') schedulePoll()
  else clearPoll()
}

onMounted(async () => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  await Promise.all([loadConfig(), loadGroups(), loadPlans(), loadResults(false)])
})

onBeforeUnmount(() => {
  clearPoll()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

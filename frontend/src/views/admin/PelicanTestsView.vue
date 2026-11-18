<template>
  <AppLayout viewport>
    <div ref="adminRoot" class="pelican-admin flex h-full min-h-0 w-full flex-col gap-2">
      <header class="flex shrink-0 flex-wrap items-center justify-between gap-2">
        <div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
          <h1 class="text-xl font-bold tracking-tight text-gray-950 dark:text-white">
            {{ t('pelican.admin.title') }}
          </h1>
          <p class="hidden min-w-0 truncate text-xs text-gray-500 dark:text-dark-300 xl:block">
            {{ t('pelican.admin.description') }}
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :aria-label="t('pelican.rules.title')"
            @click="openRules"
          >
            <Icon name="infoCircle" size="sm" />
            <span class="hidden sm:inline">{{ t('pelican.rules.title') }}</span>
          </button>
          <router-link
            to="/pelican"
            class="btn btn-secondary btn-sm"
            :aria-label="t('pelican.admin.openUserPage')"
          >
            <Icon name="eye" size="sm" />
            <span class="hidden sm:inline">{{ t('pelican.admin.openUserPage') }}</span>
          </router-link>
          <button type="button" class="btn btn-primary btn-sm" @click="openCreatePlan">
            <Icon name="plus" size="sm" />
            {{ t('pelican.admin.plans.create') }}
          </button>
        </div>
      </header>

      <form
        class="pelican-config flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-gray-200 bg-white px-3 py-2 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        @submit.prevent="saveConfig"
      >
        <label class="flex items-center gap-2 text-xs font-medium text-gray-700 dark:text-dark-100">
          <Toggle v-model="config.enabled" />
          <span>{{ t('pelican.admin.config.enabled') }}</span>
        </label>
        <span
          class="inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[11px] font-semibold"
          :class="savedCanRun
            ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
            : 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
          {{ savedCanRun ? t('pelican.admin.config.active') : t('pelican.admin.config.paused') }}
        </span>
        <label class="flex items-center gap-1.5 text-xs text-gray-600 dark:text-dark-200">
          <span>{{ t('pelican.admin.config.dailyBudget') }}</span>
          <span>$</span>
          <input
            v-model.number="config.daily_budget_usd"
            type="number"
            min="0"
            max="10000"
            step="0.01"
            class="input h-7 w-24 px-2 py-1 text-xs tabular-nums"
          />
        </label>
        <p class="min-w-40 flex-1 text-[11px] text-gray-500 dark:text-dark-300 md:hidden xl:block">
          {{ config.enabled && config.daily_budget_usd > 0
            ? t('pelican.admin.config.enabledHint')
            : t('pelican.admin.config.budgetHint') }}
        </p>
        <button type="submit" class="btn btn-primary btn-sm ml-auto" :disabled="savingConfig">
          {{ savingConfig ? t('common.saving') : t('common.save') }}
        </button>
      </form>

      <p
        v-if="pageError"
        role="alert"
        class="shrink-0 rounded-lg border border-rose-200 bg-rose-50 px-3 py-1.5 text-xs text-rose-700 dark:border-rose-900/60 dark:bg-rose-950/30 dark:text-rose-300"
      >
        {{ pageError }}
      </p>

      <section class="pelican-panel flex shrink-0 flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex min-h-8 shrink-0 items-center justify-between gap-2 px-3">
          <h2 class="text-sm font-semibold text-gray-950 dark:text-white">
            {{ t('pelican.admin.plans.title') }}
            <span class="ml-1 font-normal text-gray-500 dark:text-dark-300">{{ plans.length }}</span>
          </h2>
          <span class="hidden truncate text-[11px] text-gray-500 dark:text-dark-300 md:inline">
            {{ t('pelican.admin.plans.description') }}
          </span>
        </div>
        <div class="overflow-hidden">
          <table class="pelican-table w-full table-fixed">
            <thead>
              <tr>
                <th>{{ t('pelican.fields.group') }} / {{ t('pelican.fields.model') }}</th>
                <th class="hidden lg:table-cell">{{ t('pelican.fields.effort') }} / {{ t('pelican.fields.protocol') }}</th>
                <th class="hidden xl:table-cell">{{ t('pelican.admin.plans.interval') }} / {{ t('pelican.admin.plans.budget') }}</th>
                <th class="hidden md:table-cell">{{ t('pelican.admin.plans.scheduleEnabled') }}</th>
                <th class="hidden md:table-cell">{{ t('pelican.admin.plans.published') }}</th>
                <th class="hidden md:table-cell">{{ t('pelican.admin.plans.lastRun') }}</th>
                <th class="text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="loadingPlans">
                <td :colspan="7" class="py-8 text-center text-gray-500 dark:text-dark-300">
                  {{ t('common.loading') }}
                </td>
              </tr>
              <tr v-for="plan in planPageItems" v-else :key="plan.id" class="pelican-row pelican-plan-row">
                <td class="min-w-0">
                  <span class="pelican-cell-title">{{ plan.group_name }}</span>
                  <code class="pelican-subcell pelican-desktop-model">{{ plan.model }}</code>
                  <span class="pelican-subcell pelican-mobile-status">
                    {{ plan.enabled ? t('pelican.admin.plans.enabled') : t('pelican.admin.plans.disabled') }}
                    · {{ plan.published ? t('pelican.admin.plans.isPublished') : t('pelican.admin.plans.unpublished') }}
                    · {{ plan.model }}
                  </span>
                </td>
                <td class="hidden lg:table-cell">
                  <span class="pelican-cell-title">{{ plan.effort || t('pelican.effort.unspecified') }}</span>
                  <span class="pelican-subcell">{{ t(`pelican.protocol.${plan.protocol}`) }}</span>
                </td>
                <td class="hidden xl:table-cell">
                  <span class="pelican-cell-title">{{ t('pelican.admin.plans.minutes', { count: plan.interval_minutes }) }}</span>
                  <span class="pelican-subcell tabular-nums">${{ plan.budget_usd.toFixed(2) }} · {{ t('pelican.fields.tokens', { count: plan.max_output_tokens }) }}</span>
                </td>
                <td class="hidden md:table-cell">
                  <fieldset
                    class="m-0 flex min-w-0 items-center border-0 p-0"
                    :class="{ 'opacity-50': planBusy(plan.id) }"
                    :disabled="planBusy(plan.id)"
                  >
                    <Toggle
                      :model-value="plan.enabled"
                      :aria-label="`${plan.group_name} · ${plan.model} · ${t('pelican.admin.plans.scheduleEnabled')}`"
                      @update:model-value="updatePlanFlag(plan, 'enabled', $event)"
                    />
                  </fieldset>
                </td>
                <td class="hidden md:table-cell">
                  <fieldset
                    class="m-0 flex min-w-0 items-center border-0 p-0"
                    :class="{ 'opacity-50': planBusy(plan.id) }"
                    :disabled="planBusy(plan.id)"
                  >
                    <Toggle
                      :model-value="plan.published"
                      :aria-label="`${plan.group_name} · ${plan.model} · ${t('pelican.admin.plans.published')}`"
                      @update:model-value="updatePlanFlag(plan, 'published', $event)"
                    />
                  </fieldset>
                </td>
                <td class="hidden md:table-cell">
                  <span class="pelican-cell-title">{{ plan.last_run_at ? formatDate(plan.last_run_at) : '—' }}</span>
                  <span v-if="plan.running_until" class="pelican-subcell text-sky-600 dark:text-sky-300">
                    {{ t('pelican.admin.plans.runningUntil', { time: formatTime(plan.running_until) }) }}
                  </span>
                  <span v-else-if="!plan.enabled || !savedCanRun" class="pelican-subcell">{{ t('pelican.admin.plans.schedulePaused') }}</span>
                  <span v-else-if="plan.budget_usd > savedConfig.daily_budget_usd" class="pelican-subcell text-amber-700 dark:text-amber-300">{{ t('pelican.admin.plans.scheduleOverBudget') }}</span>
                  <span v-else class="pelican-subcell" :title="formatDate(plan.next_run_at)">{{ t('pelican.admin.plans.nextRun', { time: formatSchedule(plan.next_run_at) }) }}</span>
                </td>
                <td>
                  <div class="flex items-center justify-end gap-1">
                    <button
                      type="button"
                      class="pelican-icon-action"
                      :title="t('common.edit')"
                      :aria-label="`${t('common.edit')} · ${plan.group_name} · ${plan.model}`"
                      @click="openEditPlan(plan)"
                    >
                      <Icon name="edit" size="sm" />
                    </button>
                    <button
                      type="button"
                      class="pelican-icon-action"
                      :disabled="!savedCanRun || runningPlanID != null || planBusy(plan.id)"
                      :title="savedCanRun ? t('pelican.admin.plans.runNow') : t('pelican.admin.plans.enableBeforeRun')"
                      :aria-label="`${t('pelican.admin.plans.runNow')} · ${plan.group_name} · ${plan.model}`"
                      @click="runNow(plan)"
                    >
                      <Icon name="play" size="sm" />
                    </button>
                    <button
                      type="button"
                      class="pelican-icon-action text-rose-600 dark:text-rose-300"
                      :title="t('common.delete')"
                      :aria-label="`${t('common.delete')} · ${plan.group_name} · ${plan.model}`"
                      @click="removePlan(plan)"
                    >
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!loadingPlans && !plans.length">
                <td :colspan="7" class="py-8 text-center text-gray-500 dark:text-dark-300">
                  {{ t('pelican.admin.plans.empty') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="shrink-0 border-t border-gray-100 px-3 dark:border-dark-700">
          <PelicanCompactPager
            :page="planPageIndex"
            :total="plans.length"
            :page-size="planPageSize"
            @update:page="planPageIndex = $event"
          />
        </div>
      </section>

      <section class="pelican-panel flex shrink-0 flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex min-h-9 shrink-0 flex-wrap items-center justify-between gap-2 px-3">
          <div class="flex min-w-0 items-center gap-2">
            <h2 class="text-sm font-semibold text-gray-950 dark:text-white">
              {{ t('pelican.admin.results.title') }}
              <span class="ml-1 font-normal text-gray-500 dark:text-dark-300">{{ resultPage?.total || 0 }}</span>
            </h2>
            <span v-if="resultPage" class="hidden items-center gap-2 text-[10px] text-gray-500 dark:text-dark-300 xl:flex">
              <span>{{ t('pelican.state.completed') }} {{ resultPage.stats.completed }}</span>
              <span>{{ t('pelican.state.failed') }} {{ resultPage.stats.failed }}</span>
              <span>{{ t('pelican.state.unknown') }} {{ resultPage.stats.unknown }}</span>
              <span>{{ t('pelican.state.running') }} {{ resultPage.stats.running }}</span>
            </span>
          </div>
          <div class="flex items-center gap-1.5">
            <Select
              v-model="resultPlanFilter"
              :options="planFilterOptions"
              :placeholder="t('pelican.admin.results.allPlans')"
              searchable
              clearable
              class="pelican-filter-select w-32"
              @change="applyResultFilters"
            />
            <Select
              v-model="resultStateFilter"
              :options="stateFilterOptions"
              :placeholder="t('pelican.admin.results.allStates')"
              clearable
              class="pelican-filter-select w-28"
              @change="applyResultFilters"
            />
            <button
              type="button"
              class="pelican-icon-action"
              :disabled="loadingResults"
              :title="t('common.refresh')"
              :aria-label="t('common.refresh')"
              @click="loadResults(false)"
            >
              <Icon name="refresh" size="sm" :class="loadingResults ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
        <p
          v-if="resultError"
          role="alert"
          class="mx-3 mb-1 shrink-0 rounded-lg bg-rose-50 px-2 py-1 text-xs text-rose-700 dark:bg-rose-950/30 dark:text-rose-300"
        >
          {{ resultError }}
        </p>
        <div class="overflow-hidden">
          <table class="pelican-table w-full table-fixed">
            <thead>
              <tr>
                <th>{{ t('pelican.fields.group') }} / {{ t('pelican.fields.model') }}</th>
                <th>{{ t('common.status') }}</th>
                <th class="hidden md:table-cell">{{ t('pelican.admin.results.terminal') }}</th>
                <th class="hidden lg:table-cell">{{ t('pelican.admin.results.cost') }}</th>
                <th class="hidden md:table-cell">{{ t('pelican.admin.plans.published') }}</th>
                <th class="text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="loadingResults && !resultPage">
                <td :colspan="6" class="py-8 text-center text-gray-500 dark:text-dark-300">
                  {{ t('common.loading') }}
                </td>
              </tr>
              <tr v-for="run in resultPage?.items || []" v-else :key="run.id" class="pelican-row">
                <td class="min-w-0">
                  <span class="pelican-cell-title">{{ run.group_name }}</span>
                  <span class="pelican-subcell">{{ run.frozen.model }} · {{ formatDate(run.created_at) }}</span>
                </td>
                <td>
                  <span class="inline-flex items-center gap-1.5 whitespace-nowrap text-[11px] font-semibold" :class="stateTextClass(run.state)">
                    <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
                    {{ t(`pelican.state.${run.state}`) }}
                  </span>
                  <span class="pelican-subcell">{{ formatPelicanDuration(run.duration_ms) }}</span>
                </td>
                <td class="hidden md:table-cell">
                  <span class="pelican-cell-title">{{ run.error_kind ? t(`pelican.errorKind.${run.error_kind}`) : run.terminal || '—' }}</span>
                  <span class="pelican-subcell">#{{ run.plan_id }} · #{{ run.account_id }}</span>
                </td>
                <td class="hidden lg:table-cell">
                  <span class="pelican-cell-title tabular-nums">{{ run.cost_usd == null ? t('pelican.admin.results.unknownCost') : `$${run.cost_usd.toFixed(4)}` }}</span>
                  <span class="pelican-subcell tabular-nums">${{ run.reserved_usd.toFixed(4) }}</span>
                </td>
                <td class="hidden md:table-cell">
                  <span class="pelican-cell-title">{{ run.published ? t('pelican.admin.plans.isPublished') : t('pelican.admin.plans.unpublished') }}</span>
                </td>
                <td>
                  <div class="flex justify-end">
                    <button type="button" class="pelican-text-action" @click="openResult(run.id)">
                      {{ t('pelican.actions.details') }}
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!loadingResults && resultPage && !resultPage.items.length">
                <td :colspan="6" class="py-8 text-center text-gray-500 dark:text-dark-300">
                  {{ t('pelican.admin.results.empty') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="shrink-0 border-t border-gray-100 px-3 dark:border-dark-700">
          <PelicanCompactPager
            :page="resultPageIndex"
            :total="resultPage?.total || 0"
            :page-size="resultPageSize"
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
      <PelicanAdminResultDialog :show="resultDialogOpen" :run-id="selectedResultID" @close="resultDialogOpen = false" />
      <PelicanRulesDialog :show="rulesOpen" :prompt="rulesPrompt" :prompt-version="rulesPromptVersion" @close="rulesOpen = false" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import PelicanCompactPager from '@/components/pelican/PelicanCompactPager.vue'
import PelicanPlanDialog from '@/components/pelican/PelicanPlanDialog.vue'
import PelicanAdminResultDialog from '@/components/pelican/PelicanAdminResultDialog.vue'
import PelicanRulesDialog from '@/components/pelican/PelicanRulesDialog.vue'
import { formatPelicanDuration } from '@/components/pelican/pelicanFormat'
import { usePelicanRefresh } from '@/components/pelican/usePelicanRefresh'
import {
  deletePelicanPlan,
  getPelicanAdminResults,
  getPelicanConfig,
  getPelicanPlans,
  getPelicanResults,
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
import {
  canRunPelicanTests,
  clampPage,
  pelicanPlanInput,
  pelicanSubmitID,
  pelicanViewportRows,
} from './pelicanAdmin'

const { t } = useI18n()
const pageError = ref('')
const resultError = ref('')
const config = reactive<PelicanConfig>({ enabled: false, daily_budget_usd: 0 })
const savedConfig = ref<PelicanConfig>({ enabled: false, daily_budget_usd: 0 })
const savingConfig = ref(false)
const plans = ref<PelicanPlan[]>([])
const groups = ref<AdminGroup[]>([])
const loadingPlans = ref(false)
const planDialogOpen = ref(false)
const editingPlan = ref<PelicanPlan | null>(null)
const savingPlan = ref(false)
const togglingPlanIDs = ref<Set<number>>(new Set())
const runningPlanID = ref<number | null>(null)
const submitIDs = reactive<Record<string, string>>({})
const resultPage = ref<PelicanAdminPage | null>(null)
const loadingResults = ref(false)
const resultPageIndex = ref(1)
const resultPlanFilter = ref<number | null>(null)
const resultStateFilter = ref<PelicanRunState | null>(null)
const resultDialogOpen = ref(false)
const selectedResultID = ref<string | null>(null)
const rulesOpen = ref(false)
const rulesPrompt = ref('')
const rulesPromptVersion = ref('pelican-bicycle-v1')
const adminRoot = ref<HTMLElement | null>(null)
const viewport = reactive({ width: window.innerWidth, height: window.innerHeight })
const planPageIndex = ref(1)
let mounted = false
let layoutObserver: ResizeObserver | null = null
let planRequestSequence = 0
let resultRequestSequence = 0
const refresh = usePelicanRefresh(
  () => void Promise.all([loadPlans(true), loadResults(true)]),
  () => resultPage.value?.stats.running || plans.value.some((plan) => plan.running_until)
    ? 10_000
    : 30_000,
)

const viewportRows = computed(() => pelicanViewportRows(viewport.width, viewport.height))
const planPageSize = computed(() => viewportRows.value.plans)
const resultPageSize = computed(() => viewportRows.value.results)
const planPageItems = computed(() => {
  const start = (planPageIndex.value - 1) * planPageSize.value
  return plans.value.slice(start, start + planPageSize.value)
})
const planFilterOptions = computed(() =>
  plans.value.map((plan) => ({ value: plan.id, label: `#${plan.id} · ${plan.model}` })),
)
const stateFilterOptions = [
  { value: 'completed', label: t('pelican.state.completed') },
  { value: 'failed', label: t('pelican.state.failed') },
  { value: 'unknown', label: t('pelican.state.unknown') },
  { value: 'running', label: t('pelican.state.running') },
]
const savedCanRun = computed(() => canRunPelicanTests(savedConfig.value))

watch(
  [() => plans.value.length, planPageSize],
  () => {
    planPageIndex.value = clampPage(planPageIndex.value, plans.value.length, planPageSize.value)
  },
  { immediate: true },
)

watch(resultPageSize, () => {
  if (!mounted) return
  resultPageIndex.value = 1
  void loadResults(false)
})

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

function formatSchedule(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString(undefined, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function stateTextClass(state: PelicanRunState): string {
  if (state === 'completed') return 'text-emerald-600 dark:text-emerald-300'
  if (state === 'failed') return 'text-rose-600 dark:text-rose-300'
  if (state === 'unknown') return 'text-amber-600 dark:text-amber-300'
  return 'text-sky-600 dark:text-sky-300'
}

function planBusy(id: number): boolean {
  return togglingPlanIDs.value.has(id)
}

function setPlanBusy(id: number, busy: boolean) {
  const next = new Set(togglingPlanIDs.value)
  if (busy) next.add(id)
  else next.delete(id)
  togglingPlanIDs.value = next
}

async function loadConfig() {
  try {
    const loaded = await getPelicanConfig()
    Object.assign(config, loaded)
    savedConfig.value = { ...loaded }
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('common.error'))
  }
}

async function openRules() {
  rulesOpen.value = true
  if (rulesPrompt.value) return
  try {
    const result = await getPelicanResults({ page: 1, page_size: 1 })
    rulesPrompt.value = result.prompt
    rulesPromptVersion.value = result.prompt_version
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

async function loadPlans(background = false) {
  refresh.clear()
  const requestID = ++planRequestSequence
  if (!background) loadingPlans.value = true
  try {
    const loaded = await getPelicanPlans()
    if (requestID === planRequestSequence) plans.value = loaded
  } catch (error) {
    if (requestID === planRequestSequence) {
      pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.loadFailed'))
    }
  } finally {
    if (requestID === planRequestSequence) {
      loadingPlans.value = false
      planPageIndex.value = clampPage(planPageIndex.value, plans.value.length, planPageSize.value)
      refresh.schedule()
    }
  }
}

async function loadResults(background: boolean) {
  refresh.clear()
  const requestID = ++resultRequestSequence
  const requestedPage = resultPageIndex.value
  if (!background) loadingResults.value = true
  if (!background) resultPage.value = null
  resultError.value = ''
  try {
    const loaded = await getPelicanAdminResults({
      page: resultPageIndex.value,
      page_size: resultPageSize.value,
      ...(resultPlanFilter.value ? { plan_id: resultPlanFilter.value } : {}),
      ...(resultStateFilter.value ? { state: resultStateFilter.value } : {}),
    })
    if (requestID === resultRequestSequence) {
      const lastPage = Math.max(1, loaded.pages)
      if (requestedPage > lastPage) {
        resultPageIndex.value = lastPage
        void loadResults(background)
        return
      }
      resultPage.value = loaded
    }
  } catch (error) {
    if (requestID === resultRequestSequence) {
      resultError.value = extractApiErrorMessage(error, t('pelican.admin.results.loadFailed'))
    }
  } finally {
    if (requestID === resultRequestSequence) {
      loadingResults.value = false
      refresh.schedule()
    }
  }
}

async function saveConfig() {
  refresh.clear()
  savingConfig.value = true
  pageError.value = ''
  try {
    const saved = await savePelicanConfig({ ...config })
    Object.assign(config, saved)
    savedConfig.value = { ...saved }
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.config.saveFailed'))
  } finally {
    savingConfig.value = false
    refresh.schedule()
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
  refresh.clear()
  planRequestSequence += 1
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
    refresh.schedule()
  }
}

async function updatePlanFlag(
  plan: PelicanPlan,
  field: 'enabled' | 'published',
  value: boolean,
) {
  if (planBusy(plan.id)) return
  refresh.clear()
  planRequestSequence += 1
  setPlanBusy(plan.id, true)
  pageError.value = ''
  try {
    const input = pelicanPlanInput(plan, { [field]: value })
    const saved = await savePelicanPlan(plan.id, input)
    plans.value = plans.value.map((item) => item.id === plan.id ? saved : item)
    await loadPlans(true)
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.saveFailed'))
  } finally {
    setPlanBusy(plan.id, false)
    refresh.schedule()
  }
}

async function removePlan(plan: PelicanPlan) {
  if (!window.confirm(t('pelican.admin.plans.deleteConfirm', { model: plan.model }))) return
  refresh.clear()
  planRequestSequence += 1
  pageError.value = ''
  try {
    await deletePelicanPlan(plan.id)
    delete submitIDs[String(plan.id)]
    await loadPlans()
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.deleteFailed'))
  } finally {
    refresh.schedule()
  }
}

async function runNow(plan: PelicanPlan) {
  if (!savedCanRun.value || runningPlanID.value != null) return
  refresh.clear()
  planRequestSequence += 1
  const key = String(plan.id)
  submitIDs[key] = pelicanSubmitID(submitIDs[key], operationsUUID)
  runningPlanID.value = plan.id
  pageError.value = ''
  try {
    await runPelicanPlan(plan.id, submitIDs[key])
    delete submitIDs[key]
    resultPageIndex.value = 1
    await Promise.all([loadPlans(true), loadResults(false)])
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('pelican.admin.plans.runFailed'))
  } finally {
    runningPlanID.value = null
    refresh.schedule()
  }
}

function applyResultFilters() {
  resultPageIndex.value = 1
  void loadResults(false)
}

function changeResultPage(page: number) {
  resultPageIndex.value = page
  void loadResults(false)
}

function openResult(id: string) {
  selectedResultID.value = id
  resultDialogOpen.value = true
}

function handleResize() {
  viewport.width = window.innerWidth
  viewport.height = adminRoot.value?.clientHeight ?? window.innerHeight
}

onMounted(async () => {
  window.addEventListener('resize', handleResize)
  if (adminRoot.value) {
    viewport.height = adminRoot.value.clientHeight || window.innerHeight
    layoutObserver = new ResizeObserver(([entry]) => {
      viewport.width = window.innerWidth
      viewport.height = entry.contentRect.height
    })
    layoutObserver.observe(adminRoot.value)
  }
  mounted = true
  await Promise.all([loadConfig(), loadGroups(), loadPlans(), loadResults(false)])
})

onBeforeUnmount(() => {
  mounted = false
  layoutObserver?.disconnect()
  planRequestSequence += 1
  resultRequestSequence += 1
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.pelican-admin :deep(.btn-sm) {
  min-height: 28px;
  padding: 0.25rem 0.6rem;
  font-size: 0.75rem;
}

.pelican-table {
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 0.72rem;
}

.pelican-table th {
  height: 27px;
  border-bottom: 1px solid rgb(229 231 235);
  background: rgb(249 250 251);
  padding: 0.28rem 0.55rem;
  color: rgb(100 116 139);
  font-size: 0.64rem;
  font-weight: 500;
  text-align: left;
  white-space: nowrap;
}

.pelican-table td {
  height: 38px;
  border-top: 1px solid rgb(243 244 246);
  padding: 0.3rem 0.55rem;
  vertical-align: middle;
}

.pelican-table tbody tr:first-child td {
  border-top: 0;
}

.pelican-table th:first-child {
  width: 28%;
}

.pelican-table th:nth-child(2) {
  width: 17%;
}

.pelican-table th:nth-child(3) {
  width: 18%;
}

.pelican-table th:nth-child(4),
.pelican-table th:nth-child(5) {
  width: 8%;
}

.pelican-table th:nth-child(6) {
  width: 14%;
}

.pelican-table th:last-child {
  width: 14%;
}

.pelican-cell-title,
.pelican-subcell {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pelican-cell-title {
  color: rgb(17 24 39);
  font-weight: 500;
}

.pelican-subcell {
  margin-top: 1px;
  color: rgb(100 116 139);
  font-size: 0.64rem;
}

@media (max-width: 767px) {
  .pelican-desktop-model {
    display: none;
  }
}

@media (min-width: 768px) {
  .pelican-mobile-status {
    display: none;
  }
}

.pelican-icon-action {
  display: inline-flex;
  height: 26px;
  width: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  color: rgb(100 116 139);
}

.pelican-icon-action:hover:not(:disabled) {
  background: rgb(243 244 246);
  color: rgb(15 23 42);
}

.pelican-icon-action:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.pelican-text-action {
  color: rgb(79 70 229);
  font-size: 0.7rem;
  font-weight: 500;
  white-space: nowrap;
}

.pelican-text-action:hover {
  text-decoration: underline;
}

.pelican-filter-select :deep(.select-trigger) {
  min-height: 27px;
  padding: 0.2rem 0.45rem;
  font-size: 0.68rem;
}

.dark .pelican-table th {
  border-color: rgb(51 65 85);
  background: rgb(15 23 42);
  color: rgb(148 163 184);
}

.dark .pelican-table td {
  border-color: rgb(30 41 59);
}

.dark .pelican-cell-title {
  color: rgb(241 245 249);
}

.dark .pelican-subcell {
  color: rgb(148 163 184);
}

.dark .pelican-icon-action {
  color: rgb(148 163 184);
}

.dark .pelican-icon-action:hover:not(:disabled) {
  background: rgb(30 41 59);
  color: white;
}

@media (max-width: 900px) {
  .pelican-table th:first-child {
    width: 42%;
  }

  .pelican-table th:nth-child(2) {
    width: 20%;
  }

  .pelican-table th:last-child {
    width: 22%;
  }
}

@media (max-width: 620px) {
  .pelican-config {
    row-gap: 0.35rem;
  }

  .pelican-table th,
  .pelican-table td {
    padding-left: 0.4rem;
    padding-right: 0.4rem;
  }

  .pelican-table td {
    height: 42px;
  }

  .pelican-plan-row td {
    height: 40px;
  }

  .pelican-table th:first-child {
    width: auto;
  }

  .pelican-table th:last-child {
    width: 104px;
  }

  .pelican-filter-select {
    width: 6.75rem;
  }
}
</style>

<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" role="status" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <div v-else-if="statsError" role="alert" class="card flex flex-wrap items-center justify-between gap-3 p-6">
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('dashboard.statsLoadFailed') }}</p>
        <button type="button" class="btn btn-secondary" @click="loadStats">{{ t('common.refresh') }}</button>
      </div>
      <UserDashboardStats v-else-if="stats" :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
      <UserDashboardCharts
        v-model:startDate="startDate"
        v-model:endDate="endDate"
        v-model:granularity="granularity"
        :loading="loadingCharts"
        :trend="trendData"
        :models="modelStats"
        :model-error="modelError"
        :trend-error="trendError"
        @dateRangeChange="loadCharts"
        @granularityChange="loadCharts"
        @refresh="refreshAll"
        @retry="loadCharts"
      />
      <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" :error="recentError" @retry="loadRecent" /></div>
        <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem } from '@/types'
import { getMyPlatformQuotas } from '@/api/user'
import { formatDateLocalInput } from '@/utils/format'

const { t } = useI18n()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)
const statsError = ref(false)
const modelError = ref(false)
const trendError = ref(false)
const recentError = ref(false)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)

const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatDateLocalInput(new Date()))
const granularity = ref<'day' | 'hour'>('day')
let statsRequest = 0
let chartRequest = 0
let recentRequest = 0

const loadStats = async () => {
  const request = ++statsRequest
  loading.value = true
  statsError.value = false
  try {
    const [, result] = await Promise.all([authStore.refreshUser(), usageAPI.getDashboardStats()])
    if (request === statsRequest) stats.value = result
  } catch {
    if (request === statsRequest) statsError.value = true
  } finally {
    if (request === statsRequest) loading.value = false
  }
}

const loadCharts = async () => {
  const request = ++chartRequest
  loadingCharts.value = true
  modelError.value = false
  trendError.value = false
  trendData.value = []
  modelStats.value = []
  const range = { start_date: startDate.value, end_date: endDate.value }
  const [trend, models] = await Promise.allSettled([
    usageAPI.getDashboardTrend({ ...range, granularity: granularity.value }),
    usageAPI.getDashboardModels(range),
  ])
  if (request !== chartRequest) return
  if (trend.status === 'fulfilled') trendData.value = trend.value.trend || []
  else trendError.value = true
  if (models.status === 'fulfilled') modelStats.value = models.value.models || []
  else modelError.value = true
  loadingCharts.value = false
}

const loadRecent = async () => {
  const request = ++recentRequest
  loadingUsage.value = true
  recentError.value = false
  try {
    // This section is always labelled “last 7 days”, independently of the chart range.
    const end = new Date()
    const start = new Date(end)
    start.setDate(start.getDate() - 6)
    const result = await usageAPI.getByDateRange(formatDateLocalInput(start), formatDateLocalInput(end))
    if (request === recentRequest) recentUsage.value = result.items.slice(0, 5)
  } catch {
    if (request === recentRequest) recentError.value = true
  } finally {
    if (request === recentRequest) loadingUsage.value = false
  }
}
const loadPlatformQuotas = async () => { try { const data = await getMyPlatformQuotas(); platformQuotas.value = data.platform_quotas ?? [] } catch (error) { console.warn('Failed to load platform quotas:', error); platformQuotas.value = [] } }
const refreshAll = () => { loadStats(); loadCharts(); loadRecent(); loadPlatformQuotas() }

onMounted(() => { refreshAll() })
</script>

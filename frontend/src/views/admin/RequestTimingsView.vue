<template>
  <AppLayout>
    <div class="space-y-5">
      <header><h1 class="text-2xl font-semibold">请求计时</h1><p class="mt-2 text-sm text-gray-500">Chat / Responses 服务端阶段，保留 {{ retentionDays }} 天。缺失值显示“未观测”；flush 表示服务端写出动作。</p></header>
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="load(false)">
        <label class="text-sm">请求 ID<input v-model="requestID" class="input mt-1 block" placeholder="网关或 usage request ID" /></label>
        <label class="text-sm">账号 ID<input v-model="accountID" type="number" min="1" class="input mt-1 block w-32" /></label>
        <label class="text-sm">终态<select v-model="outcome" class="input mt-1 block"><option value="">全部</option><option value="success">成功</option><option value="http_error">HTTP 失败</option><option value="upstream_error">上游/流失败</option><option value="client_canceled">客户端取消</option></select></label>
        <button class="btn btn-primary" type="submit" :disabled="loading">{{ loading ? '查询中…' : '查询' }}</button>
      </form>
      <p class="text-xs text-gray-500">本进程队列 {{ stats.queued }} · 丢弃 {{ stats.dropped }} · 写入失败 {{ stats.write_failed }}。记录缺失可能来自未覆盖路径、保留期或诊断丢样。</p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700"><table class="w-full text-left text-sm">
        <thead class="bg-gray-50 dark:bg-dark-800"><tr><th class="p-3">时间 / 请求</th><th class="p-3">终态</th><th class="p-3">应用总耗时</th><th class="p-3">首语义 / 首正文</th><th class="p-3">尝试 / 采集</th></tr></thead>
        <tbody><tr v-for="row in items" :key="row.trace_id" class="border-t border-gray-100 dark:border-dark-700"><td class="p-3"><button class="text-left text-blue-600 hover:underline" @click="showDetail(row.trace_id)">{{ new Date(row.started_at).toLocaleString() }}<span class="block max-w-xs truncate font-mono text-xs">{{ row.request_id }}</span></button></td><td class="p-3">{{ row.outcome }} / {{ row.status }}</td><td class="p-3">{{ ms(row.duration_ms) }}</td><td class="p-3">{{ ms(row.first_semantic_ms) }} / {{ ms(row.first_text_ms) }}</td><td class="p-3">{{ row.attempts.length }} / {{ row.completeness === 'complete' ? '网络边界完整' : '部分' }}</td></tr><tr v-if="!items.length && !loading"><td colspan="5" class="p-6 text-center text-gray-500">暂无匹配记录</td></tr></tbody>
      </table></div>
      <button v-if="items.length >= 50" class="btn btn-secondary" :disabled="loading" @click="load(true)">加载更早记录</button>
      <section v-if="detail" class="space-y-4 rounded-xl border border-gray-200 p-5 dark:border-dark-700" aria-label="请求计时详情">
        <div class="flex items-start justify-between gap-3"><div><h2 class="font-semibold">请求详情</h2><p class="break-all font-mono text-xs">{{ detail.trace_id }}</p><p class="mt-1 text-sm">{{ detail.path }} · 用户 {{ detail.user_id }} · Key {{ detail.api_key_id }} · Usage {{ detail.usage_id ?? '尚未关联' }}</p></div><button class="btn btn-secondary" @click="detail = null">关闭</button></div>
        <dl class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4"><div><dt class="text-gray-500">入站到终态</dt><dd>{{ ms(detail.duration_ms) }}</dd></div><div><dt class="text-gray-500">首语义</dt><dd>{{ ms(detail.first_semantic_ms) }}</dd></div><div><dt class="text-gray-500">首正文</dt><dd>{{ ms(detail.first_text_ms) }}</dd></div><div><dt class="text-gray-500">首次内容 flush</dt><dd>{{ ms(detail.first_content_flush_ms) }}</dd></div></dl>
        <dl v-if="metrics" class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4"><div><dt class="text-gray-500">准入等待（合并重叠区间）</dt><dd>{{ ms(metrics.wait) }}</dd></div><div><dt class="text-gray-500">去除准入等待后的总耗时</dt><dd>{{ ms(metrics.withoutWait) }}</dd></div><div><dt class="text-gray-500">最终尝试耗时</dt><dd>{{ ms(metrics.finalAttempt) }}</dd></div><div><dt class="text-gray-500">前序尝试耗时合计</dt><dd>{{ ms(metrics.previousAttempts) }}</dd></div></dl><p v-if="metrics && !detail.admission_complete" class="text-xs text-gray-500">准入覆盖尚不完整，已测得等待 {{ ms(metrics.measuredWait) }}；无法据此计算完整去等待耗时。</p><div><h3 class="text-sm font-semibold">实际准入等待</h3><p v-if="!detail.admission.length" class="text-sm text-gray-500">未观测到 wait span</p><p v-for="(span, index) in detail.admission" :key="index" class="text-sm">{{ span.kind }}：{{ ms(span.start_ms) }} → {{ ms(span.end_ms) }}（{{ ms(span.end_ms - span.start_ms) }}）</p></div>
        <article v-for="(attempt, index) in detail.attempts" :key="attempt.number" class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800">
          <h3 class="font-semibold">尝试 {{ attempt.number }} · 账号 {{ attempt.account_id }} · {{ attempt.endpoint }}</h3><p class="mt-1 text-xs text-gray-500">{{ ms(attempt.start_ms) }} → {{ ms(attempt.end_ms) }} · {{ attempt.outcome }} · HTTP {{ attempt.status || '未收到' }} · 连接{{ attempt.connection_reused === null ? '未观测' : attempt.connection_reused ? '复用' : '新建' }}{{ index < detail.attempts.length - 1 ? ' · 前序尝试，单独计时' : '' }}</p>
          <div class="mt-3 space-y-1"><div v-for="event in sortedEvents(attempt)" :key="event.name" class="grid grid-cols-[minmax(100px,1fr)_2fr_90px] items-center gap-2 text-xs"><span>{{ eventLabels[event.name] ?? event.name }}</span><div class="h-2 rounded bg-gray-200 dark:bg-dark-700"><div class="h-2 rounded bg-blue-500" :style="{ width: `${position(event.at, detail.duration_ms)}%` }" /></div><span class="text-right">{{ ms(event.at) }}</span></div></div>
        </article><p class="text-xs text-gray-500">各时间均从本请求入站计算。连接复用时 DNS / TLS 可以缺失；非流式不报告内容 flush。正文和工具参数均不保存。</p>
      </section>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { requestTimingMetrics } from './requestTimingMetrics'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { getRequestTiming, listRequestTimings } from '@/api/admin/requestTimings'
import type { RequestTiming, RequestTimingAttempt, RequestTimingStats } from '@/types/requestTimings'
const route = useRoute()
const requestID = ref(typeof route.query.request_id === 'string' ? route.query.request_id : '')
const accountID = ref('')
const outcome = ref('')
const items = ref<RequestTiming[]>([])
const detail = ref<RequestTiming | null>(null)
const stats = ref<RequestTimingStats>({ queued: 0, dropped: 0, write_failed: 0 })
const metrics = computed(() => detail.value ? requestTimingMetrics(detail.value) : null)
const retentionDays = ref(7)
const loading = ref(false)
const error = ref('')
const eventLabels: Record<string, string> = { get_connection: '申请连接', got_connection: '取得连接', dns_start: 'DNS 开始', dns_end: 'DNS 结束', connect_start: '连接开始', connect_end: '连接结束', tls_start: 'TLS 开始', tls_end: 'TLS 结束', wrote_headers: '请求头已发送', upload_complete: '上传完成', upload_failed: '上传失败', first_response_byte: '首响应字节', response_headers: '响应头完整', first_semantic: '首语义', first_text: '首正文', first_content_flush: '内容 flush' }
function ms(value: number | null | undefined) { return value == null ? '未观测' : `${value.toFixed(1)} ms` }
function sortedEvents(attempt: RequestTimingAttempt) { return Object.entries(attempt.events_ms).map(([name, at]) => ({ name, at })).sort((a, b) => a.at - b.at) }
function position(at: number, duration: number | null) { return duration && duration > 0 ? Math.min(100, Math.max(0, at / duration * 100)) : 0 }
async function load(older: boolean) {
  loading.value = true; error.value = ''
  try {
    const result = await listRequestTimings({ request_id: requestID.value || undefined, account_id: accountID.value ? Number(accountID.value) : undefined, outcome: outcome.value || undefined, limit: 50, before: older ? items.value[items.value.length - 1]?.started_at : undefined })
    items.value = older ? [...items.value, ...result.items] : result.items
    stats.value = result.stats; retentionDays.value = result.retention_days
  } catch { error.value = '请求计时加载失败，请重试。' } finally { loading.value = false }
}
async function showDetail(id: string) { error.value = ''; try { detail.value = await getRequestTiming(id) } catch { error.value = '详情加载失败，记录可能已过保留期。' } }
onMounted(async () => { await load(false); if (typeof route.query.trace_id === 'string') await showDetail(route.query.trace_id) })
</script>

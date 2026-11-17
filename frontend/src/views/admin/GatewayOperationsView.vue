<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6 p-4 md:p-6">
      <h1 class="text-2xl font-bold">网关运维观察</h1>
      <p class="text-sm text-gray-500">手动查看供应商钱包、固定账号实验与缓存账单。所有实验均保留失败与未知结果，产物由人工判断。</p>
      <p v-if="error" role="alert" class="rounded bg-red-50 p-3 text-red-700">{{ error }}</p>
      <section class="space-y-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
        <h2 class="text-lg font-semibold">执行设置</h2>
        <div class="flex flex-wrap items-end gap-4"><label><input v-model="config.worker_enabled" type="checkbox" /> 后台钱包只读刷新（默认关闭）</label><label>刷新间隔（分钟）<input v-model.number="config.refresh_minutes" type="number" min="5" max="1440" class="input w-32" /></label><label>每日实验预算（USD，0 禁止执行）<input v-model.number="config.daily_budget_usd" type="number" min="0" max="10000" step="0.01" class="input w-40" /></label><button class="btn btn-primary" :disabled="busy" @click="saveConfig">保存执行设置</button></div>
        <p class="text-sm text-gray-500">预算按 UTC 日累计。执行中和结果未知的记录继续占用整笔预留；完成后按本站当前模型价格估算成本。这里不产生客户账单。</p>
      </section>
      <section class="space-y-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
        <div class="flex items-center justify-between"><h2 class="text-lg font-semibold">供应商共享钱包</h2><button class="btn btn-secondary" @click="resetWallet">新增钱包</button></div>
        <p class="text-sm text-gray-500">只支持 Sub2API / New API 的只读访问 Token。远端用户 ID 与地址共同标识钱包；多个本站账号关联同一个钱包。刷新失败保留上次余额并标记过期。低余额通知最多每 24 小时一次。</p>
        <form class="grid gap-3 md:grid-cols-3" @submit.prevent="saveWallet">
          <label>名称<input v-model="walletForm.name" required maxlength="100" class="input" /></label><label>供应商<select v-model="walletForm.provider" class="input"><option value="sub2api">Sub2API</option><option value="newapi">New API</option></select></label><label>HTTPS 地址<input v-model="walletForm.base_url" required type="url" class="input" /></label>
          <label>远端用户 ID<input v-model="walletForm.remote_user_id" required class="input" /></label><label>只读 Token（修改时留空保留）<input v-model="walletForm.credential_value" type="password" autocomplete="new-password" :required="!walletEditID" class="input" /></label><label>关联本站账号 ID（逗号分隔）<input v-model="accountIDs" class="input" /></label>
          <label>余额币种<select v-model="walletForm.currency" class="input"><option>USD</option><option>CNY</option><option>quota_units</option></select></label><label>低余额阈值（空为关闭）<input v-model="threshold" type="number" min="0" step="any" class="input" /></label><label>通知邮箱<input v-model="walletForm.notify_email" type="email" class="input" /></label>
          <button class="btn btn-primary" :disabled="busy">{{ walletEditID ? '保存钱包' : '创建钱包' }}</button>
        </form>
        <div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th>钱包 / 账号</th><th>余额</th><th>新鲜度与错误</th><th>操作</th></tr></thead><tbody><tr v-for="wallet in wallets" :key="wallet.id" class="border-t"><td class="p-2">{{ wallet.name }} · {{ wallet.provider }}<br />{{ wallet.base_url }}<br />账号 {{ wallet.account_ids?.join(', ') || '未关联' }}</td><td>{{ wallet.balance ?? '未知' }} {{ wallet.currency }}<br />阈值 {{ wallet.threshold ?? '关闭' }}</td><td>{{ wallet.stale ? '过期 / 未刷新' : '已刷新' }} · {{ date(wallet.refreshed_at) }}<br /><span class="text-red-600">{{ wallet.last_error }}</span></td><td><div class="flex gap-2"><button class="btn btn-secondary" :disabled="busy" @click="refreshWallet(wallet.id)">刷新</button><button class="btn btn-secondary" @click="editWallet(wallet)">编辑</button><button class="btn btn-secondary" :disabled="busy" @click="deleteWallet(wallet.id)">删除</button></div></td></tr><tr v-if="!wallets.length"><td colspan="4" class="p-6 text-center text-gray-500">暂无钱包，可在上方添加共享钱包。</td></tr></tbody></table></div>
        <div class="flex gap-2"><button class="btn btn-secondary" :disabled="walletOffset === 0 || busy" @click="pageWallets(-50)">上一页</button><span>{{ walletOffset + 1 }} 起</span><button class="btn btn-secondary" :disabled="wallets.length < 50 || busy" @click="pageWallets(50)">下一页</button></div>
      </section>
      <section class="space-y-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
        <h2 class="text-lg font-semibold">手动固定账号实验 / 鹈鹕</h2>
        <p class="text-sm text-gray-500">当前支持直接配置的 OpenAI 兼容 APIKey 账号。协议对应实际 /v1/chat/completions 或 /v1/responses，单提交执行一次，不自动换号或重试。输入与模型映射冻结在记录中。</p>
        <p class="text-sm text-gray-500">上游流式接收，完成后保存产物。每条记录展示实际接收方式；这是固定账号与参数下的生成观察，耗时包含网络、排队、推理和生成。</p>
        <form class="grid gap-3 md:grid-cols-3" @submit.prevent="runExperiment">
          <label>题目<select v-model="experiment.kind" class="input"><option value="pelican">鹈鹕骑自行车 · v1</option><option value="experiment">自定义受控实验</option></select></label><label>固定账号 ID<input v-model.number="experiment.account_id" required type="number" min="1" class="input" /></label><label>模型<input v-model="experiment.model" required class="input" /></label>
          <label>真实协议<select v-model="experiment.protocol" class="input"><option value="chat">Chat</option><option value="responses">Responses</option></select></label><label>Effort<select v-model="experiment.effort" class="input"><option value="">未指定</option><option v-for="effort in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']" :key="effort">{{ effort }}</option></select></label><label>最大输出 Token<input v-model.number="experiment.max_output_tokens" type="number" min="64" max="16384" required class="input" /></label>
          <label>此次预留预算（USD）<input v-model.number="experiment.budget_usd" type="number" min="0.000001" max="100" step="any" required class="input" /></label><label v-if="experiment.kind === 'experiment'">题目版本<input v-model="experiment.prompt_version" required class="input" /></label><label v-if="experiment.kind === 'experiment'" class="md:col-span-3">题目<textarea v-model="experiment.prompt" rows="5" required maxlength="65536" class="input" /></label>
          <label class="md:col-span-3"><input v-model="experimentConfirmed" type="checkbox" /> 我确认以当前账号执行真实付费提交，并预留上述预算</label><button class="btn btn-primary" :disabled="busy || !experimentConfirmed">{{ busy ? '处理中…' : '单次提交并保存观察记录' }}</button>
        </form>
        <div v-for="run in experiments" :key="run.id" class="space-y-2 rounded border p-3"><div class="flex flex-wrap justify-between gap-2"><strong>{{ run.frozen.kind }} · {{ run.state }} · {{ date(run.created_at) }}</strong><span>账号 #{{ run.frozen.account_id }} · {{ run.upstream_model }} · {{ run.frozen.effort || '未指定 effort' }}</span></div><div class="break-all text-sm">{{ run.id }} · {{ run.endpoint }} · {{ run.transport === 'sse' ? '流式接收' : 'JSON 接收' }} · {{ run.duration_ms }} ms · HTTP {{ run.http_status }} · 成本 {{ run.cost_usd == null ? '未知' : `$${run.cost_usd}` }} · 预留 ${{ run.reserved_usd }} · 题目 {{ run.frozen.prompt_version }}</div><p v-if="run.error" class="text-red-600">{{ run.error }}</p><details><summary class="cursor-pointer">冻结输入与用量</summary><pre class="max-h-64 overflow-auto whitespace-pre-wrap text-xs">{{ JSON.stringify({ frozen: run.frozen, transport: run.transport || 'json', usage: run.usage, terminal: run.terminal, raw_response: run.raw_response }, null, 2) }}</pre></details><ObservationPreview v-if="run.output" :source="run.output" :id="run.id" /></div>
        <p v-if="!experiments.length" class="rounded border p-6 text-center text-sm text-gray-500">暂无手动实验。</p>
        <div class="flex gap-2"><button class="btn btn-secondary" :disabled="experimentOffset === 0 || busy" @click="pageExperiments(-50)">上一页</button><button class="btn btn-secondary" :disabled="busy" @click="loadExperiments">刷新记录</button><button class="btn btn-secondary" :disabled="experiments.length < 50 || busy" @click="pageExperiments(50)">下一页</button></div>
      </section>
      <CacheDiagnostic />
      <MagpieImportPanel />
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import ObservationPreview from '@/components/admin/gateway-operations/ObservationPreview.vue'
import CacheDiagnostic from '@/components/admin/gateway-operations/CacheDiagnostic.vue'
import MagpieImportPanel from '@/components/admin/gateway-operations/MagpieImportPanel.vue'
import { gatewayOperationsAPI as api } from '@/api/admin/gatewayOperations'
import { operationsUUID } from '@/utils/operationsUUID'
import type { OperationsConfig, OperationsWallet, OperationsWalletInput, OperationsExperiment, OperationsExperimentInput } from '@/types/gatewayOperations'
const error = ref(''), busy = ref(false), walletOffset = ref(0), experimentOffset = ref(0), walletEditID = ref(0), accountIDs = ref(''), threshold = ref<number | string>(''), experimentConfirmed = ref(false)
const config = reactive<OperationsConfig>({ worker_enabled: false, refresh_minutes: 30, daily_budget_usd: 0 })
const wallets = ref<OperationsWallet[]>([]), experiments = ref<OperationsExperiment[]>([])
const emptyWallet = (): OperationsWalletInput => ({ name: '', provider: 'sub2api', base_url: '', remote_user_id: '', account_ids: [], credential_value: '', currency: 'USD', threshold: null, notify_email: '' })
const walletForm = reactive(emptyWallet())
const experiment = reactive<OperationsExperimentInput>({ submit_id: operationsUUID(), kind: 'pelican', account_id: 0, model: '', effort: '', protocol: 'chat', prompt: '', prompt_version: 'custom-v1', max_output_tokens: 4096, budget_usd: 1 })
function date(value: string | null) { return value ? new Date(value).toLocaleString() : '未刷新' }
async function act(fn: () => Promise<void>) { busy.value = true; error.value = ''; try { await fn() } catch (e) { error.value = e instanceof Error ? e.message : String(e) } finally { busy.value = false } }
async function loadWallets() { wallets.value = await api.wallets(walletOffset.value) }
async function loadExperiments() { await act(async () => { experiments.value = await api.experiments(experimentOffset.value) }) }
function resetWallet() { walletEditID.value = 0; accountIDs.value = ''; threshold.value = ''; Object.assign(walletForm, emptyWallet()) }
function editWallet(wallet: OperationsWallet) { walletEditID.value = wallet.id; Object.assign(walletForm, wallet, { credential_value: '' }); accountIDs.value = wallet.account_ids?.join(', ') || ''; threshold.value = wallet.threshold ?? '' }
async function saveWallet() { await act(async () => { const ids = accountIDs.value.trim() ? accountIDs.value.split(',').map(id => Number(id.trim())) : []; if (ids.some(id => !Number.isInteger(id) || id <= 0)) throw new Error('账号 ID 必须为正整数'); await api.saveWallet(walletEditID.value, { ...walletForm, account_ids: ids, threshold: threshold.value === '' ? null : Number(threshold.value) }); resetWallet(); await loadWallets() }) }
async function deleteWallet(id: number) { if (!window.confirm('删除这个钱包及其观察记录？')) return; await act(async () => { await api.deleteWallet(id); await loadWallets() }) }
async function refreshWallet(id: number) { await act(async () => { try { await api.refreshWallet(id) } finally { await loadWallets() } }) }
async function saveConfig() { await act(async () => { Object.assign(config, await api.saveConfig({ ...config })) }) }
async function runExperiment() { if (busy.value || !experimentConfirmed.value) return; await act(async () => { experimentConfirmed.value = false; try { await api.runExperiment({ ...experiment }); experiment.submit_id = operationsUUID() } finally { experimentOffset.value = 0; experiments.value = await api.experiments() } }) }
async function pageWallets(delta: number) { await act(async () => { walletOffset.value += delta; await loadWallets() }) }
async function pageExperiments(delta: number) { experimentOffset.value += delta; await loadExperiments() }
onMounted(() => act(async () => { Object.assign(config, await api.config()); await loadWallets(); experiments.value = await api.experiments() }))
</script>

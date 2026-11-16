<template>
  <section class="space-y-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
    <h2 class="text-lg font-semibold">分组缓存诊断</h2>
    <p class="text-sm text-gray-500">使用当前用户自己的 Key，经本站正常网关和正常计费。两次请求使用完全相同的长前缀；记录客户端响应中的唯一请求标识，再精确查询 usage。零缓存值仍可能是上游未报告，不能自动判断未命中。</p>
    <div class="grid gap-3 md:grid-cols-3">
      <label>自己的 Key<select v-model.number="keyID" class="input"><option :value="0">选择 Key</option><option v-for="key in keys" :key="key.id" :value="key.id">{{ key.name }} · #{{ key.id }} · 组 {{ key.group_id }}</option></select></label>
      <label>模型<input v-model="model" class="input" /></label>
      <label>协议<select v-model="protocol" class="input"><option value="chat">Chat</option><option value="responses">Responses</option></select></label>
    </div>
    <label class="block">相同前缀（建议足够长以达到供应商缓存门槛）<textarea v-model="prefix" rows="5" class="input font-mono" /></label>
    <label class="block"><input v-model="confirmed" type="checkbox" /> 我确认执行两次真实请求并按此 Key 的正常规则计费</label>
    <div class="flex flex-wrap gap-2"><button class="btn btn-primary" :disabled="busy || !confirmed || !keyID || !model || prefix.length < 100" @click="run">{{ busy ? '执行中，每个请求只发送一次…' : '发送两次相同前缀请求' }}</button><button class="btn btn-secondary" :disabled="!records.length || busy" @click="correlate">刷新 usage 关联</button></div>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <div v-for="record in records" :key="record.clientID" class="rounded border p-3 text-sm">
      <div>本地提交 {{ record.clientID }} · HTTP {{ record.status || '未知' }} · {{ record.durationMS }} ms</div>
      <div class="break-all">账单关联 {{ record.usageID || '响应缺少关联标识；不会猜测匹配' }}</div>
      <div v-if="record.error" class="text-red-600">{{ record.error }}</div>
      <template v-if="record.usage.length"><div v-for="usage in record.usage" :key="usage.id">Usage #{{ usage.id }} · 账号 #{{ usage.account_id }} · 组 {{ usage.group_id }} · 输入 {{ usage.input_tokens }} / 输出 {{ usage.output_tokens }} / 缓存读 {{ usage.cache_read_tokens }} / 缓存写 {{ usage.cache_creation_tokens }} · 实扣 ${{ usage.actual_cost }} · {{ usage.cache_fields_known ? '报告了缓存用量' : '缓存字段未知或零' }}</div></template>
      <div v-else>尚无唯一 usage 关联；可待异步写入后刷新。失败请求可能没有 usage。</div>
    </div>
  </section>
</template>
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { keysAPI } from '@/api/keys'
import { buildGatewayUrl } from '@/api/url'
import { gatewayOperationsAPI } from '@/api/admin/gatewayOperations'
import { operationsUUID } from '@/utils/operationsUUID'
import type { ApiKey } from '@/types'
import type { OperationsCacheUsage } from '@/types/gatewayOperations'
const keys = ref<ApiKey[]>([]), keyID = ref(0), model = ref(''), protocol = ref<'chat' | 'responses'>('chat'), prefix = ref(''), confirmed = ref(false), busy = ref(false), error = ref('')
interface Record { clientID: string; usageID: string; status: number; durationMS: number; error: string; usage: OperationsCacheUsage[] }
const records = ref<Record[]>([])
const submittedKeyID = ref(0)
onMounted(async () => { try { for (let page = 1; page <= 100; page++) { const result = await keysAPI.list(page, 100); keys.value.push(...result.items); if (keys.value.length >= result.total) break } } catch (e) { error.value = String(e) } })
async function correlate() { const ids = records.value.map(record => record.usageID).filter(Boolean); if (!ids.length) return; try { const usages = await gatewayOperationsAPI.cacheUsage(submittedKeyID.value, ids); for (const record of records.value) record.usage = usages.filter(usage => usage.request_id === record.usageID) } catch (e) { error.value = String(e) } }
async function run() {
 if (busy.value || !confirmed.value) return
  const key = keys.value.find(key => key.id === keyID.value); if (!key) return
  busy.value = true; error.value = ''; records.value = []; submittedKeyID.value = key.id; confirmed.value = false
  const chosenModel = model.value, chosenProtocol = protocol.value, chosenPrefix = prefix.value
  try {
    const credential = (await keysAPI.getById(key.id)).key
    const endpoint = chosenProtocol === 'chat' ? '/v1/chat/completions' : '/v1/responses'
    const target = new URL(buildGatewayUrl(endpoint), window.location.origin)
    // A key is only sent to this page's own gateway origin; wallet endpoints never enter this path.
    if (target.origin !== window.location.origin) throw new Error('缓存诊断需要与本站同源的网关')
    for (let index = 0; index < 2; index++) {
      const record: Record = { clientID: operationsUUID(), usageID: '', status: 0, durationMS: 0, error: '', usage: [] }; records.value.push(record)
      const started = performance.now(); const controller = new AbortController(); const timer = window.setTimeout(() => controller.abort(), 180000)
      try {
        const prompt = `${chosenPrefix}\n\nReply with just OK.`
        const body = chosenProtocol === 'chat' ? { model: chosenModel, messages: [{ role: 'user', content: prompt }], max_completion_tokens: 64, stream: false } : { model: chosenModel, input: prompt, max_output_tokens: 64, stream: false }
        const response = await fetch(target, { method: 'POST', headers: { Authorization: `Bearer ${credential}`, 'Content-Type': 'application/json', 'X-Request-ID': record.clientID }, body: JSON.stringify(body), signal: controller.signal, credentials: 'omit' })
        record.status = response.status
        const clientID = response.headers.get('X-Client-Request-ID'), localID = response.headers.get('X-Request-ID')
        record.usageID = clientID ? `client:${clientID}` : localID ? `local:${localID}` : ''
        await response.text()
        if (!response.ok) record.error = `HTTP ${response.status}；保留失败，不补发`
      } catch { record.error = '请求结果未知；不补发'; break } finally { clearTimeout(timer); record.durationMS = Math.round(performance.now() - started); records.value = [...records.value] }
    }
    await correlate()
  } catch (e) { error.value = String(e) } finally { busy.value = false }
}
</script>

<template>
  <section class="space-y-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
    <h2 class="text-lg font-semibold">导入 Magpie</h2>
    <p class="text-sm text-gray-500">选择当前用户自己的 Key，确认协议地址后再导入。Key 将通过已注册的 magpie://import 交给本机客户端；不自动修改现有客户端配置。</p>
    <div class="flex flex-wrap items-end gap-3"><label>自己的 Key<select v-model.number="keyID" class="input"><option :value="0">选择 Key</option><option v-for="key in keys" :key="key.id" :value="key.id">{{ key.name }} · #{{ key.id }}</option></select></label><button class="btn btn-secondary" :disabled="!keyID" @click="prepare">查看导入配置</button></div>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <div v-if="link" class="space-y-3"><pre class="overflow-auto rounded bg-gray-50 p-3 text-sm dark:bg-gray-900">{{ publicConfig }}</pre><label class="block"><input v-model="confirmed" type="checkbox" /> 我确认把所选 Key 交给本机 Magpie</label><button class="btn btn-primary" :disabled="!confirmed" @click="launch">打开 Magpie 导入</button></div>
  </section>
</template>
<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { keysAPI } from '@/api/keys'
import { getAvailable } from '@/api/groups'
import { buildGatewayUrl } from '@/api/url'
import { buildMagpieImportLink } from '@/utils/magpieImport'
import type { ApiKey } from '@/types'
const keys = ref<ApiKey[]>([]), keyID = ref(0), link = ref(''), publicConfig = ref(''), error = ref(''), confirmed = ref(false)
watch(keyID, () => { link.value = ''; confirmed.value = false; publicConfig.value = '' })
onMounted(async () => { try { for (let page = 1; page <= 100; page++) { const result = await keysAPI.list(page, 100); keys.value.push(...result.items); if (keys.value.length >= result.total) break } } catch (e) { error.value = String(e) } })
async function prepare() {
  error.value = ''; link.value = ''; confirmed.value = false
  try {
    const row = await keysAPI.getById(keyID.value)
    if (!row.group && row.group_id != null) row.group = (await getAvailable()).find(group => group.id === row.group_id)
    const result = buildMagpieImportLink({ dashboardOrigin: window.location.origin, baseUrl: buildGatewayUrl('/'), row })
    if ('error' in result) throw new Error({ unsupportedGroup: '此 Key 的分组不能明确映射客户端协议，请使用单一分组 Key', invalidBase: '网关地址不可导入', invalidKey: 'Key 无效' }[result.error])
    link.value = result.url
    const query = new URL(result.url).searchParams; query.set('key', '••••••（确认后传递所选 Key）'); publicConfig.value = [...query].map(([key, value]) => `${key}: ${value}`).join('\n')
  } catch (e) { error.value = String(e) }
}
function launch() { if (!confirmed.value || !link.value) return; window.location.assign(link.value); link.value = ''; confirmed.value = false }
</script>

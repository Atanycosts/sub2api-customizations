<template>
  <div class="cx-auth cx-scope">
    <div class="cx-auth-grid" aria-hidden="true"></div>
    <span class="cx-auth-edge" aria-hidden="true">AUTHENTICATION / AXIAL 02</span>

    <div class="cx-auth-inner">
      <!-- Shared site identity, matching the homepage and application navigation. -->
      <header v-if="settingsLoaded" class="cx-auth-brand">
        <div class="cx-auth-markrow">
          <SiteLogo :src="siteLogo" :alt="siteName" class="cx-auth-logo" />
          <span class="cx-auth-name">{{ siteName }}</span>
        </div>
        <p class="cx-auth-sub">{{ siteSubtitle }}</p>
      </header>

      <div class="cx-auth-card">
        <slot />
      </div>

      <div class="cx-auth-extra">
        <slot name="footer" />
      </div>

      <p class="cx-auth-copy">&copy; {{ currentYear }} {{ siteName }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import SiteLogo from '@/components/common/SiteLogo.vue'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.cx-auth {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: clamp(24px, 5vw, 72px) clamp(18px, 4vw, 64px);
  background: var(--cx-bg);
  color: var(--cx-ink);
  font-family: var(--cx-sans);
  overflow: hidden;
  -webkit-font-smoothing: antialiased;
}

/* 与首页同一张 80px 网格，只是更淡；径向遮罩让四角消失 */
.cx-auth-grid {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(var(--cx-grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--cx-grid) 1px, transparent 1px);
  background-size: 80px 80px;
  -webkit-mask-image: radial-gradient(ellipse 70% 62% at 50% 46%, #000 10%, transparent 100%);
  mask-image: radial-gradient(ellipse 70% 62% at 50% 46%, #000 10%, transparent 100%);
}

.cx-auth-edge {
  position: absolute;
  right: 16px;
  bottom: 16%;
  writing-mode: vertical-rl;
  font: 10px var(--cx-mono);
  letter-spacing: 0.18em;
  color: var(--cx-dim);
  pointer-events: none;
}

.cx-auth-inner {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 396px;
}

/* 品牌 */
.cx-auth-brand {
  margin-bottom: 28px;
}
.cx-auth-markrow {
  display: flex;
  align-items: center;
  gap: 10px;
}
.cx-auth-logo {
  width: 32px;
  height: 32px;
}
.cx-auth-name {
  font: 500 13px var(--cx-mono);
  letter-spacing: 0.12em;
  color: var(--cx-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cx-auth-sub {
  margin: 12px 0 0;
  font: 10px/1.6 var(--cx-mono);
  letter-spacing: 0.12em;
  color: var(--cx-dim);
}

/* 卡片：发丝线 + 2px 圆角，没有阴影 */
.cx-auth-card {
  border: 1px solid var(--cx-line-2);
  border-radius: 2px;
  background: var(--cx-card);
  padding: clamp(22px, 3vw, 34px);
}

.cx-auth-extra {
  margin-top: 18px;
}

.cx-auth-copy {
  margin: 26px 0 0;
  font: 10px var(--cx-mono);
  letter-spacing: 0.1em;
  color: var(--cx-dim);
}

/* ---- 表单元素在 CORTEX 语境下的重绘 ---- */
.cx-auth-card :deep(.input) {
  border-radius: 2px;
  border-color: var(--cx-line-2);
  background: transparent;
  font-size: 14px;
  color: var(--cx-ink);
  padding-top: 10px;
  padding-bottom: 10px;
}
.cx-auth-card :deep(.input::placeholder) {
  color: var(--cx-dim);
}
.cx-auth-card :deep(.input:focus) {
  border-color: var(--cx-accent);
  box-shadow: none;
  outline: none;
}
.cx-auth-card :deep(.input.input-error),
.cx-auth-card :deep(.input.input-error:focus),
.cx-auth-card :deep(.input.border-red-500),
.cx-auth-card :deep(.input.border-red-500:focus) {
  border-color: theme('colors.red.500');
}
.cx-auth-card :deep(.input.border-green-500),
.cx-auth-card :deep(.input.border-green-500:focus) {
  border-color: theme('colors.green.500');
}
.cx-auth-card :deep(.input-label) {
  margin-bottom: 7px;
  font: 10px var(--cx-mono);
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--cx-muted);
}
.cx-auth-card :deep(h2) {
  font-family: var(--cx-serif);
  font-weight: 900;
  letter-spacing: -0.01em;
  color: var(--cx-ink);
  font-size: 26px;
}
.cx-auth-card :deep(a) {
  color: var(--cx-accent);
}
/* 认证页内的状态提示条保留原有语义色（琥珀/绿/红），但圆角收进 2px，
   否则会出现"发丝线卡片里塞一颗胶囊"的语言冲突 */
/* 注意 .btn 的圆角来自 @apply rounded-xl，元素上并没有 rounded-xl 类，
   所以必须同时匹配 .btn，否则 OAuth 按钮仍是胶囊形 */
.cx-auth-card :deep(.btn),
.cx-auth-card :deep(.rounded-xl),
.cx-auth-card :deep(.rounded-lg) {
  border-radius: 2px;
  box-shadow: none;
}

.cx-auth-card :deep(button[type='submit']) {
  border-radius: 2px;
  font-size: 13px;
}
</style>

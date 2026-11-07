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

      <div class="cx-auth-card public-card">
        <slot />
      </div>

      <div class="cx-auth-extra">
        <slot name="footer" />
      </div>

      <PublicFooter class="cx-auth-copy" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import SiteLogo from '@/components/common/SiteLogo.vue'
import PublicFooter from '@/components/common/PublicFooter.vue'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.siteSubtitle)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

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

.cx-auth-card {
  padding: clamp(22px, 3vw, 34px);
}

.cx-auth-extra {
  margin-top: 18px;
}

.cx-auth-copy {
  margin: 26px 0 0;
  padding: 14px 0 0;
}

/* ---- 表单元素在 CORTEX 语境下的重绘 ---- */
.cx-auth-card :deep(.input) {
  font-size: 14px;
  padding-top: 10px;
  padding-bottom: 10px;
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
.cx-auth-card :deep(button[type='submit']) {
  font-size: 13px;
}
</style>

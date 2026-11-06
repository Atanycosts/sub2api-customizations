<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <SiteLogo
            :src="siteLogo"
            :alt="siteName"
            class="h-9 w-9"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <SiteLogo
          :src="siteLogo"
          :alt="siteName"
          class="mx-auto mb-6 h-20 w-20"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default Home Page — CORTEX identity -->
  <div v-else class="cortex" :class="{ 'cx-dark': isDark }">
    <header class="cx-header">
      <nav class="cx-nav">
        <div class="cx-brand">
          <SiteLogo :src="siteLogo" :alt="siteName" class="cx-brand-logo" />
          <span class="cx-brand-name">{{ siteName }}</span>
        </div>

        <div class="cx-nav-right">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="cx-icon-btn"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="cx-icon-btn"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
          </router-link>
          <button
            class="cx-icon-btn"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="cx-nav-cta">
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
            <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
      </nav>
    </header>

    <main>
      <!-- Hero: AI head as the central backdrop -->
      <section class="cx-hero">
        <div class="cx-grid"></div>
        <div class="cx-stage"><CortexHead :is-dark="isDark" /></div>
        <span class="cx-edge cx-edge-left">SUBSCRIPTION → API / AXIAL 01</span>
        <span class="cx-edge cx-edge-right">SIGNAL ROUTING / 0.2.19</span>

        <div class="cx-hero-inner">
          <div class="cx-hero-top">
            <p class="cx-eyebrow"><span class="cx-eyebrow-bar"></span>{{ t('home.cortex.eyebrowLeft') }}</p>
            <p class="cx-eyebrow cx-hide-sm">{{ t('home.cortex.eyebrowRight') }}</p>
          </div>

          <div class="cx-hero-bottom">
            <h1 class="cx-h1">
              <span class="cx-h1-line"><em class="cx-h1-term">{{ t('home.cortex.titleTermA') }}</em></span>
              <span class="cx-h1-line cx-h1-accent">{{ t('home.cortex.titleB') }}<em class="cx-h1-term cx-h1-term--hero">{{ t('home.cortex.titleTermB') }}{{ t('home.cortex.titleSuffix') }}</em><i class="cx-h1-caret"></i></span>
            </h1>
            <div class="cx-hero-row">
              <div class="cx-lead">
                <p class="cx-lead-main">{{ t('home.cortex.lead') }}</p>
                <p class="cx-lead-sub">{{ leadSubline }}</p>
              </div>
              <div class="cx-actions">
                <router-link
                  :to="isAuthenticated ? dashboardPath : '/login'"
                  class="cx-btn cx-btn-primary"
                >
                  {{ isAuthenticated ? t('home.goToDashboard') : t('home.cortex.ctaPrimary') }}
                  <Icon name="arrowRight" size="sm" />
                </router-link>
                <a
                  v-if="docUrl"
                  :href="docUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="cx-btn cx-btn-ghost"
                >
                  {{ t('home.viewDocs') }}
                </a>
                <a
                  v-else
                  :href="githubUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="cx-btn cx-btn-ghost"
                >
                  {{ t('home.cortex.ctaSecondary') }}
                </a>
              </div>
            </div>
          </div>
        </div>
      </section>

    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import SiteLogo from '@/components/common/SiteLogo.vue'
import Icon from '@/components/icons/Icon.vue'
import CortexHead from '@/components/home/CortexHead.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// GitHub URL
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

// 副标题优先用站点配置里的 site_subtitle，没配就回落到 CORTEX 自己的文案
const leadSubline = computed(() => {
  const configured = appStore.cachedPublicSettings?.site_subtitle || ''
  return configured && configured !== 'AI API Gateway Platform' ? configured : t('home.cortex.leadSub')
})

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
/* CORTEX / 皮层协议 — design tokens */
.cortex {
  /* 全部取自 src/styles/tokens.css —— 与全站同一组值，不再本地定义 */
  --cx-bg: rgb(var(--c-surface));
  --cx-panel: rgb(var(--c-surface-sunken));
  --cx-ink: rgb(var(--c-ink));
  --cx-muted: rgb(var(--c-ink-muted));
  --cx-dim: rgb(var(--c-dark-400));
  --cx-line: rgb(var(--c-ink) / 0.13);
  --cx-line-2: rgb(var(--c-ink) / 0.28);
  --cx-accent: rgb(var(--c-primary-600));
  --cx-grid: rgb(var(--c-ink) / 0.05);
  /* 例外：贴图上的仪器标注色与图像绑定，不随 UI 色板走 */
  --cx-head: #2a4654;
  --cx-img-filter: brightness(0.66) saturate(1.05) contrast(1.04);
  --cx-img-opacity: 0.94;
  --cx-lit-filter: brightness(0.26) saturate(1.12) contrast(1.18);
  --cx-term-hi: #054a40;
  --cx-sans: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif;
  --cx-serif: 'Songti SC', 'Noto Serif SC', 'Source Han Serif SC', 'STSong', 'SimSun', Georgia, serif;
  --cx-mono: ui-monospace, 'SF Mono', SFMono-Regular, Menlo, 'Cascadia Mono', Consolas, monospace;
  min-height: 100vh;
  min-height: 100svh;
  background: var(--cx-bg);
  color: var(--cx-ink);
  font-family: var(--cx-sans);
  font-feature-settings: 'kern' 1;
  text-rendering: optimizeLegibility;
  -webkit-font-smoothing: antialiased;
}
.cortex.cx-dark {
  --cx-bg: rgb(var(--c-surface));
  --cx-panel: rgb(var(--c-surface-sunken));
  --cx-ink: rgb(var(--c-ink));
  --cx-muted: rgb(var(--c-ink-muted));
  --cx-dim: rgb(var(--c-dark-500));
  --cx-line: rgb(var(--c-ink) / 0.12);
  --cx-line-2: rgb(var(--c-ink) / 0.28);
  --cx-accent: rgb(var(--c-primary-400));
  --cx-grid: rgb(var(--c-ink) / 0.045);
  --cx-head: #9bc2d4;
  --cx-img-filter: none;
  --cx-img-opacity: 0.92;
  --cx-lit-filter: brightness(2.7) saturate(1.3);
  --cx-term-hi: #d6fbee;
}

/* header */
.cx-header { position: relative; z-index: 5; border-bottom: 1px solid var(--cx-line); }
.cx-nav {
  max-width: 1440px; height: 68px; margin: 0 auto;
  padding: 0 clamp(18px, 4vw, 64px);
  display: flex; align-items: center; justify-content: space-between; gap: 20px;
}
.cx-brand { display: flex; align-items: center; gap: 10px; min-width: 0; }
.cx-brand-logo { width: 32px; height: 32px; }
.cx-brand-name { font: 500 14px var(--cx-mono); letter-spacing: 0.12em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cx-nav-right { display: flex; align-items: center; gap: 6px; flex: none; }
.cx-icon-btn {
  width: 36px; height: 36px; display: inline-flex; align-items: center; justify-content: center;
  border: 1px solid transparent; border-radius: 2px; color: var(--cx-muted);
  transition: color 0.2s, border-color 0.2s, background 0.2s;
}
.cx-icon-btn:hover { color: var(--cx-accent); border-color: var(--cx-line-2); background: var(--cx-panel); }
.cx-nav-cta {
  margin-left: 8px; min-height: 36px; padding: 0 16px;
  display: inline-flex; align-items: center; gap: 8px;
  border: 1px solid var(--cx-line-2); border-radius: 2px;
  font-size: 12px; font-weight: 500; color: var(--cx-ink);
  transition: color 0.2s, border-color 0.2s;
}
.cx-nav-cta:hover { color: var(--cx-accent); border-color: var(--cx-accent); }

/* hero */
.cx-hero {
  position: relative; display: flex; overflow: hidden;
  min-height: calc(100vh - 69px);
  min-height: calc(100svh - 69px);
}
.cx-grid {
  position: absolute; inset: 0; pointer-events: none;
  background-image: linear-gradient(var(--cx-grid) 1px, transparent 1px), linear-gradient(90deg, var(--cx-grid) 1px, transparent 1px);
  background-size: 80px 80px;
  -webkit-mask-image: radial-gradient(ellipse 72% 64% at 50% 42%, #000 8%, transparent 100%);
  mask-image: radial-gradient(ellipse 72% 64% at 50% 42%, #000 8%, transparent 100%);
}
.cx-stage {
  position: absolute; top: 43%; left: 50%; transform: translate(-50%, -50%);
  width: min(80vw, 730px); height: min(90vh, 850px);
  opacity: 1; pointer-events: none;
  -webkit-mask-image: radial-gradient(ellipse 58% 56% at 50% 46%, #000 40%, transparent 100%);
  mask-image: radial-gradient(ellipse 58% 56% at 50% 46%, #000 40%, transparent 100%);
}
.cx-edge {
  position: absolute; z-index: 2; writing-mode: vertical-rl;
  font: 10px var(--cx-mono); letter-spacing: 0.18em; color: var(--cx-dim);
}
.cx-edge-left { left: 16px; top: 30%; }
.cx-edge-right { right: 16px; bottom: 18%; }
.cx-hero-inner {
  position: relative; z-index: 2; flex: 1; width: 100%;
  max-width: 1440px; margin: 0 auto;
  padding: 28px clamp(18px, 4vw, 64px) 54px;
  display: flex; flex-direction: column; justify-content: space-between;
}
.cx-hero-top { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.cx-eyebrow {
  display: flex; align-items: center; gap: 10px; margin: 0;
  font: 10px var(--cx-mono); letter-spacing: 0.16em;
  text-transform: uppercase; color: var(--cx-muted);
}
.cx-eyebrow-bar { width: 28px; height: 1px; background: var(--cx-accent); flex: none; }
.cx-hero-bottom { position: relative; }
.cx-hero-bottom::before {
  content: ''; position: absolute; z-index: -1; pointer-events: none;
  left: -8vw; right: -8vw; bottom: -70px; height: calc(100% + 130px);
  background: linear-gradient(to top, var(--cx-bg) 0%, var(--cx-bg) 34%, transparent 96%);
  opacity: 0.96;
}
.cx-hero-bottom { position: relative; }
.cx-hero-bottom::before {
  content: ''; position: absolute; z-index: -1; pointer-events: none;
  left: -8vw; right: -8vw; bottom: -70px; height: calc(100% + 130px);
  background: linear-gradient(to top, var(--cx-bg) 0%, var(--cx-bg) 34%, transparent 96%);
  opacity: 0.96;
}
.cx-kicker {
  display: flex; align-items: center; gap: 10px;
  margin: 0; font: 400 10px/1 var(--cx-mono); letter-spacing: 0.18em;
  text-transform: uppercase; color: var(--cx-accent);
}
.cx-kicker::before {
  content: ''; flex: none; width: 18px; height: 1px; background: currentColor;
}
.cx-h1 {
  margin: 0; display: flex; flex-direction: column;
  font-family: var(--cx-serif);
  font-weight: 900;
  font-size: clamp(3.5rem, 9.6vw, 8.6rem);
  line-height: 0.94; letter-spacing: -0.022em;
}
.cx-h1-line { display: block; }
/* Latin 术语单独成套：宋体自带的拉丁是 Times 骨架，字重撑不起宋体 Black，
   所以让缩写走这套设计里的第二声部——等宽，并压到与中文字面齐高 */
.cx-h1-term {
  font-family: var(--cx-mono);
  font-style: normal;
  font-weight: 600;
  font-size: 0.86em;
  letter-spacing: -0.02em;
  margin-left: 0.26em;
  vertical-align: 0.02em;
}
/* 只有第一行行首不需要留白。注意不能用 :first-child——它只算元素节点，
   第二行的 "你的" 是文本节点，会被误判成首个子元素，把中英间距清零 */
.cx-h1-line:first-child > .cx-h1-term { margin-left: 0; }

/* 主语放大：SGI 是这句话真正的主语，"你的"只是定语 */
.cx-h1-term--hero {
  font-size: 1.28em;
  letter-spacing: -0.03em;
  vertical-align: -0.04em;
}

/* 一道光从左到右掠过字母本身——和头部的扫描是同一个母题 */
@supports (background-clip: text) or (-webkit-background-clip: text) {
  .cx-h1-term--hero {
    background-image: linear-gradient(
      100deg,
      var(--cx-accent) 0%,
      var(--cx-accent) 41%,
      var(--cx-term-hi) 50%,
      var(--cx-accent) 59%,
      var(--cx-accent) 100%
    );
    background-size: 260% 100%;
    background-repeat: no-repeat;
    background-position: 100% 0;
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
    -webkit-text-fill-color: transparent;
  }
}
@media (prefers-reduced-motion: no-preference) {
  .cx-h1-term--hero { animation: cx-term-sweep 9s ease-in-out infinite; }
}
/* background-size 260% 时，position 只有落在 0%–100% 才保证文字被完整覆盖；
   超出这个区间会让右（或左）半段完全没有背景，字直接消失 */
@keyframes cx-term-sweep {
  0%   { background-position: 100% 0; }
  30%  { background-position: 0% 0; }
  100% { background-position: 0% 0; }
}
.cx-h1-accent { color: var(--cx-accent); display: inline-flex; align-items: flex-end; }
.cx-h1-caret {
  display: inline-block; width: clamp(9px, 1.1vw, 15px); height: clamp(9px, 1.1vw, 15px);
  margin: 0 0 clamp(12px, 1.6vw, 22px) clamp(14px, 1.8vw, 26px);
  border-right: 2px solid var(--cx-accent); border-bottom: 2px solid var(--cx-accent);
  opacity: 0.6;
}
.cx-hero-row {
  display: flex; align-items: flex-end; justify-content: space-between;
  gap: 32px; margin-top: 36px;
}
.cx-lead { max-width: 440px; }
.cx-lead-main {
  margin: 0; font-size: 16px; line-height: 1.65; letter-spacing: 0.005em; color: var(--cx-ink);
}
.cx-lead-sub {
  margin: 9px 0 0; font-size: 13px; line-height: 1.7; letter-spacing: 0.01em; color: var(--cx-muted);
}
.cx-actions { display: flex; gap: 10px; flex-wrap: wrap; }
.cx-btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 10px;
  min-height: 46px; padding: 0 24px; border-radius: 2px;
  font-size: 13px; font-weight: 500; white-space: nowrap;
  transition: transform 0.2s, color 0.2s, border-color 0.2s, filter 0.2s;
}
.cx-btn:hover { transform: translateY(-2px); }
.cx-btn-primary { background: var(--cx-accent); border: 1px solid var(--cx-accent); color: var(--cx-bg); }
.cx-btn-primary:hover { filter: brightness(1.12); }
.cx-btn-ghost { border: 1px solid var(--cx-line-2); color: var(--cx-ink); }
.cx-btn-ghost:hover { border-color: var(--cx-accent); color: var(--cx-accent); }

/* ---- motion ---- */
@media (prefers-reduced-motion: no-preference) {
  /* 首屏入场 */
  .cx-eyebrow,
  .cx-h1-line,
  .cx-lead-main,
  .cx-lead-sub,
  .cx-actions {
    animation: cx-rise 0.9s cubic-bezier(0.22, 0.61, 0.36, 1) both;
  }
  .cx-eyebrow { animation-delay: 0.04s; }
  .cx-h1-line:nth-child(1) { animation-delay: 0.10s; }
  .cx-h1-line:nth-child(2) { animation-delay: 0.20s; }
  .cx-lead-main { animation-delay: 0.4s; }
  .cx-lead-sub { animation-delay: 0.46s; }
  .cx-actions { animation-delay: 0.44s; }
  .cx-stage { animation: cx-fade 1.2s ease 0.08s both; }
}

@keyframes cx-rise {
  from { opacity: 0; transform: translateY(20px); filter: blur(7px); }
  to { opacity: 1; transform: none; filter: blur(0); }
}
@keyframes cx-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

@media (max-width: 860px) {
  .cx-hide-sm, .cx-edge { display: none; }
  .cx-h1 { font-size: clamp(3rem, 12vw, 5rem); }
  .cx-hero-row { flex-direction: column; align-items: flex-start; gap: 24px; }
  .cx-stage { width: 130vw; height: 78vh; top: 42%; }
}

@media (max-width: 480px) {
  .cx-nav { height: auto; min-height: 68px; padding-top: 12px; padding-bottom: 12px; flex-wrap: wrap; gap: 12px; }
  .cx-brand { flex: none; }
  .cx-brand-name { display: none; }
  .cx-nav-right { max-width: 100%; margin-left: auto; flex-wrap: wrap; justify-content: flex-end; }
  .cx-nav-cta { white-space: nowrap; }
}
</style>

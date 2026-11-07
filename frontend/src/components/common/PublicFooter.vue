<template>
  <footer class="public-footer">
    <p>&copy; {{ currentYear }} {{ siteName }}</p>
    <nav v-if="documents.length" :aria-label="t('legal.loginAgreement')">
      <RouterLink v-for="doc in documents" :key="doc.id" :to="`/legal/${encodeURIComponent(doc.id)}`">
        {{ doc.title }}
      </RouterLink>
    </nav>
    <slot />
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const siteName = computed(() => appStore.siteName)
const currentYear = new Date().getFullYear()
const documents = computed(() =>
  (appStore.cachedPublicSettings?.login_agreement_documents ?? [])
    .filter(doc => doc.id.trim() && doc.title.trim() && doc.content_md.trim())
)
</script>

<style scoped>
.public-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 8px 20px;
  padding: 14px clamp(18px, 4vw, 64px);
  border-top: 1px solid rgb(var(--c-ink) / 0.13);
  color: rgb(var(--c-ink-muted));
  font: 11px/1.6 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  overflow-wrap: anywhere;
}
p { margin: 0; }
nav { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px 16px; }
a:hover { color: rgb(var(--c-ink)); text-decoration: underline; }
a:focus-visible { outline: 2px solid rgb(var(--c-primary-600)); outline-offset: 3px; }
</style>

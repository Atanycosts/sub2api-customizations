<template>
  <div
    class="min-h-screen bg-surface"
    :class="{ 'app-layout-viewport': viewport }"
  >
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="app-layout-content relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader />

      <!-- Main Content -->
      <main class="app-layout-main p-4 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

withDefaults(defineProps<{ viewport?: boolean }>(), { viewport: false })

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>

<style scoped>
.app-layout-viewport {
  height: 100dvh;
  min-height: 0;
}
.app-layout-viewport .app-layout-content {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.app-layout-viewport .app-layout-content > :deep(header) {
  flex-shrink: 0;
}
.app-layout-viewport .app-layout-main {
  display: flex;
  flex: 1;
  min-height: 0;
  padding: 16px 24px;
}
@media (max-width: 640px) {
  .app-layout-viewport .app-layout-main {
    padding: 12px;
  }
}
</style>

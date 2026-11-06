<template>
  <span class="site-logo" role="img" :aria-label="alt">
    <img v-if="src" :src="src" alt="" class="site-logo-image" />
    <span v-else class="site-logo-mark" aria-hidden="true" :style="maskStyle"></span>
  </span>
</template>

<script setup lang="ts">
import { DEFAULT_SITE_LOGO_URL } from '@/utils/branding'

// Callers supply the sanitized site_logo from their existing settings binding.
withDefaults(defineProps<{ src?: string; alt?: string }>(), {
  src: '',
  alt: 'Logo',
})

const maskStyle = {
  maskImage: `url("${DEFAULT_SITE_LOGO_URL}")`,
  WebkitMaskImage: `url("${DEFAULT_SITE_LOGO_URL}")`,
}
</script>

<style scoped>
.site-logo {
  display: inline-flex;
  flex-shrink: 0;
  vertical-align: middle;
}

.site-logo-image,
.site-logo-mark {
  display: block;
  width: 100%;
  height: 100%;
}

.site-logo-image {
  object-fit: contain;
}

.site-logo-mark {
  background-color: rgb(var(--c-ink));
  -webkit-mask-position: center;
  mask-position: center;
  -webkit-mask-size: contain;
  mask-size: contain;
  -webkit-mask-repeat: no-repeat;
  mask-repeat: no-repeat;
}

.dark .site-logo-mark {
  background-color: rgb(var(--c-primary-400));
}
</style>

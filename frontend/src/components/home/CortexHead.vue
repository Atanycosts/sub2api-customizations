<template>
  <div ref="root" class="cx-head" :class="{ 'is-probing': probing }" aria-hidden="true">
    <div ref="artwork" class="cx-head-art">
      <div class="cx-head-image">
        <img class="cx-head-img" :src="asset" alt="" />
        <!-- 扫描 = 照明：经过处那一片头才被照亮，而不是叠一条色块上去 -->
        <div class="cx-lit" :style="{ '--cx-mask': `url(${asset})` }">
          <div class="cx-lit-band" :style="{ '--cx-mask': `url(${asset})` }">
            <img class="cx-head-lit" :src="asset" alt="" />
          </div>
        </div>
        <div class="cx-scan" :style="{ '--cx-mask': `url(${asset})` }"></div>
      </div>
      <svg class="cx-head-svg" viewBox="0 0 720 900" preserveAspectRatio="xMidYMid meet" fill="none" role="presentation">
        <!-- orbit rings -->
        <circle cx="360" cy="352" r="300" stroke="currentColor" stroke-opacity="0.085" stroke-width="1" stroke-dasharray="2 9" />
        <circle cx="360" cy="352" r="258" stroke="currentColor" stroke-opacity="0.035" stroke-width="1" />
        <circle cx="360" cy="352" r="330" class="cx-orbit" stroke="currentColor" stroke-opacity="0.1" stroke-width="1" stroke-dasharray="1 24" />

        <!-- corner crosshairs -->
        <g stroke="currentColor" stroke-opacity="0.18" stroke-width="1">
          <path d="M34 52 h20 M44 42 v20" />
          <path d="M686 52 h-20 M676 42 v20" />
          <path d="M34 848 h20 M44 838 v20" />
          <path d="M686 848 h-20 M676 838 v20" />
        </g>

        <!-- frame labels -->
        <g class="cx-labels" fill="currentColor">
          <text class="cx-fig-tick" x="34" y="176">120</text>
          <text class="cx-fig-tick" x="34" y="324">240</text>
          <text class="cx-fig-tick" x="34" y="472">360</text>
          <text class="cx-fig-tick" x="34" y="620">480</text>
        </g>
        <g stroke="currentColor" stroke-opacity="0.11" stroke-width="1">
          <path d="M60 172 h14 M60 320 h14 M60 468 h14 M60 616 h14" />
          <path d="M646 172 h14 M646 320 h14 M646 468 h14 M646 616 h14" />
        </g>

        <!-- datum lines -->
        <g stroke="currentColor" stroke-opacity="0.07" stroke-width="0.8" stroke-dasharray="3 7">
          <path d="M96 268 H628" />
          <path d="M96 352 H628" />
          <path d="M96 442 H628" />
        </g>
        <g class="cx-labels" fill="currentColor">
          <text class="cx-fig-tick" x="634" y="271">A</text>
          <text class="cx-fig-tick" x="634" y="355">B</text>
          <text class="cx-fig-tick" x="634" y="445">C</text>
        </g>
      </svg>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import headScanDark from '@/assets/cortex/head-scan.webp'
import headScanLight from '@/assets/cortex/head-scan-light.webp'

// 两个主题用两份资产：暗色靠压 RGB 让脸沉进阴影，浅色靠压 alpha 让脸褪向纸面。
// 同一张图在两个底色上语义相反（压亮度在浅底上等于加重墨色），所以必须分开。
const props = withDefaults(defineProps<{ isDark?: boolean }>(), { isDark: true })
const asset = computed(() => (props.isDark ? headScanDark : headScanLight))

/* ---- 指针交互：头部视差 + 扫描带跟随 ---- */
const root = ref<HTMLElement | null>(null)
const artwork = ref<HTMLElement | null>(null)
const probing = ref(false)

let frame = 0
let nx = 0
let ny = 0
let reducedMotion = false

function schedule() {
  if (frame) return
  frame = requestAnimationFrame(() => {
    frame = 0
    const el = root.value
    if (!el) return
    const cx = Math.max(-1, Math.min(1, nx))
    const cy = Math.max(-1, Math.min(1, ny))
    el.style.setProperty('--hx', `${(-cx * 15).toFixed(2)}px`)
    el.style.setProperty('--hy', `${(-cy * 11).toFixed(2)}px`)
    // 扫描带的纵向位置跟着光标走，边界外留一点余量再淡出
    el.style.setProperty('--scan-y', `${((cy * 0.5 + 0.5) * 116 - 8).toFixed(1)}%`)
  })
}

function reset() {
  if (frame) {
    cancelAnimationFrame(frame)
    frame = 0
  }
  probing.value = false
  const el = root.value
  if (!el) return
  el.style.setProperty('--hx', '0px')
  el.style.setProperty('--hy', '0px')
}

function onPointerMove(e: PointerEvent) {
  const el = artwork.value
  if (!el || reducedMotion || e.pointerType !== 'mouse') return

  const r = el.getBoundingClientRect()
  const padX = r.width * 0.1
  const padY = r.height * 0.08
  const inside =
    e.clientX > r.left - padX &&
    e.clientX < r.right + padX &&
    e.clientY > r.top - padY &&
    e.clientY < r.bottom + padY

  if (!inside) {
    if (probing.value) reset()
    return
  }

  nx = (e.clientX - (r.left + r.width / 2)) / (r.width / 2)
  ny = (e.clientY - (r.top + r.height / 2)) / (r.height / 2)
  if (!probing.value) probing.value = true
  schedule()
}

onMounted(() => {
  reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (reducedMotion) return
  window.addEventListener('pointermove', onPointerMove, { passive: true })
  window.addEventListener('blur', reset)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('blur', reset)
  if (frame) cancelAnimationFrame(frame)
})
</script>

<style scoped>
.cx-head {
  position: relative;
  width: 100%;
  height: 100%;
  color: var(--cx-head);
}
.cx-head-art {
  position: absolute;
  top: 43%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: min(80vw, 730px);
  height: min(90vh, 850px);
}
.cx-head-image {
  position: absolute;
  inset: 0;
  -webkit-mask-image: radial-gradient(ellipse 58% 56% at 50% 46%, #000 40%, transparent 100%);
  mask-image: radial-gradient(ellipse 58% 56% at 50% 46%, #000 40%, transparent 100%);
}
.cx-head-img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  object-position: 50% 50%;
  filter: var(--cx-img-filter, none);
  opacity: var(--cx-img-opacity, 0.92);
  transform: translate3d(var(--hx, 0px), var(--hy, 0px), 0);
  transition: transform 0.45s cubic-bezier(0.22, 0.61, 0.36, 1);
  will-change: transform;
}

/* —— 扫描 = 照明 ——
   一层提亮副本，只在「光带」经过的那一片显形。外层 mask 用图片自身 alpha
   把头以外的区域裁掉，内层 mask 是移动的光带，两者嵌套即天然相交。 */
.cx-lit {
  position: absolute;
  inset: 0;
  pointer-events: none;
  transform: translate3d(var(--hx, 0px), var(--hy, 0px), 0);
  transition: transform 0.45s cubic-bezier(0.22, 0.61, 0.36, 1);
  -webkit-mask-image: var(--cx-mask);
  mask-image: var(--cx-mask);
  -webkit-mask-size: contain;
  mask-size: contain;
  -webkit-mask-position: center;
  mask-position: center;
  -webkit-mask-repeat: no-repeat;
  mask-repeat: no-repeat;
}
.cx-lit-band {
  position: absolute;
  inset: 0;
  -webkit-mask-image: linear-gradient(
    to bottom, transparent 0%, #000 40%, #000 60%, transparent 100%
  );
  mask-image: linear-gradient(
    to bottom, transparent 0%, #000 40%, #000 60%, transparent 100%
  );
  -webkit-mask-size: 100% 7%;
  mask-size: 100% 7%;
  -webkit-mask-position: 50% -30%;
  mask-position: 50% -30%;
  -webkit-mask-repeat: no-repeat;
  mask-repeat: no-repeat;
  animation: cx-lit-move 9s linear infinite;
  will-change: mask-position;
}
.cx-head-lit {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  object-position: 50% 50%;
  filter: var(--cx-lit-filter, none);
}

/* 光束的芯：一条极细的强调色，只负责指出方向 */
.cx-scan {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image: linear-gradient(
    to bottom,
    transparent 0%,
    transparent 42%,
    var(--cx-accent) 50%,
    transparent 58%,
    transparent 100%
  );
  background-repeat: no-repeat;
  background-size: 100% 2.2%;
  background-position: 50% -30%;
  -webkit-mask-image: var(--cx-mask);
  mask-image: var(--cx-mask);
  -webkit-mask-size: contain;
  mask-size: contain;
  -webkit-mask-position: center;
  mask-position: center;
  -webkit-mask-repeat: no-repeat;
  mask-repeat: no-repeat;
  opacity: 0.42;
  animation: cx-scan-move 9s linear infinite;
  will-change: background-position;
}

/* 指针在头部范围内时，光交给光标控制，像手动推动探头 */
.cx-head.is-probing .cx-lit-band {
  animation: none;
  -webkit-mask-position: 50% var(--scan-y, 50%);
  mask-position: 50% var(--scan-y, 50%);
  transition: mask-position 0.09s linear;
}
.cx-head.is-probing .cx-scan {
  animation: none;
  opacity: 0.5;
  background-position: 50% var(--scan-y, 50%);
  transition: background-position 0.09s linear, opacity 0.3s ease;
}

.cx-head-svg {
  position: absolute;
  top: 0;
  left: 50%;
  display: block;
  width: 100%;
  max-width: calc(100vw - 36px);
  height: 100%;
  transform: translate3d(calc(-50% + var(--hx, 0px) * 1.8), calc(var(--hy, 0px) * 1.8), 0);
  transition: transform 0.45s cubic-bezier(0.22, 0.61, 0.36, 1);
  will-change: transform;
}
.cx-labels {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  letter-spacing: 1.4px;
}
.cx-fig-tick { fill-opacity: 0.3; }
@media (max-width: 860px) {
  .cx-head-art { width: 130vw; height: 78vh; top: 42%; }
  .cx-labels { font-size: 18px; letter-spacing: 0.8px; }
}
.cx-orbit {
  transform-origin: 360px 352px;
  animation: cx-orbit 100s linear infinite;
}
@keyframes cx-scan-move {
  0% { background-position: 50% -30%; opacity: 0; }
  7% { opacity: 0.42; }
  93% { opacity: 0.42; }
  100% { background-position: 50% 130%; opacity: 0; }
}
@keyframes cx-lit-move {
  0% { mask-position: 50% -30%; opacity: 0; }
  8% { opacity: 1; }
  92% { opacity: 1; }
  100% { mask-position: 50% 130%; opacity: 0; }
}
@keyframes cx-orbit {
  to { transform: rotate(360deg); }
}
@media (prefers-reduced-motion: reduce) {
  .cx-scan, .cx-orbit, .cx-lit-band { animation: none; }
  .cx-scan { opacity: 0; }
  .cx-lit-band { opacity: 0; }
}
</style>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'

const props = defineProps({ kind: String, no: Number, tick: Number })
const k = computed(() => props.no + '-' + props.tick)

/* ---------------- M35 环形 ---------------- */
const R = 27
const C = 2 * Math.PI * R
const ringPct = 0.72

/* ---------------- M36 折线 ---------------- */
const ptsA = [[7, 50], [27, 41], [47, 45], [67, 30], [87, 35], [107, 20], [127, 25], [147, 12]]
const linePath = computed(() => ptsA.map((p, i) => (i ? 'L' : 'M') + p[0] + ' ' + p[1]).join(' '))

/* ---------------- M37 柱状 ---------------- */
const bars = [42, 64, 38, 78, 56, 88, 46]

/* ---------------- M38 面积 ---------------- */
const areaD = 'M7 52 L27 41 L47 45 L67 30 L87 35 L107 20 L127 25 L147 12 L147 66 L7 66 Z'
const areaLine = 'M7 52 L27 41 L47 45 L67 30 L87 35 L107 20 L127 25 L147 12'

/* ---------------- M39 热力 ---------------- */
const heat = Array.from({ length: 28 }, (_, i) => ({
  v: 0.15 + ((i * 37) % 9) / 11,
  d: (i % 7) * 40 + Math.floor(i / 7) * 70,
}))

/* ---------------- M41 雷达 ---------------- */
const radarOuter = '80,10 143,44 143,102 80,136 17,102 17,44'
const radarVal = '80,26 128,50 121,92 80,118 36,94 39,52'

/* ---------------- M42 Sparkline morph ---------------- */
const sparkA = [[7, 44], [27, 30], [47, 38], [67, 22], [87, 34], [107, 18], [127, 28], [147, 14]]
const sparkB = [[7, 20], [27, 34], [47, 26], [67, 40], [87, 24], [107, 36], [127, 18], [147, 26]]
const sparkD = ref('')
const morphing = ref(false)

function build(arr) {
  return arr.map((p, i) => (i ? 'L' : 'M') + p[0].toFixed(1) + ' ' + p[1].toFixed(1)).join(' ')
}
function morphTo(target) {
  const fromY = currentY.slice()
  morphing.value = true
  const t0 = performance.now()
  const dur = 760
  function step(t) {
    const p = Math.min(1, (t - t0) / dur)
    const e = 1 - Math.pow(1 - p, 3)
    sparkD.value = target
      .map((pt, i) => (i ? 'L' : 'M') + pt[0].toFixed(1) + ' ' + (fromY[i] + (pt[1] - fromY[i]) * e).toFixed(1))
      .join(' ')
    if (p < 1) requestAnimationFrame(step)
    else { morphing.value = false; currentY = target.map(q => q[1]) }
  }
  requestAnimationFrame(step)
}
let currentY = sparkA.map(p => p[1])

let rafId = 0
function toggleSpark() {
  if (morphing.value) return
  const next = currentY[0] === sparkA[0][1] ? sparkB : sparkA
  morphTo(next)
}

onMounted(() => {
  sparkD.value = build(sparkA)
})
onBeforeUnmount(() => cancelAnimationFrame(rafId))
</script>

<template>
  <!-- ============ M35 环形 ============ -->
  <div v-if="kind === 'ring'" class="c">
    <svg viewBox="0 0 72 72" width="72" height="72" :key="k">
      <circle cx="36" cy="36" :r="R" fill="none" stroke="rgba(255,255,255,.08)" stroke-width="7" />
      <circle cx="36" cy="36" :r="R" fill="none" stroke="var(--ac)" stroke-width="7"
              stroke-linecap="round"
              :stroke-dasharray="C"
              :stroke-dashoffset="C"
              transform="rotate(-90 36 36)"
              class="ring-run"
              :style="{ '--to': C * (1 - ringPct) }" />
    </svg>
    <span class="ring-txt">{{ Math.round(ringPct * 100) }}<i>%</i></span>
  </div>

  <!-- ============ M36 折线 ============ -->
  <div v-else-if="kind === 'line'" class="c">
    <svg viewBox="0 0 160 70" width="90%" height="82" :key="k">
      <defs>
        <linearGradient id="lg-line" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stop-color="var(--ac)" stop-opacity=".26"/>
          <stop offset="1" stop-color="var(--ac)" stop-opacity="0"/>
        </linearGradient>
      </defs>
      <path :d="linePath + ' L147 66 L7 66 Z'" fill="url(#lg-line)" class="line-fill" />
      <path :d="linePath" fill="none" stroke="var(--ac)" stroke-width="2"
            stroke-linecap="round" stroke-linejoin="round"
            class="line-draw" :style="{ '--len': 220 }" />
      <circle cx="147" cy="12" r="2.8" fill="var(--ac)" class="line-dot" />
    </svg>
    <span class="hint">requests / 24h</span>
  </div>

  <!-- ============ M37 柱状 ============ -->
  <div v-else-if="kind === 'bars'" class="c">
    <svg viewBox="0 0 160 72" width="90%" height="82" :key="k">
      <g v-for="(v, i) in bars" :key="i">
        <rect :x="9 + i * 21" :y="66 - v * .62" width="13" :height="v * .62"
              rx="3.5" fill="var(--ac)" :opacity=".36 + (v / 120)"
              class="bar-g" :style="{ '--i': i }" />
      </g>
    </svg>
    <span class="hint">按模型调用量</span>
  </div>

  <!-- ============ M38 面积 ============ -->
  <div v-else-if="kind === 'area'" class="c">
    <svg viewBox="0 0 160 72" width="90%" height="82" :key="k">
      <defs>
        <linearGradient id="lg-area" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stop-color="var(--ac)" stop-opacity=".42"/>
          <stop offset="1" stop-color="var(--ac)" stop-opacity=".02"/>
        </linearGradient>
      </defs>
      <path :d="areaD" fill="url(#lg-area)" class="area-in" />
      <path :d="areaLine" fill="none" stroke="var(--ac)" stroke-width="1.8"
            stroke-linecap="round" stroke-linejoin="round"
            class="line-draw" :style="{ '--len': 200 }" />
    </svg>
    <span class="hint">积分消耗趋势</span>
  </div>

  <!-- ============ M39 热力 ============ -->
  <div v-else-if="kind === 'heat'" class="heat-wrap" :key="k">
    <div class="heat">
      <span v-for="(h, i) in heat" :key="i" class="cell"
            :style="{ '--o': h.v, animationDelay: h.d + 'ms' }" />
    </div>
    <span class="hint">近 4 周活跃度</span>
  </div>

  <!-- ============ M41 雷达 ============ -->
  <div v-else-if="kind === 'radar'" class="c">
    <svg viewBox="0 0 160 146" width="88%" height="88" :key="k">
      <g class="radar-in">
        <polygon :points="radarOuter" fill="none" stroke="rgba(255,255,255,.10)" stroke-width="1" />
        <polygon points="80,24 131,52 131,96 80,124 29,96 29,52" fill="none" stroke="rgba(255,255,255,.06)" stroke-width="1" />
        <polygon points="80,38 115,60 115,88 80,110 45,88 45,60" fill="none" stroke="rgba(255,255,255,.05)" stroke-width="1" />
        <polygon :points="radarVal" fill="rgba(255,138,61,.20)" stroke="var(--ac)" stroke-width="1.8"
                 stroke-linejoin="round" class="radar-poly" />
        <g class="radar-dots" v-for="(p, i) in radarVal.split(' ')" :key="i">
          <circle :cx="p.split(',')[0]" :cy="p.split(',')[1]" r="2.2" fill="var(--ac)"
                  :style="{ animationDelay: (i * 70 + 260) + 'ms' }" class="rdot" />
        </g>
      </g>
    </svg>
    <span class="hint">模型能力评估</span>
  </div>

  <!-- ============ M42 Sparkline morph ============ -->
  <div v-else-if="kind === 'spark'" class="c spark-c" @click.stop="toggleSpark">
    <svg viewBox="0 0 160 58" width="90%" height="70">
      <path :d="sparkD" fill="none" stroke="var(--ac)" stroke-width="2"
            stroke-linecap="round" stroke-linejoin="round" class="spark-path" />
      <path :d="sparkD" fill="none" stroke="var(--ac)" stroke-width="7" opacity=".14"
            stroke-linecap="round" stroke-linejoin="round" />
    </svg>
    <span class="hint">点我切换周期</span>
  </div>

  <div v-else class="center"><span class="hint">—</span></div>
</template>

<style scoped>
.c{position:relative;display:grid;place-items:center;width:100%;height:100%;padding:0 8px}
.hint{position:absolute;bottom:4px;left:0;right:0;text-align:center;font-size:9px;color:var(--tx-4)}
.center{display:grid;place-items:center;height:100%}

/* 环形 */
.ring-run{
  animation:ring-draw 1200ms cubic-bezier(.16,1,.3,1) both;
}
@keyframes ring-draw{
  from{stroke-dashoffset:var(--len,176)}
  to{stroke-dashoffset:var(--to)}
}
.ring-txt{
  position:absolute;font-size:14px;font-weight:700;letter-spacing:-.03em;
  font-variant-numeric:tabular-nums;
  animation:k-fade-in 600ms 300ms var(--e-out) both;
}
.ring-txt i{font-style:normal;font-size:9px;color:var(--tx-4);margin-left:1px}

/* 折线 / 面积 */
.line-draw{
  stroke-dasharray:var(--len);
  stroke-dashoffset:var(--len);
  animation:k-draw 1100ms var(--e-out) both;
}
.line-fill{opacity:0;animation:k-fade-in 700ms 300ms var(--e-out) both}
.line-dot{opacity:0;animation:k-dot-pop 420ms 900ms var(--e-out) both}

.bar-g{
  transform-box:fill-box;transform-origin:bottom center;
  animation:k-grow-v 720ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 60ms);
}
.area-in{transform-box:fill-box;transform-origin:bottom center;
  animation:k-area 780ms var(--e-out) both}

/* 热力 */
.heat-wrap{width:100%;height:100%;display:grid;place-items:center;position:relative}
.heat{display:grid;grid-template-columns:repeat(7,1fr);gap:3.5px;padding:0 12px}
.cell{
  width:15px;height:15px;border-radius:3.5px;
  background:var(--ac);opacity:calc(.10 + var(--o) * .62);
  animation:heat-on 520ms var(--e-out) both;
}
@keyframes heat-on{from{opacity:0;transform:scale(.55)}to{opacity:calc(.10 + var(--o) * .62);transform:none}}

/* 雷达 */
.radar-in{transform-origin:80px 73px;animation:k-radar 720ms var(--e-out) both}
.radar-poly{
  transform-box:fill-box;transform-origin:center;
  animation:k-scale-in 640ms 120ms var(--e-out) both;
}
.rdot{animation:k-dot-pop 420ms var(--e-out) both;opacity:0}

/* spark */
.spark-c{cursor:pointer}
.spark-path{filter:drop-shadow(0 0 6px rgba(255,138,61,.3))}
</style>

<script setup>
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'

const props = defineProps({ kind: String, no: Number, tick: Number })
const k = computed(() => props.no + '-' + props.tick)

/* ---------------- M43 光斑跟随 ---------------- */
const spot = ref(null)
function move(e) {
  const el = spot.value
  if (!el) return
  const r = el.getBoundingClientRect()
  el.style.setProperty('--sx', ((e.clientX - r.left) / r.width * 100).toFixed(1) + '%')
  el.style.setProperty('--sy', ((e.clientY - r.top) / r.height * 100).toFixed(1) + '%')
}

/* ---------------- M45 扫光 ---------------- */
const sheenKey = ref(0)
function runSheen() { sheenKey.value++ }

/* ---------------- M46 流式光标 ---------------- */
const typed = ref('')
const full = '正在路由到可用账号…'
let typeTimer = null
function runType() {
  typed.value = ''
  let i = 0
  clearInterval(typeTimer)
  typeTimer = setInterval(() => {
    typed.value = full.slice(0, ++i)
    if (i >= full.length) clearInterval(typeTimer)
  }, 92)
}
onMounted(runType)

/* ---------------- M48 波纹 ---------------- */
const waves = ref(0)

/* ---------------- M49 粒子连线 ---------------- */
const canvas = ref(null)
let rafId = 0, ctx = null, W = 0, H = 0, dpr = 1
let dots = []
let alive = false

function initParticles() {
  const cv = canvas.value
  if (!cv) return
  const r = cv.getBoundingClientRect()
  dpr = Math.min(window.devicePixelRatio || 1, 2)
  W = r.width; H = r.height
  cv.width = W * dpr; cv.height = H * dpr
  ctx = cv.getContext('2d')
  ctx.scale(dpr, dpr)
  dots = Array.from({ length: 22 }, () => ({
    x: Math.random() * W, y: Math.random() * H,
    vx: (Math.random() - .5) * 12, vy: (Math.random() - .5) * 12,
    r: 0.8 + Math.random() * 1.2,
  }))
  alive = true
  let last = performance.now()
  function loop(t) {
    if (!alive) return
    const dt = Math.min((t - last) / 1000, .05); last = t
    ctx.clearRect(0, 0, W, H)
    for (const d of dots) {
      d.x += d.vx * dt; d.y += d.vy * dt
      if (d.x < 0 || d.x > W) d.vx *= -1
      if (d.y < 0 || d.y > H) d.vy *= -1
      ctx.beginPath()
      ctx.arc(d.x, d.y, d.r, 0, Math.PI * 2)
      ctx.fillStyle = 'rgba(255,180,130,.55)'
      ctx.fill()
    }
    for (let i = 0; i < dots.length; i++) {
      for (let j = i + 1; j < dots.length; j++) {
        const a = dots[i], b = dots[j]
        const dist = Math.hypot(a.x - b.x, a.y - b.y)
        if (dist < 46) {
          ctx.beginPath()
          ctx.moveTo(a.x, a.y); ctx.lineTo(b.x, b.y)
          ctx.strokeStyle = `rgba(255,150,100,${(1 - dist / 46) * .22})`
          ctx.lineWidth = .7
          ctx.stroke()
        }
      }
    }
    rafId = requestAnimationFrame(loop)
  }
  rafId = requestAnimationFrame(loop)
}

onMounted(() => {
  if (props.kind === 'particles') initParticles()
})
onBeforeUnmount(() => {
  alive = false
  cancelAnimationFrame(rafId)
  clearInterval(typeTimer)
})
</script>

<template>
  <!-- M43 光斑 -->
  <div v-if="kind === 'spot'" ref="spot" class="c spot" :key="k"
       @pointermove.stop="move">
    <div class="spot-glow" />
    <div class="spot-card">
      <b>aio</b>
      <span class="mini-bar" />
      <span class="mini-bar w2" />
    </div>
  </div>

  <!-- M44 噪点 -->
  <div v-else-if="kind === 'noise'" class="c noise-c" :key="k">
    <div class="nz" />
    <div class="noise-orb" />
    <span class="hint">颗粒缓慢漂移</span>
  </div>

  <!-- M45 扫光 -->
  <div v-else-if="kind === 'sheen'" class="c" :key="k">
    <div class="sheenbox glass-2 glass sheen" :class="{ run: sheenKey > 0 }"
         :key="sheenKey" @click.stop="runSheen">
      <span>玻璃面板</span>
    </div>
    <span class="hint">点我触发高光</span>
  </div>

  <!-- M46 流式光标 -->
  <div v-else-if="kind === 'caret'" class="c stream" :key="k" @click.stop="runType">
    <div class="bubble">
      <span class="stxt">{{ typed }}</span><i class="caret" />
    </div>
    <span class="hint">AI 输出节奏</span>
  </div>

  <!-- M47 呼吸 -->
  <div v-else-if="kind === 'breathe'" class="c" :key="k">
    <div class="led-wrap">
      <i class="halo" /><i class="led" />
    </div>
    <span class="hint">在线 · 2.4s 循环</span>
  </div>

  <!-- M48 波纹 -->
  <div v-else-if="kind === 'ripple'" class="c" :key="k" @click.stop="waves++">
    <div class="wave-wrap">
      <i v-for="n in 2" :key="waves + '-' + n" class="wave" :style="{ animationDelay: (n - 1) * 380 + 'ms' }" />
      <i class="wave-core" />
    </div>
    <span class="hint">点我扩散</span>
  </div>

  <!-- M49 粒子连线 -->
  <div v-else-if="kind === 'particles'" class="c part" :key="k">
    <canvas ref="canvas" />
    <span class="hint">登录页背景（低开销）</span>
  </div>

  <!-- M50 极光 -->
  <div v-else-if="kind === 'aurora'" class="c aurora-c" :key="k">
    <div class="aurora"><i /><i /><i /></div>
    <span class="hint">全站背景底噪</span>
  </div>

  <div v-else class="center"><span class="hint">—</span></div>
</template>

<style scoped>
.c{position:relative;width:100%;height:100%;display:grid;place-items:center;overflow:hidden}
.hint{position:absolute;bottom:4px;left:0;right:0;text-align:center;font-size:9px;color:var(--tx-4);z-index:4}
.center{display:grid;place-items:center;height:100%}

/* M43 */
.spot{cursor:crosshair}
.spot-glow{
  position:absolute;inset:0;
  background:radial-gradient(160px 130px at var(--sx,50%) var(--sy,40%),
    rgba(255,200,160,.16),rgba(255,138,61,.05) 42%,transparent 70%);
  transition:background-position 60ms linear;
}
.spot-card{
  position:relative;z-index:2;width:92px;padding:9px;border-radius:11px;
  background:rgba(255,255,255,.045);border:1px solid rgba(255,255,255,.10);
  display:flex;flex-direction:column;gap:5px;
  -webkit-backdrop-filter:blur(8px);backdrop-filter:blur(8px);
}
.spot-card b{font-size:10px;font-weight:640}
.mini-bar{height:3px;width:80%;border-radius:2px;background:rgba(255,255,255,.14);display:block}
.mini-bar.w2{width:52%}

/* M44 */
.noise-c{background:linear-gradient(160deg,rgba(255,255,255,.05),rgba(255,255,255,.01))}
.nz{position:absolute;inset:-12%;background:var(--noise);opacity:.55;
  animation:k-noise 7s steps(4,end) infinite;mix-blend-mode:overlay}
.noise-orb{
  width:52px;height:52px;border-radius:50%;
  background:radial-gradient(circle at 35% 30%,rgba(255,190,150,.55),rgba(255,138,61,.12) 60%,transparent);
  filter:blur(2px);
}

/* M45 */
.sheenbox{
  width:120px;height:58px;border-radius:13px;display:grid;place-items:center;
  font-size:11px;color:var(--tx-2);cursor:pointer;
}
.sheenbox:active{transform:scale(.98)}

/* M46 */
.stream{align-content:center;padding:0 12px}
.bubble{
  width:100%;padding:9px 11px;border-radius:10px 10px 10px 3px;
  background:rgba(255,255,255,.055);border:1px solid rgba(255,255,255,.08);
  font-size:10px;color:var(--tx-2);min-height:38px;display:flex;align-items:center;
}
.caret{
  display:inline-block;width:5px;height:11px;margin-left:2px;border-radius:1.5px;
  background:var(--ac);animation:k-blink 1.05s steps(1,end) infinite;
  vertical-align:middle;
}

/* M47 */
.led-wrap{position:relative;width:40px;height:40px;display:grid;place-items:center}
.led{width:9px;height:9px;border-radius:50%;background:var(--ok);
  box-shadow:0 0 12px 2px rgba(84,201,138,.6)}
.halo{position:absolute;width:26px;height:26px;border-radius:50%;
  background:rgba(84,201,138,.22);animation:k-breathe 2.4s var(--e-in-out) infinite}

/* M48 */
.wave-wrap{position:relative;width:56px;height:56px;display:grid;place-items:center}
.wave{
  position:absolute;width:22px;height:22px;border-radius:50%;
  border:1.5px solid rgba(255,138,61,.55);
  animation:k-wave 2.6s var(--e-out) infinite;
}
.wave-core{width:9px;height:9px;border-radius:50%;background:var(--ac);
  box-shadow:0 0 14px 3px rgba(255,138,61,.45)}

/* M49 */
.part canvas{position:absolute;inset:0;width:100%;height:100%}

/* M50 */
.aurora-c{background:#080809}
.aurora-c .aurora{inset:-25%}
</style>

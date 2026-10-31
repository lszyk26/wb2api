<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { api, fmt, state } from '../lib/api.js'

const emit = defineEmits(['need-login'])

const cfg = ref(null)
const hist = ref([])
const err = ref('')
const busy = ref(false)
const pool = ref('free')
const result = ref(null)

/* ---------------- 动效状态机 ----------------
   idle → charging（蓄力抖动） → burst（礼盒炸开） → reveal（结果卡 + 烟花）
   阶段推进靠 animationend，不用 setTimeout 猜时长；只在动画被关掉时用兜底定时器。 */
const phase = ref('idle')
const showResult = ref(false)
const shownPts = ref(0)

const reduceMotion = () => {
  try { return window.matchMedia('(prefers-reduced-motion: reduce)').matches } catch (_) { return false }
}
function perfLevel() {
  try { return document.documentElement.getAttribute('data-perf') || 'full' } catch (_) { return 'full' }
}
/* 流畅档 / 系统减少动效 → 整段动效跳过，直接出结果 */
function motionOff() {
  const p = perfLevel()
  return p === 'smooth' || reduceMotion()
}
/* 精简档：粒子减半、不做屏震 */
function liteMode() {
  return perfLevel() === 'lite'
}

/* ---------------- 幂等键 ---------------- */
let inflight = null
function uuid() {
  try {
    if (window.crypto && window.crypto.randomUUID) return window.crypto.randomUUID()
    const b = new Uint8Array(16)
    window.crypto.getRandomValues(b)
    return Array.from(b, x => x.toString(16).padStart(2, '0')).join('')
  } catch (_) {
    return 'k' + Date.now().toString(36) + Math.random().toString(36).slice(2, 10)
  }
}

const ERR = {
  LOTTERY_DISABLED: '抽奖活动暂未开放',
  LOTTERY_TEMP_EMAIL_FORBIDDEN: '临时邮箱账号不能参与抽奖',
  LOTTERY_POOL_INVALID: '奖池不存在',
  LOTTERY_INSUFFICIENT_BONUS: '付费点数不足，先充值或兑换卡密',
  LOTTERY_FREE_LIMIT_REACHED: '今天的免费次数已经用完了，明天 0 点恢复',
  LOTTERY_NEWBIE_LOCKED: '注册满 24 小时后才能抽付费池',
  LOTTERY_RATE_LIMITED: '手速太快了，歇一分钟再来',
  LOTTERY_POOL_WEIGHT_INVALID: '奖池维护中（概率配置校验未通过），请稍后再来',
  LOTTERY_IDEMPOTENCY_CONFLICT: '这次请求已处理过，刷新页面看结果',
  LOTTERY_INTERNAL: '抽奖失败了，请稍后重试',
}
function mapErr(e) {
  const c = e?.data?.error?.code
  return ERR[c] || e.message || '抽奖失败'
}

async function load() {
  try {
    cfg.value = await api.lotteryConfig()
    const h = await api.lotteryHistory(1, 20)
    hist.value = h.draws || []
  } catch (e) {
    err.value = mapErr(e)
  }
}
onMounted(load)

const enabled = computed(() => !!cfg.value?.enabled)
const freeLeft = computed(() => cfg.value?.free_remaining_today ?? 0)
const dailyFree = computed(() => cfg.value?.daily_free_limit ?? 1)
const cost = computed(() => cfg.value?.cost_points ?? 2000)
const bonus = computed(() => cfg.value?.bonus_points ?? 0)
const lottery = computed(() => cfg.value?.lottery_points ?? 0)
const bonusExpired = computed(() => !!cfg.value?.bonus_expired)
const tempEmail = computed(() => !!cfg.value?.temp_email)
const newbie = computed(() => !!cfg.value?.newbie_locked)
const showProb = computed(() => !!cfg.value?.show_probability)
const unlimited = computed(() => !!cfg.value?.unlimited)
const authed = computed(() => !!state.token)

const paidPrizes = computed(() => cfg.value?.prizes || [])
const freePrizes = computed(() => cfg.value?.free_prizes || [])
const shownPrizes = computed(() => (pool.value === 'free' ? freePrizes.value : paidPrizes.value))

function canDraw(p) {
  if (!authed.value) return true
  if (!enabled.value || busy.value) return false
  if (unlimited.value) return true
  if (tempEmail.value) return false
  if (p === 'free') return freeLeft.value > 0
  if (newbie.value) return false
  if (bonusExpired.value) return false
  return bonus.value >= cost.value
}

const blocked = computed(() => {
  if (!authed.value) return '登录后即可参与抽奖（概率与奖池可先查看）'
  if (!enabled.value) return cfg.value?.disabled_reason === 'weight_invalid' ? '奖池概率配置校验未通过，抽奖已暂停' : '抽奖活动暂未开放'
  if (tempEmail.value) return '临时邮箱账号不能参与抽奖'
  if (unlimited.value) return ''
  if (pool.value === 'paid' && newbie.value) return '注册满 24 小时后才能抽付费池'
  if (pool.value === 'paid' && bonusExpired.value) return '付费点数已过期，先充值后再抽'
  if (pool.value === 'paid' && bonus.value < cost.value) return `付费点数不足，抽一次需要 ${fmt.n(cost.value)} 点`
  if (pool.value === 'free' && freeLeft.value <= 0) return '今天的免费次数用完了，明天 0 点恢复'
  return ''
})

/* ---------------- 奖项档位 ---------------- */
function tierOf(v) {
  if (!v || v <= 0) return 0
  if (v < 500) return 1
  if (v < 5000) return 2
  if (v < 50000) return 3
  return 4
}

/* ---------------- 抽奖流程 ---------------- */
async function doDraw() {
  if (!state.token) { emit('need-login'); return }
  if (!canDraw(pool.value)) return
  err.value = ''
  busy.value = true
  result.value = null
  showResult.value = false
  shownPts.value = 0
  phase.value = motionOff() ? 'idle' : 'charging'

  let key
  if (inflight && inflight.pool === pool.value) key = inflight.key
  else { key = uuid(); inflight = { pool: pool.value, key } }

  const t0 = performance.now()
  try {
    const r = await api.lotteryDraw(pool.value, key)
    inflight = null

    // 最短悬念：让蓄力动画走完再炸，避免「点完瞬间出结果」的廉价感
    const minSpin = phase.value === 'charging' ? 1050 : 0
    const elapsed = performance.now() - t0
    if (elapsed < minSpin) await sleep(minSpin - elapsed)

    result.value = { ...r, poolName: r.pool === 'free' ? '免费池' : '付费池', tier: tierOf(r.prize_value) }

    if (phase.value === 'charging') {
      phase.value = 'burst'
      nextTick(() => {
        startFx(result.value ? result.value.tier : 0)
        playBurst()
      })
    } else {
      reveal()
    }
    load()
  } catch (e) {
    if (e.status) inflight = null
    err.value = mapErr(e)
    phase.value = 'idle'
  } finally {
    busy.value = false
  }
}

/* 等「炸开」那条动画真正跑完再揭晓。
   注意不要用 @animationend：scoped CSS 会把 keyframes 改名为 flashPop-xxxx，
   而且阶段切换时移除规则会触发 animationcancel 而不是 end。
   这里直接用 Web Animations API 等 finished，并设一个硬上限防止卡死。 */
async function playBurst() {
  const el = flashEl.value
  // 以 CSS 里真实时长为准（620ms），再挂一次性监听提前收尾；
  // 不要依赖 @animationend：scoped CSS 会把 keyframes 改名。
  let ms = 660
  try {
    const d = parseFloat(getComputedStyle(el).animationDuration)
    if (d > 0) ms = Math.round(d * 1000)
  } catch (_) {}
  await new Promise((res) => {
    let done = false
    const fin = () => {
      if (done) return
      done = true
      if (el) {
        el.removeEventListener('animationend', fin)
        el.removeEventListener('animationcancel', fin)
      }
      res()
    }
    if (el) {
      el.addEventListener('animationend', fin, { once: true })
      el.addEventListener('animationcancel', fin, { once: true })
    }
    setTimeout(fin, ms + 140) // 动画被系统关掉时的兜底
  })
  onBurstDone()
}

function sleep(ms) { return new Promise(r => setTimeout(r, ms)) }

function onBurstDone() {
  if (phase.value !== 'burst') return
  reveal()
}
function reveal() {
  phase.value = 'reveal'
  nextTick(() => {
    showResult.value = true
    countUp(result.value?.prize_value || 0)
  })
}

function countUp(target) {
  if (motionOff()) { shownPts.value = target; return }
  const t0 = performance.now()
  const dur = target >= 10000 ? 900 : 480
  const step = (t) => {
    const k = Math.min(1, (t - t0) / dur)
    const e = 1 - Math.pow(1 - k, 3)
    shownPts.value = Math.round(target * e)
    if (k < 1) requestAnimationFrame(step)
    else shownPts.value = target
  }
  requestAnimationFrame(step)
}

function closeResult() {
  showResult.value = false
  stopFx()
  setTimeout(() => { phase.value = 'idle'; result.value = null }, 220)
}
function again() {
  closeResult()
  setTimeout(() => doDraw(), 260)
}

/* ---------------- 烟花（canvas 粒子） ---------------- */
const flashEl = ref(null)
const cvs = ref(null)
let ctx = null, raf = 0, parts = [], fxEnd = 0, W = 0, H = 0

const PALETTE = ['#ff8a3d', '#ffb37a', '#ffd9b8', '#fff1e2', '#ffc76b']

function resizeCanvas() {
  const c = cvs.value
  if (!c) return
  const dpr = Math.min(2, window.devicePixelRatio || 1)
  W = window.innerWidth
  H = window.innerHeight
  c.width = Math.floor(W * dpr)
  c.height = Math.floor(H * dpr)
  c.style.width = W + 'px'
  c.style.height = H + 'px'
  ctx = c.getContext('2d')
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
}

function spark(cx, cy, n, opt = {}) {
  const speed = opt.speed || 1
  for (let i = 0; i < n; i++) {
    const a = Math.random() * Math.PI * 2
    const v = (1.4 + Math.random() * 3.6) * speed
    parts.push({
      kind: 's', x: cx, y: cy,
      vx: Math.cos(a) * v, vy: Math.sin(a) * v - 1.1,
      life: 1, decay: 0.007 + Math.random() * 0.012,
      r: (opt.r || 2.1) * (0.55 + Math.random() * 0.9),
      c: opt.color || PALETTE[(Math.random() * PALETTE.length) | 0],
    })
  }
}
function confetti(n) {
  for (let i = 0; i < n; i++) {
    parts.push({
      kind: 'c', x: Math.random() * W, y: -20 - Math.random() * H * 0.3,
      vx: (Math.random() - 0.5) * 1.6, vy: 1.2 + Math.random() * 2.4,
      rot: Math.random() * Math.PI, vr: (Math.random() - 0.5) * 0.22,
      w: 4 + Math.random() * 5, h: 8 + Math.random() * 8,
      life: 1, decay: 0.0035,
      c: ['#ff8a3d', '#ffd9b8', '#fff1e2', '#ffb37a'][(Math.random() * 4) | 0],
    })
  }
}
function ring(cx, cy, color, decay) {
  parts.push({ kind: 'r', x: cx, y: cy, r: 8, life: 1, decay: decay || 0.022, c: color || '#ffd9b8' })
}

function startFx(tier) {
  if (motionOff()) return
  if (!cvs.value) return
  resizeCanvas()
  const k = liteMode() ? 0.45 : 1
  const cx = W / 2, cy = H * 0.42
  if (tier <= 0) {
    // 空手也要有一下：小团灰烟 + 暗环，明确「这一抽用掉了」
    ring(cx, cy, '#8a7f76', 0.55)
    spark(cx, cy, Math.round(26 * k), { color: '#9a8f86', speed: 0.7, r: 1.5 })
    fxEnd = performance.now() + 1600
    if (!raf) raf = requestAnimationFrame(tick)
    return
  }
  const shots = [0, 2, 3, 5, 8][tier]
  ring(cx, cy)
  spark(cx, cy, Math.round((tier >= 3 ? 90 : 60) * k), { speed: tier >= 3 ? 1.25 : 1 })

  for (let i = 0; i < shots; i++) {
    setTimeout(() => {
      if (!ctx) return
      const bx = W * (0.2 + Math.random() * 0.6)
      const by = H * (0.18 + Math.random() * 0.3)
      spark(bx, by, Math.round((tier >= 3 ? 70 : 50) * k), { speed: 1.1 })
      if (tier >= 3) spark(bx, by, Math.round(30 * k), { color: '#fff1e2', speed: 0.75, r: 1.6 })
    }, 120 + i * 190)
  }
  if (tier >= 3) confetti(Math.round(70 * k))
  if (tier >= 4) confetti(Math.round(110 * k))

  fxEnd = performance.now() + 3200
  if (!raf) raf = requestAnimationFrame(tick)
}

function tick(now) {
  raf = 0
  if (!ctx) return
  // 拖尾：擦掉一部分旧帧，透明画布上用 destination-out
  ctx.globalCompositeOperation = 'destination-out'
  ctx.fillStyle = 'rgba(0,0,0,.22)'
  ctx.fillRect(0, 0, W, H)

  ctx.globalCompositeOperation = 'lighter'
  for (let i = parts.length - 1; i >= 0; i--) {
    const p = parts[i]
    p.life -= p.decay
    if (p.life <= 0) { parts.splice(i, 1); continue }
    if (p.kind === 'c') {
      p.x += p.vx; p.y += p.vy; p.vy += 0.035; p.rot += p.vr
      ctx.globalCompositeOperation = 'source-over'
      ctx.save()
      ctx.translate(p.x, p.y)
      ctx.rotate(p.rot)
      ctx.globalAlpha = Math.min(1, p.life * 1.6)
      ctx.fillStyle = p.c
      ctx.fillRect(-p.w / 2, -p.h / 2, p.w, p.h)
      ctx.restore()
      ctx.globalCompositeOperation = 'lighter'
      continue
    }
    if (p.kind === 'r') {
      p.r += 9
      ctx.globalAlpha = p.life * 0.5
      ctx.strokeStyle = p.c
      ctx.lineWidth = 2
      ctx.beginPath()
      ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2)
      ctx.stroke()
      continue
    }
    p.vy += 0.045
    p.vx *= 0.985
    p.vy *= 0.985
    p.x += p.vx
    p.y += p.vy
    ctx.globalAlpha = Math.max(0, p.life)
    ctx.fillStyle = p.c
    ctx.beginPath()
    ctx.arc(p.x, p.y, p.r * (0.4 + p.life * 0.6), 0, Math.PI * 2)
    ctx.fill()
  }
  ctx.globalAlpha = 1
  ctx.globalCompositeOperation = 'source-over'

  if (parts.length > 0) {
    raf = requestAnimationFrame(tick)
  } else if (now < fxEnd) {
    raf = requestAnimationFrame(tick)
  } else {
    ctx.clearRect(0, 0, W, H)
  }
}

function stopFx() {
  if (raf) { cancelAnimationFrame(raf); raf = 0 }
  parts = []
  if (ctx) ctx.clearRect(0, 0, W, H)
}

onMounted(() => {
  window.addEventListener('resize', onResize, { passive: true })
})
function onResize() { if (cvs.value) resizeCanvas() }

onBeforeUnmount(() => {
  stopFx()
  inflight = null
  window.removeEventListener('resize', onResize)
})

/* ---------------- 小工具 ---------------- */
function fmtWhen(s) { return s ? String(s).replace('T', ' ').slice(0, 16) : '' }
function untilText(s) { return s ? String(s).slice(0, 10) : '' }
function rank(v) {
  if (v >= 50000) return 'legend'
  if (v >= 5000) return 'big'
  if (v >= 500) return 'mid'
  return ''
}
</script>

<template>
  <section class="lot container">
    <header class="ahead">
      <div>
        <p class="eyebrow">LOTTERY</p>
        <h1>抽奖</h1>
        <p class="sub" style="margin-top:8px">
          每天 1 次免费抽；额外抽一次消耗 <b>{{ fmt.n(cost) }}</b> 付费点数。
          中奖一律进入「赠点」，永久有效、只能用于 API 消费，<b>不能再次抽奖</b>。
        </p>
      </div>
    </header>

    <p v-if="err" class="err">{{ err }}</p>

    <template v-if="cfg">
      <div class="wallets">
        <article class="glass glass-2 w">
          <p class="eyebrow">付费点数</p>
          <b>{{ fmt.n(bonus) }}</b>
          <i>充值 / 卡密 / 加油包，有有效期{{ cfg.bonus_until ? '（至 ' + untilText(cfg.bonus_until) + '）' : '' }}</i>
          <span v-if="bonusExpired" class="pill err">已过期</span>
        </article>
        <article class="glass glass-2 w">
          <p class="eyebrow">赠点</p>
          <b class="ac">{{ fmt.n(lottery) }}</b>
          <i>抽奖所得 · 永久有效 · 只能消费，不能抽奖</i>
        </article>
        <article class="glass glass-2 w">
          <p class="eyebrow">今日次数</p>
          <b v-if="unlimited">无限</b>
          <b v-else>{{ freeLeft }}<small> / {{ dailyFree }}</small></b>
          <i v-if="unlimited">开发者账号 · 免费与付费均不限次、不扣点</i>
          <i v-else>免费剩余 · 今日已付费抽 {{ cfg.paid_draws_today }} 次</i>
        </article>
      </div>

      <p class="notice glass glass-1">{{ cfg.announcement }}</p>

      <div class="pooltabs">
        <button class="pt" :data-on="pool === 'free' ? 1 : 0" @click="pool = 'free'">
          免费池 <span v-if="freeLeft > 0" class="dot" />
        </button>
        <button class="pt" :data-on="pool === 'paid' ? 1 : 0" @click="pool = 'paid'">
          付费池 · {{ fmt.n(cost) }} 点/次
        </button>
      </div>

      <article class="glass glass-2 drawbox" :data-shake="phase === 'burst' && result && result.tier >= 4 ? 1 : 0">
        <!-- 礼盒舞台 -->
        <div class="stage" :data-phase="phase">
          <div class="gift">
            <div class="gift-lid">
              <span class="gift-bow" />
            </div>
            <div class="gift-box">
              <span class="rib-v" />
              <span class="rib-h" />
            </div>
            <div class="gift-glow" />
          </div>
          <div ref="flashEl" class="flash" />
          <div class="ring" />
        </div>

        <p class="stage-hint">
          <template v-if="phase === 'charging'">正在拆礼盒…</template>
          <template v-else-if="phase === 'burst'">砰！</template>
          <template v-else-if="blocked">{{ blocked }}</template>
          <template v-else>{{ pool === 'free' ? '点一下拆开今天的礼盒' : '花 ' + fmt.n(cost) + ' 点拆一个' }}</template>
        </p>

        <div class="drawbar">
          <button class="btn pri lg" :disabled="busy || !canDraw(pool)" @click="doDraw">
            <template v-if="!authed">登录后抽奖</template>
            <template v-else-if="busy || phase === 'charging'">拆礼盒中…</template>
            <template v-else-if="unlimited">拆 1 个 · 开发者不扣点</template>
            <template v-else-if="pool === 'free'">免费拆 1 次</template>
            <template v-else>消耗 {{ fmt.n(cost) }} 点拆 1 次</template>
          </button>
          <p v-if="!blocked" class="hint dim">
            {{ unlimited ? '开发者账号：次数不限、不扣点（流水标记 dev，不计入返奖统计）'
               : pool === 'free' ? '每天 0 点重置免费次数' : '中奖点数直接进入赠点，不能再次抽奖' }}
          </p>
        </div>

        <!-- 奖池一览 -->
        <div class="prizes">
          <div v-for="p in shownPrizes" :key="p.id" class="pr" :class="rank(p.points)"
               :data-out="p.remaining_stock === 0 ? 1 : 0">
            <b>{{ p.name }}</b>
            <span v-if="showProb">{{ p.probability }}</span>
            <i v-if="p.daily_stock >= 0">{{ p.remaining_stock === 0 ? '今日已抽完' : '剩 ' + p.remaining_stock }}</i>
          </div>
        </div>
      </article>

      <article v-if="showProb" class="glass glass-2 panel">
        <h3>概率公示</h3>
        <p class="tiny dim" style="margin:0 0 10px">
          权重合计 10000；大奖每日限量，先到先得，售罄后该奖项权重并入「0 点」。
          兑换口径：{{ fmt.n(cfg.point_per_yuan) }} 点 = 1 元。
        </p>
        <div class="probs">
          <div v-for="p in shownPrizes" :key="'x' + p.id" class="prow">
            <span class="pn">{{ p.name }}</span>
            <div class="pbar"><i :style="{ width: Math.max(1.5, parseFloat(p.probability)) + '%' }" /></div>
            <span class="pv">{{ p.probability }}</span>
          </div>
        </div>
      </article>

      <article class="glass glass-2 panel">
        <h3>最近抽奖 <i class="tiny dim">{{ hist.length }}</i></h3>
        <p v-if="!hist.length" class="tiny dim" style="margin:0">还没有抽奖记录。</p>
        <ul v-else class="hlist">
          <li v-for="h in hist" :key="h.draw_id">
            <span class="pill" :class="h.pool === 'free' ? '' : 'ac'">{{ h.pool === 'free' ? '免费' : '付费' }}</span>
            <b :class="rank(h.prize_value)">{{ h.prize_name }}</b>
            <span class="tiny dim">扣 {{ fmt.n(h.cost_points) }} 点</span>
            <span class="tiny dim when">{{ fmtWhen(h.created_at) }}</span>
          </li>
        </ul>
      </article>
    </template>

    <!-- 烟花 + 结果（Teleport 到 body：页面切换的 transform 会劫持 fixed 定位） -->
    <Teleport to="body">
      <canvas v-if="phase === 'burst' || phase === 'reveal'" ref="cvs" class="lot-fx" />
      <Transition name="lotrd">
        <div v-if="showResult && result" class="lot-scrim" @click.self="closeResult">
          <div class="lot-wrap" :data-shake="result.tier >= 4 ? 1 : 0">
          <div class="lot-card" :data-tier="result.tier">
            <div class="lot-sheen" />
            <p class="lot-eyebrow">{{ result.poolName }}</p>
            <h2 class="lot-prize">{{ result.prize_name }}</h2>
            <p v-if="result.prize_value > 0" class="lot-sum">
              <b>+{{ fmt.n(shownPts) }}</b> 赠点已存入余额
            </p>
            <p v-else class="lot-sum dim">谢谢参与，下次再来</p>
            <p class="lot-sub">
              {{ result.prize_value > 0 ? '永久有效 · 只能用于 API 消费 · 不能再次抽奖' : '这一抽没有奖品' }}
            </p>
            <p class="lot-meta">
              扣 {{ fmt.n(result.cost_points) }} 点 · 付费余额 {{ fmt.n(result.bonus_points_after) }} · 赠点 {{ fmt.n(result.lottery_points_after) }}
            </p>
            <div class="lot-btns">
              <button class="btn" @click="closeResult">知道了</button>
              <button class="btn pri" :disabled="!canDraw(pool)" @click="again">再拆一个</button>
            </div>
          </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
/* ---------------- 顶部余额 ---------------- */
.wallets{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px;margin:14px 0}
.w{padding:16px 18px;display:flex;flex-direction:column;gap:4px;position:relative}
.w .eyebrow{margin:0}
.w b{font:700 24px/1.2 var(--sf);color:var(--tx-1)}
.w b.ac{color:#ffcba6}
.w b small{font-size:13px;color:var(--tx-3);font-weight:500}
.w i{font-style:normal;font-size:11.5px;color:var(--tx-3);line-height:1.5}
.w .pill{position:absolute;right:14px;top:14px}
.notice{margin:0 0 14px;padding:11px 14px;font-size:12.5px;color:var(--tx-2);line-height:1.6}
.pooltabs{display:flex;gap:8px;margin-bottom:12px}
.pt{flex:1;height:38px;border-radius:11px;border:1px solid var(--line);background:rgba(255,255,255,.04);
    color:var(--tx-2);font-size:13px;cursor:pointer;transition:background .2s,border-color .2s,color .2s;position:relative}
.pt[data-on="1"]{background:var(--ac-dim);border-color:var(--ac-line);color:#ffcba6;font-weight:600}
.pt .dot{width:6px;height:6px;border-radius:50%;background:#7fe3a1;display:inline-block;margin-left:6px;vertical-align:middle}

.drawbox{padding:16px 18px 18px}

/* ---------------- 礼盒 ---------------- */
.stage{position:relative;height:188px;display:flex;align-items:center;justify-content:center}
.gift{position:relative;width:120px;height:106px;transform-origin:50% 90%;
  animation:giftBob 3.4s ease-in-out infinite alternate;will-change:transform}
.gift-box{position:absolute;left:0;bottom:0;width:120px;height:80px;border-radius:10px 10px 12px 12px;
  background:linear-gradient(180deg,#ffa463,#e06a26);
  box-shadow:0 12px 30px rgba(255,138,61,.26),inset 0 1px 0 rgba(255,255,255,.38)}
.gift-lid{position:absolute;left:-7px;top:0;width:134px;height:32px;border-radius:9px;
  background:linear-gradient(180deg,#ffb87e,#ef8038);
  box-shadow:0 4px 16px rgba(0,0,0,.34),inset 0 1px 0 rgba(255,255,255,.45);transform-origin:20% 100%}
.gift-bow{position:absolute;left:50%;top:-16px;width:44px;height:20px;margin-left:-22px;border-radius:50%;
  border:3px solid #ffe3c8;border-bottom-color:transparent;opacity:.9}
.rib-v{position:absolute;left:50%;top:0;bottom:0;width:15px;margin-left:-7.5px;
  background:linear-gradient(180deg,#ffe6cf,#ffcd9c);opacity:.9}
.rib-h{position:absolute;left:0;right:0;top:50%;height:15px;margin-top:-7.5px;
  background:linear-gradient(90deg,#ffe6cf,#ffcd9c);opacity:.9}
.gift-glow{position:absolute;left:50%;bottom:-16px;width:170px;height:30px;margin-left:-85px;border-radius:50%;
  background:radial-gradient(50% 50% at 50% 50%,rgba(255,138,61,.5),transparent 72%);
  animation:glowPulse 3.4s ease-in-out infinite alternate}

@keyframes giftBob{from{transform:translateY(0) rotate(-.6deg)}to{transform:translateY(-7px) rotate(.6deg)}}
@keyframes glowPulse{from{opacity:.55;transform:scale(.94)}to{opacity:.95;transform:scale(1.06)}}

/* 蓄力：抖动 + 收紧 */
.stage[data-phase="charging"] .gift{animation:giftShake .17s linear infinite}
.stage[data-phase="charging"] .gift-glow{animation:glowCharge .5s ease-in-out infinite alternate}
@keyframes giftShake{
  0%{transform:translate(0,0) rotate(0) scale(.97)}
  25%{transform:translate(-3px,-1px) rotate(-2.2deg) scale(.985)}
  50%{transform:translate(3px,1px) rotate(2.4deg) scale(1)}
  75%{transform:translate(-2px,1px) rotate(-1.6deg) scale(1.01)}
  100%{transform:translate(0,0) rotate(0) scale(.97)}
}
@keyframes glowCharge{
  from{opacity:.5;transform:scale(.9)}
  to{opacity:1;transform:scale(1.25)}
}

/* 炸开 */
.stage[data-phase="burst"] .gift,.stage[data-phase="reveal"] .gift{animation:none;opacity:0;transition:opacity .12s}
.stage[data-phase="burst"] .gift-lid{animation:lidFly .58s cubic-bezier(.32,.72,0,1) forwards}
.stage[data-phase="burst"] .gift-box{animation:boxPop .34s cubic-bezier(.32,.72,0,1) forwards}
.stage[data-phase="burst"] .gift-glow{animation:glowBurst .6s ease-out forwards}
@keyframes lidFly{
  0%{transform:translate(0,0) rotate(0) scale(1)}
  55%{transform:translate(-26px,-96px) rotate(-38deg) scale(1.04)}
  100%{transform:translate(-58px,-186px) rotate(-72deg) scale(.8);opacity:0}
}
@keyframes boxPop{
  0%{transform:scale(1.06)}
  45%{transform:scale(1.16) translateY(4px)}
  100%{transform:scale(.62) translateY(16px);opacity:0}
}
@keyframes glowBurst{
  0%{opacity:1;transform:scale(1.2)}
  100%{opacity:0;transform:scale(2.6)}
}
.flash{position:absolute;left:50%;top:52%;width:120px;height:120px;margin:-60px 0 0 -60px;border-radius:50%;
  background:radial-gradient(circle,rgba(255,247,238,.95),rgba(255,138,61,.55) 42%,rgba(255,138,61,0) 72%);
  opacity:0;pointer-events:none;will-change:transform,opacity}
.stage[data-phase="burst"] .flash{animation:flashPop .62s cubic-bezier(.22,.9,.3,1) forwards}
@keyframes flashPop{
  0%{opacity:0;transform:scale(.25)}
  20%{opacity:.95;transform:scale(.75)}
  100%{opacity:0;transform:scale(2.8)}
}
.ring{position:absolute;left:50%;top:52%;width:100px;height:100px;margin:-50px 0 0 -50px;border-radius:50%;
  border:2px solid rgba(255,201,150,.75);opacity:0;pointer-events:none}
.stage[data-phase="burst"] .ring{animation:ringOut .78s cubic-bezier(.2,.8,.3,1) forwards}
@keyframes ringOut{
  0%{opacity:.85;transform:scale(.35)}
  100%{opacity:0;transform:scale(2.5)}
}

/* 巨奖屏震（只动 transform） */
.drawbox[data-shake="1"]{animation:shakeHard .5s cubic-bezier(.36,.07,.19,.97) 1}
@keyframes shakeHard{
  10%{transform:translate(-3px,1px)} 20%{transform:translate(4px,-2px)}
  30%{transform:translate(-5px,2px)} 40%{transform:translate(5px,-1px)}
  55%{transform:translate(-3px,2px)} 70%{transform:translate(3px,-2px)}
  85%{transform:translate(-1px,1px)} 100%{transform:none}
}

.stage-hint{margin:2px 0 12px;text-align:center;font-size:12.5px;color:var(--tx-3);min-height:19px}
.drawbar{display:flex;flex-direction:column;gap:8px;align-items:flex-start}
.hint{margin:0;font-size:12px;color:#ff9a9a}
.hint.dim{color:var(--tx-3)}

/* ---------------- 奖池/面板 ---------------- */
.prizes{display:grid;grid-template-columns:repeat(auto-fill,minmax(96px,1fr));gap:8px;margin-top:16px}
.pr{border:1px solid var(--line);border-radius:10px;padding:9px 8px;text-align:center;background:rgba(255,255,255,.03)}
.pr b{display:block;font-size:13px;color:var(--tx-2)}
.pr span{display:block;font-size:10.5px;color:var(--tx-3);margin-top:2px}
.pr i{display:block;font-style:normal;font-size:10px;color:#f0d29a;margin-top:3px}
.pr[data-out="1"]{opacity:.42}
.pr.mid b{color:#ffe0c4}
.pr.big b{color:#ffcba6}
.pr.legend{border-color:var(--ac-line);background:var(--ac-dim)}
.pr.legend b{color:#ffb37a;font-weight:700}
.panel{padding:16px 18px;margin-top:14px}
.panel h3{margin:0 0 8px;font-size:14px;display:flex;gap:6px;align-items:center}
.probs{display:flex;flex-direction:column;gap:6px}
.prow{display:flex;align-items:center;gap:10px;font-size:12px}
.pn{min-width:72px;color:var(--tx-2)}
.pbar{flex:1;height:6px;border-radius:4px;background:rgba(255,255,255,.06);overflow:hidden}
.pbar i{display:block;height:100%;background:linear-gradient(90deg,rgba(255,138,61,.55),rgba(255,179,122,.85));border-radius:4px}
.pv{min-width:58px;text-align:right;color:var(--tx-3);font-variant-numeric:tabular-nums}
.hlist{list-style:none;margin:0;padding:0;max-height:320px;overflow:auto}
.hlist li{display:flex;align-items:center;gap:10px;padding:8px 0;border-bottom:1px solid var(--line);font-size:12.5px}
.hlist li:last-child{border-bottom:0}
.hlist b{color:var(--tx-1)}
.hlist b.big,.hlist b.legend{color:#ffb37a}
.hlist .when{margin-left:auto;white-space:nowrap}

/* ---------------- 全屏烟花 ---------------- */
.lot-fx{position:fixed;inset:0;width:100vw;height:100vh;z-index:9550;pointer-events:none}

/* ---------------- 结果卡 ---------------- */
.lot-scrim{position:fixed;inset:0;z-index:9600;background:rgba(4,4,6,.58);
  display:flex;align-items:center;justify-content:center;padding:20px;
  padding-bottom:calc(20px + env(safe-area-inset-bottom))}
.lot-wrap{width:100%;max-width:390px}
.lot-wrap[data-shake="1"]{animation:shakeHard .55s .1s cubic-bezier(.36,.07,.19,.97) 1}
.lot-card{position:relative;width:100%;padding:30px 24px 24px;border-radius:22px;
  overflow:hidden;text-align:center;color:var(--tx-1);
  background:linear-gradient(180deg,rgba(28,26,24,.92),rgba(18,17,16,.94));
  border:1px solid rgba(255,255,255,.13);
  box-shadow:0 30px 80px rgba(0,0,0,.62);
  animation:cardIn .44s cubic-bezier(.32,.72,0,1) backwards}
@keyframes cardIn{
  from{opacity:0;transform:translateY(14px) scale(.9)}
  to{opacity:1;transform:none}
}
.lot-card[data-tier="2"]{border-color:rgba(255,138,61,.34);box-shadow:0 30px 80px rgba(0,0,0,.62),0 0 0 1px rgba(255,138,61,.12) inset}
.lot-card[data-tier="3"],.lot-card[data-tier="4"]{
  border-color:rgba(255,171,90,.55);
  box-shadow:0 30px 90px rgba(0,0,0,.7),0 0 60px rgba(255,138,61,.22),0 0 0 1px rgba(255,171,90,.18) inset}
.lot-sheen{position:absolute;left:-40%;top:-60%;width:60%;height:220%;pointer-events:none;
  background:linear-gradient(100deg,transparent,rgba(255,255,255,.16),transparent);
  animation:sheen 1.1s .12s cubic-bezier(.32,.72,0,1) backwards}
@keyframes sheen{from{transform:translateX(0) rotate(8deg);opacity:0}
  to{transform:translateX(280%) rotate(8deg);opacity:1}}
.lot-eyebrow{margin:0;font-size:10.5px;letter-spacing:.16em;color:var(--tx-4);text-transform:uppercase}
.lot-prize{margin:8px 0 10px;font-size:34px;line-height:1.1;font-weight:750;letter-spacing:-.02em;color:var(--tx-1)}
.lot-card[data-tier="2"] .lot-prize{color:#ffd0aa}
.lot-card[data-tier="3"] .lot-prize,.lot-card[data-tier="4"] .lot-prize{
  color:#ffb37a;text-shadow:0 0 26px rgba(255,138,61,.45)}
.lot-card[data-tier="4"] .lot-prize{font-size:40px}
.lot-sum{margin:0;font-size:13px;color:var(--tx-2)}
.lot-sum b{color:#ffcba6;font-size:16px}
.lot-sum.dim{color:var(--tx-3)}
.lot-sub{margin:6px 0 0;font-size:11.5px;color:var(--tx-3);line-height:1.6}
.lot-meta{margin:12px 0 0;font-size:11px;color:var(--tx-4)}
.lot-btns{display:flex;gap:8px;justify-content:center;margin-top:20px}

.lotrd-enter-active{transition:opacity .22s}
.lotrd-leave-active{transition:opacity .2s}
.lotrd-enter-from,.lotrd-leave-to{opacity:0}
.lotrd-enter-from .lot-card{animation:cardIn .44s cubic-bezier(.32,.72,0,1) backwards}

.err{color:#ff9a9a;font-size:12.5px;margin:0 0 10px}

/* ---------------- 降级 ---------------- */
html[data-perf="smooth"] .gift,
html[data-perf="smooth"] .gift-glow,
html[data-perf="smooth"] .flash,
html[data-perf="smooth"] .ring,
html[data-perf="smooth"] .lot-sheen,
html[data-perf="smooth"] .drawbox[data-shake="1"]{animation:none !important}
html[data-perf="lite"] .gift-glow{animation:none;opacity:.8}

@media (prefers-reduced-motion: reduce){
  .gift,.gift-glow,.flash,.ring,.lot-sheen,.lot-card,.drawbox[data-shake="1"]{animation:none !important}
  .lot-card{opacity:1;transform:none}
}

@media (max-width:820px){
  .stage{height:168px}
  .prizes{grid-template-columns:repeat(auto-fill,minmax(84px,1fr))}
  .lot-prize{font-size:28px}
  .lot-card[data-tier="4"] .lot-prize{font-size:32px}
}
</style>

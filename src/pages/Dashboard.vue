<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { spring } from '../lib/spring.js'
import { api, fmt } from '../lib/api.js'

const emit = defineEmits(['go'])

const d = ref(null)
const loading = ref(true)
const err = ref('')

async function load() {
  try {
    d.value = await api.overview()
    err.value = ''
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
    animateNums()
  }
}
onMounted(load)

let timer = null
onMounted(() => { timer = setInterval(load, 60000) })
onBeforeUnmount(() => clearInterval(timer))

/* ---------------- 数字滚动 ---------------- */
const nCalls = ref(0)
const nPoints = ref(0)
const nModels = ref(0)
const nLeft = ref(0)

function animateNums() {
  if (!d.value) return
  spring(nCalls.value, d.value.today.calls, v => { nCalls.value = v }, { stiffness: 160, damping: 26 })
  spring(nPoints.value, d.value.today.points, v => { nPoints.value = v }, { stiffness: 160, damping: 26 })
  spring(nModels.value, (d.value.by_model || []).length, v => { nModels.value = v }, { stiffness: 160, damping: 26 })
  spring(nLeft.value, d.value.quota.left_today, v => { nLeft.value = v }, { stiffness: 160, damping: 26 })
}

/* ---------------- 趋势图 ---------------- */
const VB_W = 640, VB_H = 178, PAD = 6
const series = computed(() => d.value?.series || [])
const maxCalls = computed(() => Math.max(4, ...series.value.map(s => s.calls)) * 1.15)
const xOf = i => PAD + i * ((VB_W - PAD * 2) / Math.max(1, series.value.length - 1))
const yOf = v => VB_H - 24 - (v / maxCalls.value) * (VB_H - 44)
const linePath = computed(() => series.value.map((s, i) => (i ? 'L' : 'M') + xOf(i).toFixed(1) + ' ' + yOf(s.calls).toFixed(1)).join(' '))
const areaPath = computed(() => {
  if (!series.value.length) return ''
  const n = series.value.length
  return linePath.value + ` L${xOf(n - 1).toFixed(1)} ${VB_H - 12} L${PAD} ${VB_H - 12} Z`
})

const hover = ref(-1)
function onMove(e) {
  const r = e.currentTarget.getBoundingClientRect()
  const x = ((e.clientX - r.left) / r.width) * VB_W
  const n = series.value.length
  if (!n) return
  const step = (VB_W - PAD * 2) / Math.max(1, n - 1)
  hover.value = Math.max(0, Math.min(n - 1, Math.round((x - PAD) / step)))
}

/* ---------------- 池 ---------------- */
const pool = computed(() => d.value?.pool || {})
const poolOk = computed(() => pool.value.ok)

/* ---------------- 配额环 ---------------- */
const HC = 2 * Math.PI * 22
const quotaPct = computed(() => {
  if (!d.value) return 0
  const q = d.value.quota
  const total = q.daily + (q.bonus || 0)
  if (!total) return 0
  return Math.max(0, Math.min(100, (q.left_today / total) * 100))
})
const hOff = computed(() => HC * (1 - quotaPct.value / 100))

/* 加油包 / 学生认证 */
const redeemCode = ref('')
const redeemMsg = ref('')
const redeeming = ref(false)
async function doRedeem() {
  if (!redeemCode.value.trim() || redeeming.value) return
  redeeming.value = true
  redeemMsg.value = ''
  try {
    const r = await api.redeem(redeemCode.value.trim())
    redeemMsg.value = r.kind === 'points'
      ? `已兑换 ${r.points} 点`
      : `档位已生效：${r.plan.toUpperCase()}，到期 ${r.until}`
    redeemCode.value = ''
    await load()
  } catch (e) {
    redeemMsg.value = e.message
  } finally { redeeming.value = false }
}

const logs = computed(() => d.value?.logs || [])
const byModel = computed(() => d.value?.by_model || [])

const prevDelta = computed(() => {
  if (!d.value) return null
  const t = d.value.today, p = d.value.today.prev_calls
  if (!p) return null
  return ((t.calls - p) / p) * 100
})

const now = ref(new Date())
let clock = null
onMounted(() => { clock = setInterval(() => (now.value = new Date()), 30000) })
onBeforeUnmount(() => clearInterval(clock))
const clockStr = computed(() => now.value.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }))
</script>

<template>
  <section class="dash container">
    <!-- Hero -->
    <header class="hero">
      <div class="hero-l">
        <p class="eyebrow">OVERVIEW</p>
        <h1>{{ d ? `你好，${d.user?.username || ''}。` : '正在读取…' }}</h1>
        <p class="lead">
          今日已用 <b>{{ fmt.pts(d?.quota?.used_today || 0) }}</b> 点，
          剩余 <b>{{ fmt.pts(d?.quota?.left_today || 0) }}</b> 点
          <template v-if="d?.quota?.bonus">，另有加油包 {{ fmt.pts(d.quota.bonus) }} 点</template>。
          <span class="dim">（{{ clockStr }} · 每日 0 点重置）</span>
        </p>
        <div class="hero-actions">
          <button class="btn pri lg" @click="emit('go', 'keys')">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="8" cy="12" r="3.6"/><path d="M11.6 12H21M18 12v3M15 12v2" stroke-linecap="round"/>
            </svg>
            管理密钥
          </button>
          <button class="btn lg" @click="emit('go', 'playground')">测试一个回答</button>
          <button class="btn lg ghost" @click="emit('go', 'pricing')">查看档位</button>
        </div>
        <div class="hero-meta">
          <span class="pill" :class="poolOk ? 'ok' : 'warn'">
            <i class="live" :class="{ off: !poolOk }" />{{ poolOk ? '账号池在线' : '账号池待连接' }}
          </span>
          <span class="pill" v-if="d?.user?.plan">{{ (d.user.plan || '').toUpperCase() }}</span>
          <span class="pill" v-else>游客</span>
        </div>
      </div>

      <!-- 配额环 -->
      <div class="hero-r">
        <div class="health glass glass-2">
          <div class="hring">
            <svg viewBox="0 0 56 56" width="104" height="104">
              <circle cx="28" cy="28" r="22" fill="none" stroke="rgba(255,255,255,.07)" stroke-width="4" />
              <circle cx="28" cy="28" r="22" fill="none" stroke="url(#qg)" stroke-width="4"
                      stroke-linecap="round" :stroke-dasharray="HC" :stroke-dashoffset="hOff"
                      transform="rotate(-90 28 28)" />
              <defs>
                <linearGradient id="qg" x1="0" y1="1" x2="1" y2="0">
                  <stop offset="0" stop-color="#ff8a3d" stop-opacity=".45"/>
                  <stop offset="1" stop-color="#ffb37a"/>
                </linearGradient>
              </defs>
            </svg>
            <div class="hval">
              <b>{{ Math.round(quotaPct) }}</b><span>%</span>
            </div>
          </div>
          <div class="hinfo">
            <p class="eyebrow">TODAY QUOTA</p>
            <h3>剩余 {{ fmt.pts(d?.quota?.left_today || 0) }} 点</h3>
            <p class="tiny dim">每日 {{ fmt.pts(d?.quota?.daily || 0) }} 点 · 已用 {{ fmt.pts(d?.quota?.used_today || 0) }} 点 · 0 点重置</p>
            <div class="hspark">
              <i v-for="(s, i) in series.slice(-12)" :key="i"
                 :style="{ height: Math.max(8, (s.calls / maxCalls) * 100) + '%', '--i': i }" />
            </div>
          </div>
        </div>
      </div>
    </header>

    <p v-if="err" class="errline">{{ err }}</p>

    <!-- 指标 -->
    <div class="metrics">
      <article class="mcard glass glass-1">
        <div class="mlabel"><span class="ico">▤</span>今日调用</div>
        <b class="mnum">{{ Math.round(nCalls).toLocaleString() }}</b>
        <span class="msub">
          较昨日
          <em v-if="prevDelta !== null" :class="prevDelta >= 0 ? 'up' : 'down'">
            {{ prevDelta >= 0 ? '+' : '' }}{{ prevDelta.toFixed(0) }}%
          </em>
          <em v-else class="dim">—</em>
        </span>
      </article>
      <article class="mcard glass glass-1">
        <div class="mlabel"><span class="ico">◈</span>今日消耗</div>
        <b class="mnum">{{ fmt.pts(nPoints) }}</b>
        <span class="msub">点数 · 昨日 {{ fmt.pts(d?.today?.prev_points || 0) }}</span>
      </article>
      <article class="mcard glass glass-1">
        <div class="mlabel"><span class="ico">◎</span>在用模型</div>
        <b class="mnum">{{ Math.round(nModels) }}</b>
        <span class="msub">近 7 天有调用</span>
      </article>
      <article class="mcard glass glass-1">
        <div class="mlabel"><span class="ico">✓</span>失败请求</div>
        <b class="mnum" :class="{ warnnum: d?.today?.errors > 0 }">{{ d?.today?.errors ?? 0 }}</b>
        <span class="msub">今日 · 4xx/5xx</span>
      </article>
    </div>

    <!-- 趋势 + 模型 -->
    <div class="main-grid">
      <article class="panel glass glass-1">
        <div class="phead">
          <div>
            <p class="eyebrow">TRAFFIC</p>
            <h3>近 24 小时</h3>
          </div>
          <span class="pill">{{ series.reduce((a, b) => a + b.calls, 0) }} 次</span>
        </div>

        <div class="chart-wrap" @pointermove="onMove" @pointerleave="hover = -1">
          <svg :viewBox="`0 0 ${VB_W} ${VB_H}`" preserveAspectRatio="none" class="chart">
            <defs>
              <linearGradient id="ag" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0" stop-color="#ff8a3d" stop-opacity=".32"/>
                <stop offset="1" stop-color="#ff8a3d" stop-opacity="0"/>
              </linearGradient>
            </defs>
            <g stroke="rgba(255,255,255,.05)" stroke-width="1">
              <line v-for="i in 3" :key="i" x1="0" :x2="VB_W"
                    :y1="VB_H - 24 - i * (VB_H - 44) / 3" :y2="VB_H - 24 - i * (VB_H - 44) / 3" />
            </g>
            <path v-if="areaPath" :d="areaPath" fill="url(#ag)" />
            <path v-if="linePath" :d="linePath" fill="none" stroke="#ff8a3d" stroke-width="2.2"
                  stroke-linecap="round" stroke-linejoin="round" class="cline" />
            <g v-if="hover >= 0 && series[hover]">
              <line :x1="xOf(hover)" :x2="xOf(hover)" y1="8" :y2="VB_H - 24"
                    stroke="rgba(255,255,255,.2)" stroke-dasharray="3 3" />
              <circle :cx="xOf(hover)" :cy="yOf(series[hover].calls)" r="4" fill="#0a0a0c"
                      stroke="#ff8a3d" stroke-width="2.2" />
            </g>
          </svg>
          <div v-if="hover >= 0 && series[hover]" class="tip glass glass-3"
               :style="{ left: (xOf(hover) / VB_W * 100) + '%', top: (yOf(series[hover].calls) / VB_H * 100) + '%' }">
            <b>{{ series[hover].calls }}</b><span>次</span>
            <i>{{ series[hover].hour }}</i>
          </div>
        </div>
      </article>

      <article class="panel glass glass-1">
        <div class="phead">
          <div>
            <p class="eyebrow">MODELS</p>
            <h3>近 7 天</h3>
          </div>
          <button class="btn sm ghost" @click="emit('go', 'usage')">明细 →</button>
        </div>
        <div v-if="byModel.length" class="mlist">
          <div v-for="(m, i) in byModel.slice(0, 6)" :key="m.model" class="mrow" :style="{ '--i': i }">
            <span class="mname mono trunc">{{ m.model }}</span>
            <span class="mcalls">{{ fmt.n(m.calls) }}</span>
            <span class="mpts">{{ fmt.pts(m.points) }}</span>
          </div>
        </div>
        <p v-else class="empty-line">
          还没有调用记录。建一把密钥，或者在「对话测试」里发一条试试。
        </p>
      </article>
    </div>

    <!-- 兑换 + 日志 -->
    <div class="main-grid two">
      <article class="panel glass glass-1">
        <div class="phead">
          <div>
            <p class="eyebrow">REDEEM</p>
            <h3>兑换码</h3>
          </div>
        </div>
        <div class="redeem">
          <input v-model.trim="redeemCode" class="input" placeholder="WB2-XXXX-XXXX"
                 @keydown.enter="doRedeem" />
          <button class="btn pri" :disabled="redeeming || !redeemCode" @click="doRedeem">
            {{ redeeming ? '兑换中…' : '兑换' }}
          </button>
        </div>
        <p v-if="redeemMsg" class="redeem-msg">{{ redeemMsg }}</p>
        <p class="tiny dim">
          卡密可以兑换点数或档位，也可以送给同学。赞助后我会把卡密发给你。
        </p>
      </article>

      <article class="panel glass glass-1">
        <div class="phead">
          <div>
            <p class="eyebrow">RECENT</p>
            <h3>最近调用</h3>
          </div>
          <button class="btn sm ghost" @click="emit('go', 'usage')">全部 →</button>
        </div>
        <ul class="recent">
          <li v-for="(l, i) in logs.slice(0, 6)" :key="l.id" :style="{ '--i': i }">
            <span class="rdot" :class="l.status < 400 ? 'ok' : 'err'" />
            <span class="rmono mono trunc">{{ l.model }}</span>
            <span class="rpts">{{ l.points.toFixed(2) }}</span>
            <span class="rtime tiny dim">{{ l.ago }}</span>
          </li>
          <li v-if="!logs.length" class="rempty">还没有记录</li>
        </ul>
      </article>
    </div>
  </section>
</template>

<style scoped>
.dash{padding-top:26px;display:flex;flex-direction:column;gap:15px}
.hero{display:grid;grid-template-columns:1.3fr .7fr;gap:22px;align-items:center;padding:6px 0 2px}
@media (max-width:900px){ .hero{grid-template-columns:1fr;gap:16px} }

.hero h1{margin:8px 0 12px;letter-spacing:-.038em;animation:k-fade-up 620ms var(--e-out) both}
.lead{max-width:52ch;color:var(--tx-3);font-size:14px;line-height:1.78;
  animation:k-fade-up 620ms 70ms var(--e-out) both}
.lead b{color:var(--tx-1);font-weight:650;font-variant-numeric:tabular-nums}
.hero-actions{display:flex;gap:9px;margin-top:19px;flex-wrap:wrap;
  animation:k-fade-up 620ms 140ms var(--e-out) both}
.hero-meta{display:flex;align-items:center;gap:8px;margin-top:15px;flex-wrap:wrap;
  animation:k-fade-up 620ms 200ms var(--e-out) both}
.live{width:6px;height:6px;border-radius:50%;background:var(--ok);
  box-shadow:0 0 8px 1px rgba(84,201,138,.6);animation:k-breathe 2.4s var(--e-in-out) infinite}
.live.off{background:var(--warn);box-shadow:0 0 8px 1px rgba(232,181,82,.5)}

.hero-r{animation:k-fade-up 680ms 120ms var(--e-out) both}
.health{display:flex;align-items:center;gap:16px;padding:16px 18px 16px 16px}
.hring{position:relative;flex:none;display:grid;place-items:center}
.hval{position:absolute;display:flex;align-items:baseline;gap:1px;letter-spacing:-.03em}
.hval b{font-size:17px;font-weight:700;font-variant-numeric:tabular-nums}
.hval span{font-size:9px;color:var(--tx-4)}
.hinfo{min-width:0;display:flex;flex-direction:column;gap:5px}
.hinfo h3{font-size:14px}
.hspark{display:flex;align-items:flex-end;gap:2.5px;height:22px;margin-top:3px}
.hspark i{width:4px;border-radius:2px;
  background:linear-gradient(180deg,rgba(255,138,61,.85),rgba(255,138,61,.2));
  animation:k-grow-v 620ms var(--e-out) both;animation-delay:calc(var(--i,0) * 42ms + 200ms)}

.errline{font-size:12px;color:#f6b1ac;background:rgba(240,103,95,.10);
  border:1px solid rgba(240,103,95,.22);border-radius:9px;padding:8px 11px}

.metrics{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}
@media (max-width:900px){ .metrics{grid-template-columns:repeat(2,1fr)} }
.mcard{padding:15px 16px 14px;display:flex;flex-direction:column;gap:3px;
  animation:k-fade-up 560ms var(--e-out) both;transition:transform 320ms var(--e-out),border-color 280ms}
.mcard:hover{transform:translateY(-2px);border-color:rgba(255,255,255,.16)}
.mcard:nth-child(1){animation-delay:40ms}
.mcard:nth-child(2){animation-delay:90ms}
.mcard:nth-child(3){animation-delay:140ms}
.mcard:nth-child(4){animation-delay:190ms}
.mlabel{display:flex;align-items:center;gap:5px;font-size:11.5px;color:var(--tx-3);font-weight:600}
.mlabel .ico{color:var(--tx-4);font-size:10px}
.mnum{font-size:29px;font-weight:720;letter-spacing:-.04em;line-height:1.16;
  font-variant-numeric:tabular-nums;margin-top:3px}
.mnum.warnnum{color:#f0d29a}
.msub{font-size:10.5px;color:var(--tx-4)}
.up{font-style:normal;color:#8fe8b6}
.down{font-style:normal;color:#f6b1ac}

.main-grid{display:grid;grid-template-columns:1.55fr .95fr;gap:11px}
.main-grid.two{grid-template-columns:1fr 1fr}
@media (max-width:1000px){ .main-grid,.main-grid.two{grid-template-columns:1fr} }

.panel{padding:15px 17px 13px;display:flex;flex-direction:column;gap:11px;min-width:0}
.phead{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.phead h3{margin-top:3px}

.chart-wrap{position:relative}
.chart{width:100%;height:166px;display:block;overflow:visible}
.cline{stroke-dasharray:1400;stroke-dashoffset:0;animation:cl 1200ms var(--e-out) both}
@keyframes cl{from{stroke-dashoffset:1400}to{stroke-dashoffset:0}}
.tip{position:absolute;transform:translate(-50%,-128%);padding:6px 9px;border-radius:9px;
  display:flex;align-items:baseline;gap:3px;font-size:11px;pointer-events:none;white-space:nowrap;
  animation:k-scale-in 170ms var(--e-out) both}
.tip b{font-size:14px;font-weight:700;font-variant-numeric:tabular-nums}
.tip span{font-size:9px;color:var(--tx-4)}
.tip i{font-style:normal;font-size:9px;color:var(--tx-4);margin-left:5px;padding-left:6px;
  border-left:1px solid var(--line)}

.mlist{display:flex;flex-direction:column}
.mrow{display:grid;grid-template-columns:1fr 56px 62px;align-items:center;gap:8px;
  min-height:32px;border-top:1px solid rgba(255,255,255,.045);
  animation:k-fade-up 460ms var(--e-out) both;animation-delay:calc(var(--i,0) * 50ms)}
.mrow:first-child{border-top:0}
.mname{font-size:11.5px;color:var(--tx-2)}
.mcalls,.mpts{font-size:11px;text-align:right;font-variant-numeric:tabular-nums}
.mcalls{color:var(--tx-4)}
.mpts{color:var(--ac-2)}
.empty-line{font-size:12px;color:var(--tx-4);line-height:1.7;padding:12px 0}

.redeem{display:flex;gap:8px}
.redeem .input{flex:1;height:38px;font-size:12.5px}
.redeem .btn{flex:none;height:38px}
.redeem-msg{font-size:11.5px;color:#a9e8c5;background:rgba(84,201,138,.09);
  border:1px solid rgba(84,201,138,.2);border-radius:8px;padding:7px 10px}

.recent{display:flex;flex-direction:column}
.recent li{display:flex;align-items:center;gap:9px;min-height:32px;
  border-top:1px solid rgba(255,255,255,.045);
  animation:k-fade-up 440ms var(--e-out) both;animation-delay:calc(var(--i,0) * 40ms)}
.recent li:first-child{border-top:0}
.rdot{width:5px;height:5px;border-radius:50%;flex:none}
.rdot.ok{background:var(--ok)}
.rdot.err{background:var(--err)}
.rmono{flex:1;min-width:0;font-size:11px;color:var(--tx-2)}
.rpts{font-size:11px;color:var(--tx-3);font-variant-numeric:tabular-nums}
.rtime{flex:none;width:58px;text-align:right}
.rempty{color:var(--tx-4);font-size:12px;padding:14px 0;border-top:0}
</style>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { api, fmt } from '../lib/api.js'

const emit = defineEmits([])

const days = ref(7)
const usage = ref(null)
const logs = ref([])
const page = ref(1)
const total = ref(0)
const filter = ref('')
const loading = ref(true)
const err = ref('')
const me = ref(null)

async function loadUsage() {
  try {
    usage.value = await api.usage(days.value)
  } catch (e) { err.value = e.message }
}

async function loadLogs() {
  loading.value = true
  try {
    const f = filter.value === 'error' ? 'status=error' : filter.value === 'ok' ? 'status=ok' : ''
    const d = await api.logs(page.value, f)
    logs.value = d.logs || []
    total.value = d.total || 0
  } catch (e) { err.value = e.message }
  finally { loading.value = false }
}

onMounted(async () => {
  await Promise.all([loadUsage(), loadLogs()])
  try { me.value = await api.me() } catch (_) {}
})
watch(days, loadUsage)
watch([page, filter], loadLogs)

/* ---------- 图表 ---------- */
const VB_W = 640, VB_H = 170, PAD = 8
const series = computed(() => usage.value?.days || [])
const maxPts = computed(() => Math.max(1, ...series.value.map(d => d.points * 1.2)))
const xOf = i => PAD + i * ((VB_W - PAD * 2) / Math.max(1, series.value.length - 1))
const yOf = v => VB_H - 22 - (v / maxPts.value) * (VB_H - 40)

const linePath = computed(() => {
  if (!series.value.length) return ''
  return series.value.map((d, i) => (i ? 'L' : 'M') + xOf(i).toFixed(1) + ' ' + yOf(d.points).toFixed(1)).join(' ')
})
const areaPath = computed(() => {
  if (!series.value.length) return ''
  const n = series.value.length
  return linePath.value + ` L${xOf(n - 1).toFixed(1)} ${VB_H - 12} L${PAD} ${VB_H - 12} Z`
})

const hover = ref(-1)
function onMove(e) {
  const el = e.currentTarget
  const r = el.getBoundingClientRect()
  const x = ((e.clientX - r.left) / r.width) * VB_W
  const n = series.value.length
  if (!n) return
  const step = (VB_W - PAD * 2) / Math.max(1, n - 1)
  hover.value = Math.max(0, Math.min(n - 1, Math.round((x - PAD) / step)))
}

/* ---------- 导出 CSV ---------- */
function exportCsv() {
  const rows = [['时间', '模型', '输入tokens', '输出tokens', '点数', '状态', '耗时ms', '首字ms']]
  for (const l of logs.value) {
    rows.push([l.at, l.model, l.prompt_tokens, l.completion_tokens, l.points, l.status, l.ms, l.ttft_ms])
  }
  const csv = rows.map(r => r.join(',')).join('\n')
  const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `wb2-usage-${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(a.href)
}

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / 30)))

const modelTotal = computed(() => {
  const m = usage.value?.by_model || []
  return m.reduce((a, b) => a + b.points, 0) || 1
})
</script>

<template>
  <section class="usage container">
    <header class="uhead">
      <div>
        <p class="eyebrow">USAGE</p>
        <h1>用量与账单</h1>
        <p class="sub" style="margin-top:8px">按模型、按天的点数消耗；日志保留期由档位决定。</p>
      </div>
      <div class="range">
        <button v-for="d in [7, 30, 90]" :key="d" class="rbtn" :data-on="days === d ? 1 : 0"
                @click="days = d">{{ d }} 天</button>
      </div>
    </header>

    <p v-if="err" class="errline">{{ err }}</p>

    <!-- 汇总 -->
    <div class="cards">
      <article class="c glass glass-1">
        <span>调用次数</span>
        <b>{{ fmt.n(usage?.total?.calls) }}</b>
        <i>{{ days }} 天累计</i>
      </article>
      <article class="c glass glass-1">
        <span>消耗点数</span>
        <b>{{ fmt.pts(usage?.total?.points) }}</b>
        <i>1 元 = 1000 点</i>
      </article>
      <article class="c glass glass-1">
        <span>Token 总量</span>
        <b>{{ fmt.n(usage?.total?.tokens) }}</b>
        <i>输入 + 输出</i>
      </article>
      <article class="c glass glass-1" v-if="me">
        <span>赠点余额</span>
        <b>{{ fmt.pts(me.quota?.lottery_points) }}</b>
        <i>抽奖所得 · 永久有效 · 只能消费</i>
      </article>
      <article class="c glass glass-1">
        <span>日均</span>
        <b>{{ fmt.pts((usage?.total?.points || 0) / days) }}</b>
        <i>点 / 天</i>
      </article>
    </div>

    <!-- 趋势 -->
    <article class="panel glass glass-1">
      <div class="phead">
        <div>
          <p class="eyebrow">TREND</p>
          <h3>每日点数</h3>
        </div>
        <button class="btn sm" @click="exportCsv">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 4v11M7.5 11.5L12 16l4.5-4.5" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M5 19h14" stroke-linecap="round"/>
          </svg>
          导出当前页 CSV
        </button>
      </div>

      <div class="chart-wrap" @pointermove="onMove" @pointerleave="hover = -1">
        <svg :viewBox="`0 0 ${VB_W} ${VB_H}`" preserveAspectRatio="none" class="chart">
          <defs>
            <linearGradient id="ug" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0" stop-color="#ff8a3d" stop-opacity=".32"/>
              <stop offset="1" stop-color="#ff8a3d" stop-opacity="0"/>
            </linearGradient>
          </defs>
          <g stroke="rgba(255,255,255,.05)" stroke-width="1">
            <line v-for="i in 3" :key="i" x1="0" :x2="VB_W"
                  :y1="VB_H - 22 - i * (VB_H - 40) / 3" :y2="VB_H - 22 - i * (VB_H - 40) / 3" />
          </g>
          <path v-if="areaPath" :d="areaPath" fill="url(#ug)" />
          <path v-if="linePath" :d="linePath" fill="none" stroke="#ff8a3d" stroke-width="2.2"
                stroke-linecap="round" stroke-linejoin="round"
                stroke-dasharray="1200" stroke-dashoffset="0" class="ul" />
          <g v-if="hover >= 0 && series[hover]">
            <line :x1="xOf(hover)" :x2="xOf(hover)" y1="6" :y2="VB_H - 22"
                  stroke="rgba(255,255,255,.2)" stroke-dasharray="3 3" />
            <circle :cx="xOf(hover)" :cy="yOf(series[hover].points)" r="4" fill="#0a0a0c"
                    stroke="#ff8a3d" stroke-width="2.2" />
          </g>
        </svg>
        <div v-if="hover >= 0 && series[hover]" class="tip glass glass-3"
             :style="{ left: (xOf(hover) / VB_W * 100) + '%', top: (yOf(series[hover].points) / VB_H * 100) + '%' }">
          <b>{{ series[hover].points.toFixed(1) }}</b><span>点</span>
          <i>{{ series[hover].day.slice(5) }}</i>
        </div>
        <p v-if="!series.length" class="nochart">这段时间还没有调用记录</p>
      </div>
    </article>

    <!-- 按模型 -->
    <article class="panel glass glass-1" v-if="usage?.by_model?.length">
      <div class="phead">
        <div>
          <p class="eyebrow">BY MODEL</p>
          <h3>模型占比</h3>
        </div>
      </div>
      <div class="models">
        <div v-for="(m, i) in usage.by_model" :key="m.model" class="mrow" :style="{ '--i': i }">
          <span class="mname mono trunc">{{ m.model }}</span>
          <div class="mbar">
            <i :style="{ width: (m.points / modelTotal * 100) + '%' }" />
          </div>
          <span class="mval">{{ fmt.pts(m.points) }} 点</span>
          <span class="mcalls">{{ fmt.n(m.calls) }} 次</span>
        </div>
      </div>
    </article>

    <!-- 日志 -->
    <article class="panel glass glass-1">
      <div class="phead">
        <div>
          <p class="eyebrow">LOGS</p>
          <h3>调用日志</h3>
        </div>
        <div class="filters">
          <button class="fbtn" :data-on="filter === '' ? 1 : 0" @click="filter = ''; page = 1">全部</button>
          <button class="fbtn" :data-on="filter === 'ok' ? 1 : 0" @click="filter = 'ok'; page = 1">成功</button>
          <button class="fbtn" :data-on="filter === 'error' ? 1 : 0" @click="filter = 'error'; page = 1">失败</button>
        </div>
      </div>

      <div class="table">
        <div class="tr th">
          <span>时间</span><span>模型</span><span class="num">输入</span><span class="num">输出</span>
          <span class="num">点数</span><span class="num">首字</span><span class="num">耗时</span><span>状态</span>
        </div>
        <div v-for="(l, i) in logs" :key="l.id" class="tr" :style="{ '--i': i }">
          <span class="mono dim tiny">{{ l.at.slice(5) }}</span>
          <span class="mono trunc">{{ l.model }}</span>
          <span class="num">{{ l.prompt_tokens }}</span>
          <span class="num">{{ l.completion_tokens }}</span>
          <span class="num ac">{{ l.points.toFixed(2) }}</span>
          <span class="num">{{ l.ttft_ms ? l.ttft_ms + 'ms' : '—' }}</span>
          <span class="num">{{ l.ms }}ms</span>
          <span>
            <i class="st" :class="l.status < 400 ? 'ok' : 'err'">{{ l.status }}</i>
          </span>
        </div>
        <p v-if="!logs.length && !loading" class="nologs">没有记录。</p>
      </div>

      <div class="pager" v-if="total > 30">
        <button class="btn sm ghost" :disabled="page <= 1" @click="page--">上一页</button>
        <span class="tiny dim">{{ page }} / {{ pageCount }} · 共 {{ total }} 条</span>
        <button class="btn sm ghost" :disabled="page >= pageCount" @click="page++">下一页</button>
      </div>
    </article>
  </section>
</template>

<style scoped>
.usage{padding-top:26px;display:flex;flex-direction:column;gap:13px}
.uhead{display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap;margin-bottom:2px}
.uhead h1{margin-top:6px}
.range{display:flex;gap:6px;padding:3px;border-radius:10px;
  background:rgba(255,255,255,.04);border:1px solid var(--line)}
.rbtn{height:28px;padding:0 12px;border-radius:7px;font-size:11.5px;font-weight:600;color:var(--tx-4);
  transition:color 220ms,background-color 220ms}
.rbtn[data-on="1"]{color:var(--tx-1);background:rgba(255,255,255,.09)}
.errline{font-size:12px;color:#f6b1ac;background:rgba(240,103,95,.10);
  border:1px solid rgba(240,103,95,.22);border-radius:9px;padding:8px 11px}

.cards{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}
@media (max-width:760px){ .cards{grid-template-columns:repeat(2,1fr)} }
.c{padding:14px 15px;display:flex;flex-direction:column;gap:3px;
  animation:k-fade-up 520ms var(--e-out) both}
.c span{font-size:11px;color:var(--tx-3)}
.c b{font-size:23px;font-weight:720;letter-spacing:-.035em;font-variant-numeric:tabular-nums}
.c i{font-style:normal;font-size:10px;color:var(--tx-4)}

.panel{padding:15px 17px 13px;display:flex;flex-direction:column;gap:11px}
.phead{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.phead h3{margin-top:3px}

.chart-wrap{position:relative}
.chart{width:100%;height:160px;display:block}
.ul{animation:ul-draw 1100ms var(--e-out) both}
@keyframes ul-draw{from{stroke-dashoffset:1200}to{stroke-dashoffset:0}}
.nochart{position:absolute;inset:0;display:grid;place-items:center;
  font-size:12px;color:var(--tx-4)}
.tip{position:absolute;transform:translate(-50%,-128%);padding:5px 8px;border-radius:8px;
  display:flex;align-items:baseline;gap:3px;font-size:11px;pointer-events:none;white-space:nowrap;
  animation:k-scale-in 160ms var(--e-out) both}
.tip b{font-size:13px;font-weight:700;font-variant-numeric:tabular-nums}
.tip span{font-size:9px;color:var(--tx-4)}
.tip i{font-style:normal;font-size:9px;color:var(--tx-4);margin-left:4px;
  padding-left:5px;border-left:1px solid var(--line)}

/* 模型占比 */
.models{display:flex;flex-direction:column;gap:7px}
.mrow{display:grid;grid-template-columns:130px 1fr 78px 62px;align-items:center;gap:10px;
  animation:k-fade-up 460ms var(--e-out) both;animation-delay:calc(var(--i,0) * 40ms)}
@media (max-width:600px){ .mrow{grid-template-columns:104px 1fr 66px;gap:8px} .mcalls{display:none} }
.mname{font-size:11px;color:var(--tx-2)}
.mbar{height:5px;border-radius:99px;background:rgba(255,255,255,.06);overflow:hidden}
.mbar i{display:block;height:100%;border-radius:99px;
  background:linear-gradient(90deg,rgba(255,138,61,.55),rgba(255,179,122,.95));
  animation:k-grow-h 700ms var(--e-out) both;transform-origin:left}
.mval{font-size:11px;color:var(--tx-2);text-align:right;font-variant-numeric:tabular-nums}
.mcalls{font-size:10px;color:var(--tx-4);text-align:right;font-variant-numeric:tabular-nums}

/* 表格 */
.filters{display:flex;gap:5px}
.fbtn{height:26px;padding:0 10px;border-radius:7px;font-size:11px;color:var(--tx-4);
  border:1px solid transparent;transition:all 200ms}
.fbtn[data-on="1"]{color:var(--tx-1);background:rgba(255,255,255,.07);border-color:var(--line)}
.table{display:flex;flex-direction:column}
.tr{
  display:grid;grid-template-columns:76px 1fr 52px 52px 56px 56px 62px 44px;
  align-items:center;gap:8px;min-height:31px;
  border-top:1px solid rgba(255,255,255,.045);font-size:11px;
  animation:k-fade-up 380ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 18ms);
}
.tr.th{border-top:0;color:var(--tx-4);font-size:10px;min-height:26px}
.tr.th span{font-weight:600}
.num{text-align:right;font-variant-numeric:tabular-nums}
.tr .ac{color:var(--ac-2)}
.st{font-style:normal;font-size:10px;padding:1.5px 5px;border-radius:4px;
  font-variant-numeric:tabular-nums}
.st.ok{color:#a9e8c5;background:rgba(84,201,138,.10)}
.st.err{color:#f6b1ac;background:rgba(240,103,95,.12)}
@media (max-width:640px){
  .tr{grid-template-columns:62px 1fr 44px 44px 48px 40px}
  .tr span:nth-child(6),.tr span:nth-child(7){display:none}
}
.nologs{text-align:center;padding:24px 0;color:var(--tx-4);font-size:12px}
.pager{display:flex;align-items:center;justify-content:center;gap:12px;padding-top:8px}
</style>

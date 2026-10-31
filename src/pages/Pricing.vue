<script setup>
import { ref, computed, onMounted } from 'vue'
import { spring } from '../lib/spring.js'
import { api, state } from '../lib/api.js'
import PayModal from '../components/PayModal.vue'
import { MATRIX, EXTRAS, FAQ, POINT_PER_YUAN } from '../lib/plans.js'

const SPONSOR_TIERS = ref([])
const CONTRIBUTOR_TIERS = ref([])
const POINT_PACKS = ref([])
const loadErr = ref('')

onMounted(async () => {
  try {
    const d = await api.plans()
    SPONSOR_TIERS.value = (d.sponsor || []).map(p => ({
      id: p.code, code: p.code.toUpperCase(), name: p.name, tagline: p.tagline,
      price: p.price, yearPrice: p.year_price, daily: p.daily,
      conc: p.conc, rpm: p.rpm, logDays: p.log_days,
      perks: p.perks || [], missing: [], hot: p.hot, tone: p.hot ? 'accent' : 'base',
    }))
    POINT_PACKS.value = (d.points || []).map(p => ({
      code: p.code, name: p.name, tagline: p.tagline,
      price: p.price, points: p.points || p.daily, hot: p.hot,
    }))
    CONTRIBUTOR_TIERS.value = (d.contrib || []).map(p => ({
      id: p.code, code: p.code.toUpperCase(), name: p.name, tagline: p.tagline,
      ratio: Math.round((p.ratio || 0) * 100), floorDaily: p.daily,
      conc: p.conc, rpm: p.rpm, perks: p.perks || [], need: p.need || [], hot: p.hot,
      tone: p.hot ? 'accent' : 'base',
    }))
  } catch (e) { loadErr.value = e.message }
})

const mode = ref('sponsor')          // sponsor | contributor
const yearly = ref(false)
const cards = computed(() => (mode.value === 'sponsor' ? SPONSOR_TIERS.value : CONTRIBUTOR_TIERS.value))

/* 切换时给卡片重播入场：改 key */
const rev = ref(0)
function setMode(m) {
  if (mode.value === m) return
  mode.value = m
  rev.value++
}

/* ---------------- 价格数字滚动 ---------------- */
const shown = ref({})
onMounted(() => {
  SPONSOR_TIERS.value.forEach((t, i) => {
    shown.value[t.id] = 0
    setTimeout(() => {
      spring(0, t.price, v => { shown.value[t.id] = v }, {
        stiffness: 210, damping: 24,
        onComplete: () => { shown.value[t.id] = t.price },
      })
    }, 220 + i * 90)
    // 兜底：rAF 被节流（后台标签/无头环境）时也要落到准确数字
    setTimeout(() => { shown.value[t.id] = t.price }, 1700 + i * 90)
  })
})
const priceOf = t => (yearly.value ? t.yearPrice : t.price)
const priceShown = t => (t.id in shown.value && mode.value === 'sponsor' && !yearly.value
  ? Math.round(shown.value[t.id])
  : priceOf(t))

/* ---------------- 对比矩阵（只对比赞助三档） ---------------- */
const cellText = v => (v === true ? '✓' : v === false ? '—' : v)

/* ---------------- 下单 / 跳转 ---------------- */
const toast = ref('')
let toastTimer = null
function flash(t, ms = 2600) {
  toast.value = t
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.value = '' }, ms)
}

const payTier = ref(null)
function act(t) {
  if (mode.value === 'contributor') {
    window.__goto && window.__goto('accounts')
    return
  }
  // 未登录 → 打开登录
  if (!state.token) {
    window.dispatchEvent(new CustomEvent('wb2-need-login'))
    flash('请先登录再赞助')
    return
  }
  payTier.value = t
}

function buyPack(p) {
  if (!state.token) {
    window.dispatchEvent(new CustomEvent('wb2-need-login'))
    flash('请先登录再购买')
    return
  }
  payTier.value = { id: p.code, code: p.code.toUpperCase(), name: p.name, price: p.price, pack: true }
}

async function onPaid(st) {
  // 付款成功后把「实际生效的额度」直接说清楚，避免用户看到剩余 0 以为没到账
  let msg = '已到账，档位已开通'
  try {
    const d = await api.me()
    const q = d.quota || {}
    const num = (v, f) => Math.round(v == null ? (f || 0) : v)
    const daily = num(q.daily, d.daily_points)
    const used = num(q.used_today, d.points_today)
    const left = num(q.left_today, d.points_left)
    const bonus = num(q.bonus, d.bonus_points)
    const plan = String(d.plan || '').toUpperCase()
    msg = `${plan ? plan + ' 已开通 · ' : ''}今日额度 ${daily} 点，已用 ${used}，剩余 ${left}`
    if (bonus > 0) msg += `（加油包 ${bonus} 点）`
    if (left <= 0) msg += ' · 今日额度已用完，明天 0 点回满'
  } catch (_) {}
  flash(msg, 5200)
  // 通知 App 刷新用户信息（额度/档位变了）
  window.dispatchEvent(new CustomEvent('wb2-refresh-me'))
  setTimeout(() => { payTier.value = null }, 1200)
}

/* ---------------- FAQ ---------------- */
const openFaq = ref(0)
function toggleFaq(i) { openFaq.value = openFaq.value === i ? -1 : i }

/* ---------------- 图标 ---------------- */
const ICONS = {
  school: '<path d="M12 4l9 4.2-9 4.2L3 8.2 12 4z"/><path d="M6.5 10.4v4.4c0 1.6 2.5 2.9 5.5 2.9s5.5-1.3 5.5-2.9v-4.4"/>',
  card: '<rect x="3.2" y="5.6" width="17.6" height="12.8" rx="2.6"/><path d="M3.4 10h17.2"/><path d="M7 14.4h4"/>',
  invite: '<circle cx="9" cy="8.6" r="3"/><path d="M4 19.4c0-2.8 2.2-4.8 5-4.8s5 2 5 4.8"/><path d="M17.4 8.2v5M19.9 10.7h-5"/>',
  shield: '<path d="M12 3.8l7 2.6v5.2c0 4.2-2.9 7.2-7 8.6-4.1-1.4-7-4.4-7-8.6V6.4l7-2.6z"/><path d="M9 12l2.2 2.2L15.4 10"/>',
}
</script>

<template>
  <section class="pricing container">
    <!-- ============ 头 ============ -->
    <header class="phead3">
      <p class="eyebrow">SUSTAIN</p>
      <h1>两条路，通向同一个池子</h1>
      <p class="lead2">
        这里没有「充值套餐」，只有两种参与方式：<b>交账号</b>贡献真实算力，或者<b>赞助</b>分摊成本。
        两条路都能用，也可以叠加。
      </p>

      <p v-if="loadErr" class="loaderr">档位加载失败：{{ loadErr }}</p>

      <!-- 模式切换 -->
      <div class="modes">
        <div class="seg2">
          <span class="seg2-ink" :style="{ transform: `translateX(${mode === 'sponsor' ? 0 : 100}%)` }" />
          <button :data-on="mode === 'sponsor' ? 1 : 0" @click="setMode('sponsor')">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 3.6v16.8M16.4 7.2H10a2.6 2.6 0 000 5.2h4a2.6 2.6 0 010 5.2H7.6"/>
            </svg>
            赞助
          </button>
          <button :data-on="mode === 'contributor' ? 1 : 0" @click="setMode('contributor')">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="9" cy="8.6" r="3"/><path d="M4 19.4c0-2.8 2.2-4.8 5-4.8s5 2 5 4.8"/>
              <path d="M17.4 8.2v5M19.9 10.7h-5"/>
            </svg>
            授权账号
          </button>
        </div>

        <Transition name="fy">
          <div v-if="mode === 'sponsor'" class="yearly">
            <button class="ychip" :data-on="!yearly ? 1 : 0" @click="yearly = false">月付</button>
            <button class="ychip" :data-on="yearly ? 1 : 0" @click="yearly = true">
              年付 <i>省 2 个月</i>
            </button>
          </div>
        </Transition>
      </div>
    </header>

    <!-- ============ 档位卡 ============ -->
    <div class="tiers" :key="rev + '-' + (yearly ? 'y' : 'm')">
      <article v-for="(t, i) in cards" :key="t.id" class="tier glass"
               :class="[t.tone, { hot: t.hot }]" :style="{ '--i': i }">
        <span v-if="t.hot" class="hot-tag">最多人选</span>

        <div class="t-head">
          <span class="t-code mono">{{ t.code }}</span>
          <h3>{{ t.name }}</h3>
          <p class="t-tag">{{ t.tagline }}</p>
        </div>

        <!-- 价格 -->
        <div v-if="mode === 'sponsor'" class="t-price">
          <span class="cur">¥</span>
          <b class="num">{{ priceShown(t) }}</b>
          <span class="per">/ {{ yearly ? '年' : '月' }}</span>
        </div>
        <div v-else class="t-price contrib">
          <b class="num">{{ t.ratio }}<span class="pct">%</span></b>
          <span class="per">账号余额折算</span>
        </div>

        <p class="t-mini">
          <template v-if="mode === 'sponsor'">
            约 {{ (t.daily / 4).toLocaleString() }} 次对话 / 日 · {{ t.daily.toLocaleString() }} 点
          </template>
          <template v-else>
            保底 {{ (t.floorDaily / 4).toLocaleString() }} 次 / 日 · 并发 {{ t.conc }}
          </template>
        </p>

        <ul class="t-perks">
          <li v-for="p in t.perks" :key="p">
            <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor"
                 stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
              <path d="M5 12.6l4.3 4.2L19 6.9"/>
            </svg>
            {{ p }}
          </li>
        </ul>

        <ul v-if="t.missing && t.missing.length" class="t-miss">
          <li v-for="m in t.missing" :key="m">— {{ m }}</li>
        </ul>

        <!-- 门槛（贡献模式） -->
        <div v-if="mode === 'contributor'" class="t-need">
          <span class="tiny dim">准入</span>
          <span v-for="n in t.need" :key="n" class="need-chip">{{ n }}</span>
        </div>

        <button class="btn t-cta" :class="t.hot ? 'pri' : ''" @click="act(t)">
          {{ mode === 'sponsor' ? `赞助 · ¥${yearly ? t.yearPrice : t.price}` : '提交账号' }}
        </button>
      </article>
    </div>

    <p class="helper">
      <template v-if="mode === 'sponsor'">
        1 元 = {{ POINT_PER_YUAN }} 点。额度按日恢复，不做清零惩罚；用超只降速到 20%，第二天 0 点回满。
      </template>
      <template v-else>
        折算每月 1 日结算，账号可随时撤回；掉线超过 24 小时折算暂停，保底降为最低档。
      </template>
    </p>

    <!-- ============ 加油包 ============ -->
    <article v-if="mode === 'sponsor' && POINT_PACKS.length" class="packs glass glass-1">
      <div class="m-head">
        <p class="eyebrow">TOP-UP</p>
        <h3 class="pk-h3">加油包 · 随时补点，不占日额度</h3>
      </div>
      <div class="pk-grid">
        <div v-for="p in POINT_PACKS" :key="p.code" class="pk" :class="{ hot: p.hot }">
          <div class="pk-top">
            <b>{{ p.name }}</b>
            <span v-if="p.hot" class="pk-hot">划算</span>
          </div>
          <p class="tiny dim">{{ p.tagline }}</p>
          <div class="pk-num">{{ p.points }} 点</div>
          <button class="btn" :class="p.hot ? 'pri' : ''" @click="buyPack(p)">¥{{ p.price }} 购买</button>
        </div>
      </div>
      <p class="helper">
        日额度用完后优先扣加油包，用到 1.5 倍才拦；加油包 30 天有效，不占每日额度，也不会清零日额度。
      </p>
    </article>

    <!-- ============ 每天能调多少次 ============ -->
    <article class="glass glass-1 qtimes">
      <div class="m-head">
        <p class="eyebrow">HOW MANY</p>
        <h3 class="pk-h3">每天到底能调多少次？</h3>
      </div>
      <div class="q-table">
        <div class="q-row q-title">
          <span>使用场景</span><span>每次消耗</span><span>新手前 7 天</span><span>7 天后（未付费）</span>
        </div>
        <div class="q-row"><span>短问短答<br><i>300 / 300 tok</i></span><b>3 点</b><span>约 2666 次</span><span class="q-dim">约 66 次</span></div>
        <div class="q-row"><span>日常对话<br><i>1000 / 500 tok</i></span><b>6 点</b><span>约 1333 次</span><span class="q-dim">约 33 次</span></div>
        <div class="q-row"><span>贴代码 / 长文<br><i>8000 / 1500 tok</i></span><b>28 点</b><span>约 285 次</span><span class="q-dim">约 7 次</span></div>
      </div>
      <p class="helper">
        计费 = (输入 tok × 2 + 输出 tok × 8) ÷ 1000 × 模型倍率（glm-5.3-flash / deepseek-v4.1-flash 倍率 1.0）。
        <b>注册前 7 天每天 8000 点 + 一份 3000 点加油包</b>；7 天后不付费每天只剩 200 点，够试不够干活。想长期用就买加油包或赞助档位。
      </p>
    </article>

    <!-- ============ 权益矩阵 ============ -->
    <article class="matrix glass glass-1">
      <div class="m-head">
        <p class="eyebrow">COMPARE</p>
        <h3>赞助档位对照</h3>
      </div>
      <div class="m-table no-sb">
        <div class="m-row m-title">
          <span class="m-k">权益</span>
          <span v-for="t in SPONSOR_TIERS" :key="t.id" class="m-v">{{ t.code }}</span>
        </div>
        <div v-for="(row, i) in MATRIX" :key="row.k" class="m-row" :style="{ '--i': i }">
          <span class="m-k">{{ row.k }}</span>
          <span v-for="(v, j) in row.v" :key="j" class="m-v"
                :class="{ yes: v === true, no: v === false, hi: j === 1 }">
            {{ cellText(v) }}
          </span>
        </div>
      </div>
    </article>

    <!-- ============ 附加规则 ============ -->
    <div class="extras">
      <article v-for="(e, i) in EXTRAS" :key="e.title" class="extra glass glass-1"
               :class="{ danger: e.danger }" :style="{ '--i': i }">
        <div class="e-top">
          <span class="e-ico">
            <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor"
                 stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" v-html="ICONS[e.icon]" />
          </span>
          <h3>{{ e.title }}</h3>
          <span class="pill" :class="e.danger ? 'err' : ''">{{ e.pill }}</span>
        </div>
        <p>{{ e.desc }}</p>
      </article>
    </div>

    <!-- ============ 订阅叠加说明 ============ -->
    <article class="stack glass glass-2">
      <div class="st-l">
        <p class="eyebrow">STACK</p>
        <h3>两条路可以叠加</h3>
        <p class="sub">
          授权了账号、又赞助了档位的人，额度按 <b>1.5 倍</b> 计。
          因为这类人既出了算力又出了钱，池子最稳的就是他们。
        </p>
      </div>
      <div class="st-r">
        <div class="st-eq">
          <span class="st-chip">授权 C2</span>
          <i>＋</i>
          <span class="st-chip ac">赞助 LV2</span>
          <i>=</i>
          <span class="st-out">6 万 × 1.5 = <b>9 万</b>点/日</span>
        </div>
      </div>
    </article>

    <!-- ============ FAQ ============ -->
    <div class="faq-wrap">
      <div class="faq-head">
        <p class="eyebrow">FAQ</p>
        <h2>先把话说在前面</h2>
      </div>
      <div class="faq">
        <div v-for="(f, i) in FAQ" :key="f.q" class="faq-it" :class="{ open: openFaq === i }">
          <button class="faq-q" @click="toggleFaq(i)">
            <span>{{ f.q }}</span>
            <i class="chev">
              <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor"
                   stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M6.5 9.5L12 15l5.5-5.5"/>
              </svg>
            </i>
          </button>
          <div class="faq-a">
            <div class="faq-in"><p>{{ f.a }}</p></div>
          </div>
        </div>
      </div>
    </div>
    <Transition name="toast">
      <div v-if="toast" class="flashtoast glass glass-3">{{ toast }}</div>
    </Transition>

    <PayModal v-if="payTier" :tier="payTier" :yearly="yearly"
              @close="payTier = null" @paid="onPaid" />
  </section>
</template>

<style scoped>
.pricing{padding-top:26px;display:flex;flex-direction:column;gap:18px}

/* ---------- 头 ---------- */
.loaderr{font-size:12px;color:#f6b1ac;margin-top:10px}
.phead3{max-width:66ch}
.phead3 h1{margin:7px 0 12px;letter-spacing:-.036em;
  animation:k-fade-up 620ms var(--e-out) both}
.lead2{color:var(--tx-3);font-size:14.5px;line-height:1.78;
  animation:k-fade-up 620ms 70ms var(--e-out) both}
.lead2 b{color:var(--tx-1);font-weight:640}

.modes{display:flex;align-items:center;gap:12px;margin-top:22px;flex-wrap:wrap;
  animation:k-fade-up 620ms 140ms var(--e-out) both}
.seg2{
  position:relative;display:flex;padding:3px;border-radius:12px;
  background:rgba(255,255,255,.045);border:1px solid var(--line);
}
.seg2-ink{
  position:absolute;top:3px;left:3px;height:calc(100% - 6px);width:calc((100% - 6px) / 2);
  border-radius:9px;
  background:linear-gradient(160deg,rgba(255,255,255,.16),rgba(255,255,255,.07));
  border:1px solid rgba(255,255,255,.14);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.18),0 6px 16px -12px rgba(0,0,0,.9);
  transition:transform 440ms var(--e-out);
}
.seg2 button{
  position:relative;z-index:1;display:flex;align-items:center;justify-content:center;gap:6px;
  height:34px;padding:0 16px;border-radius:9px;flex:1;white-space:nowrap;
  font-size:12.5px;font-weight:620;color:var(--tx-3);
  transition:color 240ms var(--e-out-soft);
}
.seg2 button[data-on="1"]{color:var(--tx-1)}

.yearly{display:flex;gap:6px}
.ychip{
  height:30px;padding:0 11px;border-radius:9px;
  border:1px solid var(--line);background:rgba(255,255,255,.03);
  font-size:11.5px;font-weight:600;color:var(--tx-4);
  display:inline-flex;align-items:center;gap:6px;
  transition:transform 240ms var(--e-out),color 200ms,border-color 200ms,background-color 200ms;
}
.ychip:active{transform:scale(.96)}
.ychip i{font-style:normal;font-size:9px;color:var(--ac-2);
  padding:1px 5px;border-radius:4px;background:var(--ac-dim)}
.ychip[data-on="1"]{color:var(--tx-1);border-color:var(--line-2);background:rgba(255,255,255,.075)}
.fy-enter-active{animation:k-fade-up 320ms var(--e-out) both}
.fy-leave-active{animation:k-fade-in 140ms reverse both}

/* ---------- 档位卡 ---------- */
.tiers{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;align-items:stretch}
@media (max-width:900px){ .tiers{grid-template-columns:1fr} }

.tier{
  position:relative;padding:20px 18px 16px;display:flex;flex-direction:column;gap:11px;
  border-radius:var(--r-lg);
  animation:k-fade-up 560ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 80ms);
  transition:transform 340ms var(--e-out),border-color 300ms,box-shadow 340ms;
}
.tier:hover{transform:translateY(-3px)}
.tier.hot{
  border-color:var(--ac-line);
  background:
    linear-gradient(155deg,rgba(255,138,61,.12),rgba(255,138,61,.035) 42%,rgba(255,255,255,.055)),
    rgba(255,255,255,.055);
  box-shadow:
    inset 0 1px 0 rgba(255,220,190,.20),
    inset 0 -1px 0 rgba(255,255,255,.04),
    0 22px 46px -26px rgba(255,138,61,.30),
    var(--sh-lg);
}
.hot-tag{
  position:absolute;top:-9px;left:18px;
  font-size:9.5px;font-weight:700;letter-spacing:.04em;
  padding:3px 9px;border-radius:99px;
  color:#2a1408;background:linear-gradient(180deg,#ffb37a,#ff8a3d);
  box-shadow:0 4px 14px -4px rgba(255,138,61,.6);
}

.t-head{display:flex;flex-direction:column;gap:4px}
.t-code{
  font-size:9.5px;font-weight:700;letter-spacing:.10em;color:var(--tx-4);
  align-self:flex-start;padding:2px 6px;border-radius:5px;
  background:rgba(255,255,255,.05);border:1px solid var(--line);
}
.tier.hot .t-code{color:#ffcba6;background:var(--ac-dim);border-color:var(--ac-line)}
.t-head h3{font-size:16px;font-weight:660;letter-spacing:-.02em}
.t-tag{font-size:11px;color:var(--tx-4);line-height:1.45;min-height:32px}

.t-price{display:flex;align-items:baseline;gap:2px;margin-top:-2px}
.t-price .cur{font-size:15px;font-weight:600;color:var(--tx-3);margin-right:1px}
.t-price .num{
  font-size:38px;font-weight:720;letter-spacing:-.045em;line-height:1;
  font-variant-numeric:tabular-nums;
}
.t-price .per{font-size:11px;color:var(--tx-4);margin-left:4px}
.t-price.contrib .num{font-size:34px;color:var(--ac-2)}
.t-price.contrib .pct{font-size:16px;margin-left:1px}
.t-mini{font-size:10.5px;color:var(--tx-4);font-variant-numeric:tabular-nums}

.t-perks{display:flex;flex-direction:column;gap:7px;margin-top:2px}
.t-perks li{
  display:flex;align-items:flex-start;gap:8px;
  font-size:12px;line-height:1.55;color:var(--tx-2);
}
.t-perks svg{flex:none;margin-top:4px;color:var(--ok)}
.tier.hot .t-perks svg{color:var(--ac-2)}

.t-miss{display:flex;flex-direction:column;gap:4px;padding-top:7px;
  border-top:1px dashed rgba(255,255,255,.075)}
.t-miss li{font-size:10.5px;color:var(--tx-4);line-height:1.4}

.t-need{display:flex;flex-wrap:wrap;align-items:center;gap:5px;padding-top:8px;
  border-top:1px solid var(--line)}
.need-chip{
  font-size:9.5px;color:var(--tx-3);padding:2.5px 7px;border-radius:99px;
  background:rgba(255,255,255,.05);border:1px solid var(--line);
}

.t-cta{margin-top:auto;height:40px;width:100%}

.helper{font-size:11.5px;color:var(--tx-4);line-height:1.6;text-align:center;margin-top:-4px}

/* ---------- 矩阵 ---------- */
.matrix{padding:16px 18px 14px;display:flex;flex-direction:column;gap:11px}
/* ---------- 加油包 ---------- */
.packs{padding:20px}
.pk-h3{margin:2px 0 0;font-size:15px;font-weight:640;letter-spacing:-.01em}

/* ---------- 每天能调多少次 ---------- */
.qtimes{padding:20px}
.q-table{margin-top:14px;display:flex;flex-direction:column}
.q-row{display:grid;grid-template-columns:1.5fr .8fr 1fr 1.3fr;gap:8px;align-items:center;
  padding:10px 0;border-bottom:1px solid var(--line);font-size:12.5px;color:var(--tx-2)}
.q-row:last-child{border-bottom:0}
.q-row span{min-width:0}
.q-row i{font-size:10.5px;color:var(--tx-4);font-style:normal}
.q-row b{color:var(--ac-1);font-weight:640}
.q-title{font-size:10px;letter-spacing:.08em;color:var(--tx-4);font-weight:700;border-bottom-color:var(--line-2)}
.q-dim{color:var(--tx-4)}
.qtimes .helper{margin-top:12px;line-height:1.7}
@media (max-width:640px){
  .q-row{grid-template-columns:1.4fr .7fr 1fr 1.1fr;gap:6px;font-size:11.5px}
  .q-row i{display:none}
}
.pk-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-top:14px}
@media (max-width:820px){ .pk-grid{grid-template-columns:1fr} }
.pk{padding:14px 16px;border-radius:14px;border:1px solid var(--line);
  background:rgba(255,255,255,.025);display:flex;flex-direction:column;gap:7px;min-width:0}
.pk.hot{border-color:var(--ac-line);background:rgba(255,138,61,.08)}
.pk-top{display:flex;align-items:center;justify-content:space-between;gap:8px}
.pk-top b{font-size:13.5px;font-weight:640;color:var(--tx-1)}
.pk-hot{flex:none;font-size:10px;padding:2px 7px;border-radius:99px;
  background:var(--ac-dim);color:#ffcba6;border:1px solid var(--ac-line);font-weight:600}
.pk p{margin:0;line-height:1.5;font-size:11.5px}
.pk-num{font:700 20px/1.25 var(--sf);color:var(--tx-1);letter-spacing:-.02em}
.pk .btn{margin-top:2px;align-self:flex-start;font-size:12px;height:32px;padding:0 12px}
.packs .helper{margin-top:12px}
.m-head{display:flex;flex-direction:column;gap:3px}
.m-table{display:flex;flex-direction:column}
.m-row{
  display:grid;grid-template-columns:1.5fr repeat(3,1fr);
  min-height:34px;align-items:center;
  border-top:1px solid rgba(255,255,255,.05);
  animation:k-fade-up 420ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 26ms + 80ms);
}
.m-row:first-child{border-top:0}
.m-title{animation:none}
.m-title .m-k,.m-title .m-v{font-size:10px;font-weight:700;letter-spacing:.10em;
  text-transform:uppercase;color:var(--tx-4)}
.m-k{font-size:12px;color:var(--tx-3);padding-right:8px}
.m-v{font-size:12px;color:var(--tx-2);text-align:center;font-variant-numeric:tabular-nums}
.m-v.yes{color:#8fe8b6;font-size:13px}
.m-v.no{color:var(--tx-4)}
.m-v.hi{background:rgba(255,138,61,.055)}

@media (max-width:600px){
  .m-row{grid-template-columns:1.35fr repeat(3,.8fr)}
  .m-k{font-size:11px}
  .m-v{font-size:11px}
}

/* ---------- 附加 ---------- */
.extras{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:10px}
.extra{
  padding:14px 15px;display:flex;flex-direction:column;gap:7px;
  animation:k-fade-up 520ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 70ms + 60ms);
  transition:transform 300ms var(--e-out),border-color 260ms;
}
.extra:hover{transform:translateY(-2px);border-color:rgba(255,255,255,.14)}
.e-top{display:flex;align-items:center;gap:8px}
.e-ico{
  width:26px;height:26px;flex:none;border-radius:8px;display:grid;place-items:center;
  color:var(--ac-2);background:var(--ac-dim);border:1px solid rgba(255,138,61,.20);
}
.e-top h3{font-size:12.8px;font-weight:640;flex:1;min-width:0}
.e-top .pill{font-size:9px;height:19px;padding:0 6px;flex:none}
.extra p{font-size:11.5px;line-height:1.62;color:var(--tx-3)}
.extra.danger .e-ico{color:#f6b1ac;background:rgba(240,103,95,.10);border-color:rgba(240,103,95,.24)}

/* ---------- 叠加 ---------- */
.stack{padding:18px 20px;display:flex;align-items:center;gap:22px;flex-wrap:wrap}
.st-l{min-width:0;flex:1}
.st-l h3{margin:4px 0 6px}
.st-r{flex:none}
.st-eq{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.st-eq i{font-style:normal;color:var(--tx-4);font-size:13px}
.st-chip{
  font-size:11px;font-weight:640;padding:5px 10px;border-radius:8px;
  background:rgba(255,255,255,.06);border:1px solid var(--line-2);color:var(--tx-2);
}
.st-chip.ac{color:#ffcba6;background:var(--ac-dim);border-color:var(--ac-line)}
.st-out{font-size:12px;color:var(--tx-2);font-variant-numeric:tabular-nums}
.st-out b{font-size:15px;font-weight:700;color:var(--ac-2);letter-spacing:-.02em}

/* ---------- FAQ ---------- */
.flashtoast{position:fixed;left:50%;bottom:26px;transform:translateX(-50%);
  padding:9px 15px;border-radius:11px;font-size:12px;z-index:120;white-space:nowrap}
.toast-enter-active{animation:k-toast-in 380ms var(--e-out) both}
.toast-leave-active{animation:k-toast-out 220ms var(--e-in-out) both}
.faq-wrap{display:flex;flex-direction:column;gap:14px;margin-top:4px}
.faq-head h2{margin-top:5px}
.faq{display:flex;flex-direction:column;gap:7px}
.faq-it{
  border-radius:13px;border:1px solid var(--line);
  background:rgba(255,255,255,.028);
  overflow:hidden;
  transition:border-color 260ms,background-color 260ms;
}
.faq-it.open{border-color:rgba(255,255,255,.14);background:rgba(255,255,255,.05)}
.faq-q{
  display:flex;align-items:center;justify-content:space-between;gap:14px;
  width:100%;padding:13px 15px;text-align:left;
  font-size:13px;font-weight:600;color:var(--tx-2);
  transition:color 220ms;
}
.faq-it.open .faq-q{color:var(--tx-1)}
.chev{display:grid;place-items:center;color:var(--tx-4);flex:none;
  transition:transform 340ms var(--e-out),color 220ms}
.faq-it.open .chev{transform:rotate(180deg);color:var(--ac-2)}
.faq-a{display:grid;grid-template-rows:0fr;opacity:0;
  transition:grid-template-rows 400ms var(--e-out),opacity 280ms var(--e-out)}
.faq-it.open .faq-a{grid-template-rows:1fr;opacity:1}
.faq-in{overflow:hidden}
.faq-in p{
  padding:0 15px 14px;font-size:12px;line-height:1.78;color:var(--tx-3);
}
</style>

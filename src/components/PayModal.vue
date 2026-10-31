<script setup>
import { ref, computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { api } from '../lib/api.js'

const props = defineProps({
  tier: Object,        // 当前档位
  yearly: Boolean,
})
const emit = defineEmits(['close', 'paid'])

const step = ref('method')   // method | paying | done | manual
const method = ref('wechat')
const channels = ref([])
const loading = ref(false)
const err = ref('')
const pay = ref(null)
const order = ref(null)
const poll = ref(0)
let timer = null
let pollTimer = null

const months = computed(() => (props.yearly ? 12 : 1))
const amount = computed(() => {
  if (!props.tier) return 0
  const t = props.tier
  const y = t.yearPrice != null ? t.yearPrice : t.year_price
  const m = t.price
  return props.yearly ? (y || 0) : (m || 0)
})
/* 后端档位码是小写；UI 展示用大写。旧代码直接把大写 LV1 发给后端 → 400 档位不存在 */
const planCode = computed(() => {
  const t = props.tier || {}
  return String(t.id || t.code || '').toLowerCase()
})

const ALL = [
  { code: 'wechat', name: '微信支付', tone: 'wechat', icon: 'wechat' },
  { code: 'alipay', name: '支付宝', tone: 'alipay', icon: 'alipay' },
  { code: 'alipay2', name: '支付宝（备用通道）', tone: 'alipay', icon: 'alipay' },
  { code: 'manual', name: '手动转账 / 联系管理员', tone: 'manual', icon: 'manual' },
]

const usable = computed(() => {
  const enabled = channels.value.map(c => c.code)
  return ALL.filter(a => a.code === 'manual' || enabled.includes(a.code))
})

onMounted(async () => {
  lockScroll()
  try {
    const d = await api.channels()
    channels.value = d.channels || []
    const first = d.channels?.[0]?.code
    if (first && !usable.value.find(u => u.code === method.value)) method.value = first
  } catch (_) {}
})

/* 打开时锁住底层滚动（移动端尤其重要，否则弹窗背后页面会跟着滑） */
function lockScroll() {
  const b = document.body
  const prev = b.style.overflow
  b.dataset.pmPrevOverflow = prev
  b.style.overflow = 'hidden'
}
function unlockScroll() {
  const b = document.body
  b.style.overflow = b.dataset.pmPrevOverflow || ''
  delete b.dataset.pmPrevOverflow
}

async function go() {
  if (loading.value) return
  loading.value = true
  err.value = ''
  try {
    const d = await api.createOrder(planCode.value, months.value, method.value, '')
    order.value = d.order
    if (d.pay && d.pay.pay_url) {
      pay.value = d.pay
      step.value = 'paying'
      startPoll()
    } else {
      step.value = 'manual'
    }
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

function startPoll() {
  stopPoll()
  poll.value = 0
  pollTimer = setInterval(async () => {
    if (!order.value) return
    poll.value++
    try {
      const st = await api.orderStatus(order.value.order_no)
      if (st.status === 'paid') {
        stopPoll()
        step.value = 'done'
        emit('paid', st)
      }
    } catch (_) {}
    if (poll.value > 100) stopPoll()  // 5 分钟
  }, 3000)
}
function stopPoll() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}
onBeforeUnmount(() => { stopPoll(); unlockScroll() })

function openPay() {
  if (pay.value?.pay_url) window.open(pay.value.pay_url, '_blank', 'noopener')
}

const qrSrc = computed(() => {
  if (!pay.value?.qrcode) return ''
  const q = pay.value.qrcode
  if (q.startsWith('http') || q.startsWith('data:')) return q
  return ''
})

const copied = ref(false)
async function copyNo() {
  try { await navigator.clipboard.writeText(order.value.order_no) } catch (_) {}
  copied.value = true
  setTimeout(() => (copied.value = false), 1500)
}

const methodMeta = code => ALL.find(a => a.code === code) || { name: code }
</script>

<template>
  <Teleport to="body">
    <div class="pm-scrim" @click.self="emit('close')">
      <div class="pm glass glass-3" role="dialog" aria-label="支付">
      <!-- 头 -->
      <div class="pm-head">
        <div>
          <p class="eyebrow">{{ step === 'done' ? 'PAID' : 'CHECKOUT' }}</p>
          <h3 v-if="step === 'done'">已到账</h3>
          <h3 v-else-if="step === 'paying'">等待支付</h3>
          <h3 v-else-if="step === 'manual'">订单已创建</h3>
          <h3 v-else>选择支付方式</h3>
        </div>
        <button class="pm-x" @click="emit('close')" aria-label="关闭">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
            <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>
          </svg>
        </button>
      </div>

      <!-- 订单摘要 -->
      <div v-if="step !== 'done'" class="pm-sum">
        <div class="sum-l">
          <b>{{ tier.code.toUpperCase() }} · {{ tier.name }}</b>
          <span class="tiny dim">{{ months }} 个月{{ yearly ? '（年付）' : '' }}</span>
        </div>
        <div class="sum-r">
          <span class="cur">¥</span><b>{{ amount }}</b>
        </div>
      </div>

      <!-- 方式选择 -->
      <div v-if="step === 'method'" class="pm-body">
        <div class="methods">
          <button v-for="m in usable" :key="m.code" class="mth" :class="[m.tone, { on: method === m.code }]"
                  @click="method = m.code">
            <span class="mico" :class="m.icon">
              <svg v-if="m.icon === 'wechat'" viewBox="0 0 24 24" width="17" height="17" fill="currentColor">
                <path d="M9.2 4.2c-3.4 0-6.1 2.3-6.1 5.1 0 1.6.9 3 2.3 4l-.6 1.9 2.2-1.1c.7.2 1.4.3 2.2.3h.4a5.7 5.7 0 01-.2-1.5c0-3 2.9-5.4 6.5-5.4h.5C15.7 5.7 12.8 4.2 9.2 4.2zm-2.3 3.3a.9.9 0 110 1.8.9.9 0 010-1.8zm4.7 0a.9.9 0 110 1.8.9.9 0 010-1.8z"/>
                <path d="M21.6 12.4c0-2.4-2.3-4.3-5.1-4.3s-5.2 1.9-5.2 4.3c0 2.5 2.4 4.4 5.2 4.4.6 0 1.2-.1 1.8-.3l1.9.9-.5-1.6c1.2-.8 1.9-2 1.9-3.4zm-6.8-1.1a.75.75 0 110 1.5.75.75 0 010-1.5zm3.5 0a.75.75 0 110 1.5.75.75 0 010-1.5z"/>
              </svg>
              <svg v-else-if="m.icon === 'alipay'" viewBox="0 0 24 24" width="17" height="17" fill="currentColor">
                <path d="M4.4 5.2h15.2c.7 0 1.2.5 1.2 1.2v11.2c0 .7-.5 1.2-1.2 1.2H4.4c-.7 0-1.2-.5-1.2-1.2V6.4c0-.7.5-1.2 1.2-1.2zm2 2.6v1.4h4.2v-.9h1.5v.9h4v1.3h-3.5c.2.5.4.9.7 1.3 1.1-.3 2-.7 2.6-1.1v1.4c-.6.3-1.3.6-2.1.9.8 1.2 2 2.2 3.6 2.8v1.4c-1.9-.4-3.4-1.2-4.6-2.3-1.2 1.1-2.9 1.9-4.8 2.3v-1.4c1.7-.5 3-1.4 3.8-2.6l-.6-.5c-.7.9-1.7 1.6-3 2v-1.4c1.9-.6 3.1-1.8 3.6-3.2H6.4V9h4.2v-.9H8.9v-.3z"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor"
                   stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 3.6v16.8M16.4 7.2H10a2.6 2.6 0 000 5.2h4a2.6 2.6 0 010 5.2H7.6"/>
              </svg>
            </span>
            <span class="mtxt">{{ m.name }}</span>
            <span class="mck">
              <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor"
                   stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                <path d="M5 12.6l4.3 4.2L19 6.9"/>
              </svg>
            </span>
          </button>
        </div>

        <p class="tiny dim note">
          <template v-if="method === 'manual'">
            下单后请联系管理员确认收款，确认后档位立即生效。
          </template>
          <template v-else>
            支付由第三方支付平台处理，本站不保存任何银行卡信息。到账后自动开通。
          </template>
        </p>

        <p v-if="err" class="errline">{{ err }}</p>

        <button class="btn pri lg go" :disabled="loading || !(amount > 0)" @click="go">
          <span v-if="loading" class="spin" />
          {{ loading ? '创建订单…' : (method === 'manual' ? '创建订单' : `支付 ¥${amount}`) }}
        </button>
      </div>

      <!-- 等待支付 -->
      <div v-else-if="step === 'paying'" class="pm-body">
        <div class="qrbox">
          <img v-if="qrSrc" :src="qrSrc" alt="支付二维码" class="qr" />
          <div v-else class="qrph">
            <svg viewBox="0 0 24 24" width="34" height="34" fill="none" stroke="currentColor"
                 stroke-width="1.5" stroke-linecap="round">
              <rect x="4" y="4" width="6" height="6" rx="1.4"/>
              <rect x="14" y="4" width="6" height="6" rx="1.4"/>
              <rect x="4" y="14" width="6" height="6" rx="1.4"/>
              <path d="M14 14h3v3h-3zM20 14v3M14 20h3M20 20h.01"/>
            </svg>
            <span>点击下方按钮打开收银台</span>
          </div>
        </div>

        <button class="btn pri lg go" @click="openPay">
          打开收银台 ↗
        </button>
        <p class="tiny dim center-text">已在新窗口打开？支付完成后本页会自动更新。</p>

        <div class="waiting">
          <span class="pulse" />
          <span>等待到账…（已检测 {{ poll }} 次）</span>
        </div>

        <div class="orderline">
          <span class="tiny dim">订单号</span>
          <code class="tiny">{{ order?.order_no }}</code>
          <button class="tiny copybtn" :class="{ done: copied }" @click="copyNo">
            {{ copied ? '已复制' : '复制' }}
          </button>
        </div>
      </div>

      <!-- 手动 -->
      <div v-else-if="step === 'manual'" class="pm-body">
        <div class="manualbox">
          <p class="eyebrow">ORDER</p>
          <code class="ordno">{{ order?.order_no }}</code>
          <p class="sub">
            订单已创建（¥{{ amount }} · {{ months }} 个月）。
            把订单号发给管理员，确认收款后档位自动开通。
          </p>
        </div>
        <button class="btn lg go" @click="copyNo">{{ copied ? '订单号已复制' : '复制订单号' }}</button>
        <p class="tiny dim center-text">
          也可以先在「兑换码」里用卡密即时开通，不需要等待。
        </p>
      </div>

      <!-- 成功 -->
      <div v-else-if="step === 'done'" class="pm-body done">
        <div class="okring">
          <svg viewBox="0 0 24 24" width="30" height="30" fill="none" stroke="currentColor"
               stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M5 12.6l4.3 4.2L19 6.9"/>
          </svg>
        </div>
        <h3>{{ tier.code.toUpperCase() }} 已开通</h3>
        <p class="sub center-text">{{ months }} 个月 · ¥{{ amount }}</p>
        <button class="btn pri lg go" @click="emit('close')">开始使用</button>
      </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.pm-scrim{position:fixed;inset:0;z-index:210;display:grid;place-items:center;
  padding:calc(16px + env(safe-area-inset-top)) 16px calc(16px + env(safe-area-inset-bottom));
  background:rgba(0,0,0,.55);-webkit-backdrop-filter:blur(7px);backdrop-filter:blur(7px);
  overscroll-behavior:contain;
  animation:k-fade-in 200ms var(--e-out) both}
.pm{width:100%;max-width:400px;padding:18px;display:flex;flex-direction:column;gap:14px;
  animation:k-scale-spring 460ms cubic-bezier(.34,1.32,.64,1) both;
  max-height:calc(100dvh - 24px - env(safe-area-inset-top) - env(safe-area-inset-bottom));
  overflow-y:auto;overscroll-behavior:contain;-webkit-overflow-scrolling:touch}

.pm-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.pm-head h3{margin-top:3px;font-size:16px}
.pm-x{width:28px;height:28px;flex:none;border-radius:50%;display:grid;place-items:center;
  color:var(--tx-3);background:rgba(255,255,255,.05);border:1px solid var(--line);
  transition:transform 220ms var(--e-out),background-color 180ms}
.pm-x:active{transform:scale(.9);background:rgba(255,255,255,.1)}

.pm-sum{display:flex;align-items:center;justify-content:space-between;gap:12px;
  padding:11px 13px;border-radius:12px;background:rgba(255,255,255,.04);border:1px solid var(--line)}
.sum-l{display:flex;flex-direction:column;gap:2px;min-width:0}
.sum-l b{font-size:12.5px;font-weight:640}
.sum-r{display:flex;align-items:baseline;gap:1px;flex:none}
.sum-r .cur{font-size:12px;color:var(--tx-3)}
.sum-r b{font-size:21px;font-weight:720;letter-spacing:-.03em;font-variant-numeric:tabular-nums}

.pm-body{display:flex;flex-direction:column;gap:12px}
.methods{display:flex;flex-direction:column;gap:7px}
.mth{display:flex;align-items:center;gap:10px;padding:10px 12px;border-radius:11px;
  border:1px solid var(--line);background:rgba(255,255,255,.025);
  transition:all 220ms var(--e-out)}
.mth.on{border-color:var(--ac-line);background:rgba(255,138,61,.08)}
.mth.wechat.on{border-color:rgba(84,201,138,.4);background:rgba(84,201,138,.09)}
.mico{width:26px;height:26px;flex:none;border-radius:8px;display:grid;place-items:center;
  background:rgba(255,255,255,.06);color:var(--tx-3)}
.mth.wechat .mico{color:#8fe8b6;background:rgba(84,201,138,.12)}
.mth.alipay .mico{color:#79a8f0;background:rgba(121,168,240,.12)}
.mtxt{flex:1;font-size:12.5px;font-weight:600;color:var(--tx-2);text-align:left}
.mth.on .mtxt{color:var(--tx-1)}
.mck{width:16px;height:16px;flex:none;border-radius:50%;display:grid;place-items:center;
  color:transparent;border:1.5px solid var(--line-2);transition:all 220ms}
.mth.on .mck{color:#fff;background:var(--ac);border-color:var(--ac)}
.mth.wechat.on .mck{background:var(--ok);border-color:var(--ok)}

.note{line-height:1.6}
.go{width:100%}
.errline{font-size:12px;color:#f6b1ac;background:rgba(240,103,95,.10);
  border:1px solid rgba(240,103,95,.22);border-radius:9px;padding:8px 11px}

.qrbox{display:grid;place-items:center;padding:8px}
.qr{width:170px;height:170px;border-radius:12px;background:#fff;padding:6px}
.qrph{width:170px;height:170px;border-radius:12px;display:flex;flex-direction:column;
  align-items:center;justify-content:center;gap:8px;
  border:1px dashed rgba(255,255,255,.16);background:rgba(0,0,0,.2);color:var(--tx-4)}
.qrph span{font-size:10.5px}

.waiting{display:flex;align-items:center;justify-content:center;gap:8px;
  font-size:11px;color:var(--tx-4)}
.pulse{width:6px;height:6px;border-radius:50%;background:var(--ac);
  box-shadow:0 0 8px 1px rgba(255,138,61,.6);animation:k-breathe 1.8s var(--e-in-out) infinite}

.orderline{display:flex;align-items:center;gap:7px;justify-content:center;
  padding-top:9px;border-top:1px solid var(--line)}
.orderline code{color:var(--tx-3)}
.copybtn{color:var(--ac-2);padding:2px 6px;border-radius:5px;background:var(--ac-dim)}
.copybtn.done{color:#8fe8b6;background:rgba(84,201,138,.12)}

.manualbox{padding:14px;border-radius:12px;background:rgba(255,255,255,.03);
  border:1px solid var(--line);display:flex;flex-direction:column;gap:7px;align-items:center;text-align:center}
.ordno{font-size:13px;color:var(--ac-2);background:var(--ac-dim);padding:6px 10px;border-radius:8px;
  letter-spacing:.02em}
.center-text{text-align:center;line-height:1.6}

.done{align-items:center;padding:8px 0}
.okring{width:60px;height:60px;border-radius:50%;display:grid;place-items:center;
  color:var(--ok);background:rgba(84,201,138,.12);border:1px solid rgba(84,201,138,.3);
  animation:k-scale-spring 600ms cubic-bezier(.34,1.4,.64,1) both}
.done h3{font-size:15px;margin-top:2px}

.spin{width:13px;height:13px;border-radius:50%;border:1.8px solid rgba(255,255,255,.3);
  border-top-color:#fff;animation:spin .7s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
</style>

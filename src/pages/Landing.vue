<script setup>
/* ============================================================
   Landing.vue — 未登录落地页（移动优先）
   目标：一屏之内说清「这是什么 / 怎么用 / 多少钱」，再引导注册。
   ============================================================ */
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { api } from '../lib/api.js'

const emit = defineEmits(['login'])

const status = ref(null)
const sponsor = ref([])

const modelCount = computed(() => status.value?.model_count || 0)
const poolOk = computed(() => !!status.value?.upstream?.ok)
const models = computed(() => status.value?.models || [])

onMounted(async () => {
  // 公开接口，拿不到也不影响首屏
  api.status().then(d => { status.value = d }).catch(() => {})
  api.plans().then(d => {
    sponsor.value = (d.sponsor || []).map(p => ({
      code: p.code, name: p.name, tagline: p.tagline, price: p.price, year: p.year_price,
      daily: p.daily, conc: p.conc, rpm: p.rpm, hot: p.hot,
    }))
    // 价格卡片是异步渲染出来的，必须等 DOM 更新后再挂观察器，
    // 否则它们永远停在 opacity:0（移动端看不到价格表）。
    nextTick(setupReveal)
  }).catch(() => {})
  nextTick(setupReveal)
})

/* ---------------- 滚动显现 ---------------- */
let io = null
function setupReveal() {
  // 数据异步到达后可能新增 .lp-rv 节点（如价格卡片），需要重新收集：
  // 只挑还没 .in 的，避免重复观察已显现的元素
  const els = document.querySelectorAll('.lp-rv:not(.in)')
  if (!els.length) return
  if (!('IntersectionObserver' in window)) {
    els.forEach(e => e.classList.add('in'))
    return
  }
  if (!io) {
    io = new IntersectionObserver((entries) => {
      entries.forEach(en => {
        if (en.isIntersecting) {
          en.target.classList.add('in')
          io.unobserve(en.target)
        }
      })
    }, { rootMargin: '0px 0px -12% 0px', threshold: 0.08 })
  }
  els.forEach(e => io.observe(e))
}
onBeforeUnmount(() => { io && io.disconnect() })

/* ---------------- 顶栏：滚动后加深 ---------------- */
const solid = ref(false)
function onScroll() { solid.value = window.scrollY > 24 }
onMounted(() => window.addEventListener('scroll', onScroll, { passive: true }))
onBeforeUnmount(() => window.removeEventListener('scroll', onScroll))

/* ---------------- 代码示例 ---------------- */
const TAB = ref('python')
const CODE = {
  python: `from openai import OpenAI

client = OpenAI(
    base_url="https://your-domain.com/v1",
    api_key="sk-wb-…",          # 控制台里签发
)

stream = client.chat.completions.create(
    model="glm-5.3-flash",
    messages=[{"role": "user", "content": "你好"}],
    stream=True,
)
for c in stream:
    print(c.choices[0].delta.content or "", end="")`,
  curl: `curl https://your-domain.com/v1/chat/completions \\
  -H "Authorization: Bearer sk-wb-…" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"glm-5.3-flash",
       "messages":[{"role":"user","content":"你好"}],
       "stream":true}'`,
  node: `import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: 'https://your-domain.com/v1',
  apiKey: process.env.WB2_KEY,
})

const r = await client.chat.completions.create({
  model: 'glm-5.3-flash',
  messages: [{ role: 'user', content: '你好' }],
  stream: true,
})`,
}
const code = computed(() => CODE[TAB.value])

const copied = ref(false)
let copyTimer = null
async function copyCode() {
  try {
    await navigator.clipboard.writeText(code.value)
  } catch (_) {
    const ta = document.createElement('textarea')
    ta.value = code.value
    ta.style.cssText = 'position:fixed;opacity:0'
    document.body.appendChild(ta)
    ta.select()
    try { document.execCommand('copy') } catch (__) {}
    document.body.removeChild(ta)
  }
  copied.value = true
  clearTimeout(copyTimer)
  copyTimer = setTimeout(() => { copied.value = false }, 1600)
}

/* ---------------- 能力 ---------------- */
const FEATURES = [
  { t: 'OpenAI 兼容', d: '改个 base_url 就能跑。SDK、客户端、插件全都不用动。',
    i: '<path d="M4 7.2A3.2 3.2 0 017.2 4h9.6A3.2 3.2 0 0120 7.2v9.6a3.2 3.2 0 01-3.2 3.2H7.2A3.2 3.2 0 014 16.8z"/><path d="M8.6 12h6.8M12 8.6v6.8"/>' },
  { t: '账号池轮换', d: '上游账号自动调度，遇冷却、限流、掉线就换下一个，请求不会砸在你脸上。',
    i: '<path d="M4 12a8 8 0 0113.7-5.6"/><path d="M20 12a8 8 0 01-13.7 5.6"/><path d="M17.7 3.4v3.4h-3.4M6.3 20.6v-3.4h3.4"/>' },
  { t: 'SSE 流式', d: '首字直出，不攒包。nginx 关缓冲，长连接能挂满 10 分钟。',
    i: '<path d="M3.6 12h3.2l2.4-6 3.6 12 2.4-6h5.2"/>' },
  { t: '实时用量', d: '每次调用都有记录：模型、token、点数、TTFT、耗时。按天按模型都能看。',
    i: '<path d="M4 19.2V10"/><path d="M9.3 19.2V5.4"/><path d="M14.7 19.2v-6.6"/><path d="M20 19.2V8"/>' },
  { t: '密钥隔离', d: '一账号多把 key，随时轮换或吊销。库里只存哈希，明文加密备份。',
    i: '<circle cx="8" cy="12" r="3.6"/><path d="M11.6 12H21M18 12v3.2M15 12v2.2"/>' },
  { t: '额度透明', d: '日额度按档位固定，超额先扣加油包，用到 150% 才拦。不偷偷限你。',
    i: '<circle cx="12" cy="12" r="8.4"/><path d="M12 7.4v5l3.4 2"/>' },
]

/* ---------------- 起步三步 ---------------- */
const STEPS = [
  { n: '01', t: '注册', d: '邮箱验证码验证，10 秒。送 3000 算力点，7 天可用。' },
  { n: '02', t: '签发密钥', d: '控制台点一下，得到 sk-wb-… 开头的 key。' },
  { n: '03', t: '改 base_url', d: '把客户端指向 /v1，模型名照抄列表里的即可。' },
]

const fmtPts = (v) => (v >= 10000 ? (v / 10000).toFixed(v % 10000 ? 1 : 0) + ' 万' : String(v))

function goLogin(tab) {
  emit('login', tab)
}

function backTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
</script>

<template>
  <div class="lp">
    <!-- ============ 顶栏 ============ -->
    <header class="lp-nav" :class="{ solid }">
      <div class="lp-nav-in">
        <a class="lp-brand" href="#" @click.prevent="backTop">
          <span class="lp-mark">
            <svg viewBox="0 0 32 32" width="20" height="20" fill="none">
              <circle cx="16" cy="16" r="10.5" stroke="currentColor" stroke-width="3"
                      stroke-linecap="round" stroke-dasharray="46 20" transform="rotate(-45 16 16)" />
            </svg>
          </span>
          <span class="lp-bname">WorkBuddy <b>2.0</b></span>
        </a>

        <nav class="lp-links">
          <a href="#how">怎么用</a>
          <a href="#models">模型</a>
          <a href="#price">价格</a>
        </nav>

        <div class="lp-nav-end">
          <button class="btn sm ghost lp-hide-sm" @click="goLogin('login')">登录</button>
          <button class="btn sm pri" @click="goLogin('register')">免费开始</button>
        </div>
      </div>
    </header>

    <main>
      <!-- ============ Hero ============ -->
      <section class="lp-hero">
        <div class="lp-wrap">
          <p class="lp-eyebrow">
            <span class="lp-live" :class="{ off: !poolOk }" />
            {{ poolOk ? '账号池在线' : '正在连接账号池' }} · 独立实例 v2.0
          </p>

          <h1 class="lp-h1">
            一个 OpenAI 接口，<br />
            背后是<b>整池</b> WorkBuddy 账号。
          </h1>

          <p class="lp-lead">
            群友授权的 WorkBuddy / CodeBuddy 账号在这里汇成池子。
            你按标准 OpenAI 协议调用，路由、冷却、轮换、计费都在服务端完成——
            客户端一行代码都不用改。
          </p>

          <div class="lp-cta">
            <button class="btn pri lg" @click="goLogin('register')">
              免费注册
              <span class="lp-cta-sub">送 3000 点</span>
            </button>
            <button class="btn lg" @click="goLogin('login')">已有账号，登录</button>
          </div>

          <!-- 实时状态条 -->
          <dl class="lp-stats">
            <div>
              <dt>可用模型</dt>
              <dd>{{ modelCount || '—' }}<i>个</i></dd>
            </div>
            <div>
              <dt>账号池</dt>
              <dd :class="{ okc: poolOk }">{{ poolOk ? '在线' : '待连接' }}</dd>
            </div>
            <div>
              <dt>一次对话约</dt>
              <dd>4<i>点</i></dd>
            </div>
            <div>
              <dt>1 元兑换</dt>
              <dd>1000<i>点</i></dd>
            </div>
          </dl>

          <p class="lp-endpoint">
            <span class="mono">Base URL</span>
            <code>https://your-domain.com/v1</code>
          </p>
        </div>
      </section>

      <!-- ============ 代码 ============ -->
      <section class="lp-sec lp-code-sec">
        <div class="lp-wrap">
          <div class="lp-codecard glass glass-2 lp-rv">
            <div class="lp-codehead">
              <div class="lp-tabs">
                <button v-for="t in [['python','Python'],['curl','cURL'],['node','Node']]" :key="t[0]"
                        :data-on="TAB === t[0] ? 1 : 0" @click="TAB = t[0]">{{ t[1] }}</button>
              </div>
              <button class="lp-copy" @click="copyCode">
                <svg v-if="!copied" width="13" height="13" viewBox="0 0 24 24" fill="none"
                     stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="8.4" y="8.4" width="11.2" height="11.2" rx="2.2"/>
                  <path d="M15.6 5.4H6.6a2 2 0 00-2 2v9"/>
                </svg>
                <svg v-else width="13" height="13" viewBox="0 0 24 24" fill="none"
                     stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M5 12.6l4.4 4.2L19 6.6"/>
                </svg>
                {{ copied ? '已复制' : '复制' }}
              </button>
            </div>
            <pre class="lp-pre"><code>{{ code }}</code></pre>
          </div>
        </div>
      </section>

      <!-- ============ 能力 ============ -->
      <section class="lp-sec">
        <div class="lp-wrap">
          <header class="lp-shead lp-rv">
            <p class="eyebrow">WHY</p>
            <h2>不是又一个转发，是把账号管起来</h2>
          </header>
          <div class="lp-grid">
            <article v-for="(f, i) in FEATURES" :key="f.t" class="lp-feat glass glass-1 lp-rv"
                     :style="{ '--d': i * 55 + 'ms' }">
              <span class="lp-fic">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"
                     stroke-linecap="round" stroke-linejoin="round" v-html="f.i" />
              </span>
              <h3>{{ f.t }}</h3>
              <p>{{ f.d }}</p>
            </article>
          </div>
        </div>
      </section>

      <!-- ============ 模型 ============ -->
      <section id="models" class="lp-sec">
        <div class="lp-wrap">
          <header class="lp-shead lp-rv">
            <p class="eyebrow">MODELS</p>
            <h2>{{ modelCount || '二十多' }} 个模型，同一个 key</h2>
            <p class="lp-slead">列表每天自动刷新。模型名直接照抄，不用记路由。</p>
          </header>
          <ul class="lp-chips lp-rv">
            <li v-for="m in models.slice(0, 14)" :key="m">{{ m }}</li>
            <li v-if="models.length > 14" class="more">+ {{ models.length - 14 }} 个</li>
            <li v-if="!models.length" class="more">加载中…</li>
          </ul>
        </div>
      </section>

      <!-- ============ 起步 ============ -->
      <section id="how" class="lp-sec">
        <div class="lp-wrap">
          <header class="lp-shead lp-rv">
            <p class="eyebrow">START</p>
            <h2>三步，两分钟</h2>
          </header>
          <ol class="lp-steps">
            <li v-for="(s, i) in STEPS" :key="s.n" class="lp-rv" :style="{ '--d': i * 70 + 'ms' }">
              <span class="lp-n mono">{{ s.n }}</span>
              <h3>{{ s.t }}</h3>
              <p>{{ s.d }}</p>
            </li>
          </ol>
        </div>
      </section>

      <!-- ============ 价格预览 ============ -->
      <section id="price" class="lp-sec">
        <div class="lp-wrap">
          <header class="lp-shead lp-rv">
            <p class="eyebrow">PRICE</p>
            <h2>两条路：赞助，或者交账号</h2>
            <p class="lp-slead">不交钱也能用——注册前 7 天每天 8000 点，另送 3000 点加油包；之后不付费每天 200 点。</p>
          </header>

          <div class="lp-plans">
            <article v-for="p in sponsor" :key="p.code"
                     class="lp-plan glass glass-2 lp-rv" :class="{ hot: p.hot }">
              <div class="lp-plan-top">
                <b>{{ p.name }}</b>
                <span v-if="p.hot" class="pill ac">最多人选</span>
              </div>
              <p class="lp-price">
                <i>¥</i>{{ p.price }}<em>/月</em>
              </p>
              <p class="lp-plan-tag">{{ p.tagline }}</p>
              <ul class="lp-plan-li">
                <li>每日 {{ fmtPts(p.daily) }} 点</li>
                <li>并发 {{ p.conc }} · {{ p.rpm }} 请求/分</li>
              </ul>
              <button class="btn sm" :class="p.hot ? 'pri' : ''" @click="goLogin('register')">选这档</button>
            </article>

            <article v-if="!sponsor.length" class="lp-plan glass glass-1 lp-rv">
              <p class="lp-plan-tag">档位加载中…</p>
            </article>
          </div>

          <p class="lp-plan-more lp-rv">
            也能用账号换额度：交 1 个账号，按余额 30%–55% 折算成每月点数，保底比赞助更高。
            <button class="lp-link" @click="goLogin('login')">登录后看全部档位 →</button>
          </p>
        </div>
      </section>

      <!-- ============ 收尾 ============ -->
      <section class="lp-sec lp-final">
        <div class="lp-wrap">
          <div class="lp-finalcard glass glass-3 lp-rv">
            <h2>现在就可以发第一个请求</h2>
            <p>注册只要邮箱验证码 + 用户名密码，也能用邮箱验证码直接登录。额度、密钥、用量都在控制台里。</p>
            <div class="lp-cta center">
              <button class="btn pri lg" @click="goLogin('register')">免费注册</button>
              <button class="btn lg" @click="goLogin('login')">登录</button>
            </div>
          </div>
        </div>
      </section>
    </main>

    <footer class="lp-foot">
      <div class="lp-wrap">
        <span>WorkBuddy 2.0 · 独立实例</span>
        <span class="lp-foot-end">
          <span class="mono">v2.0.0</span>
          <button class="lp-link" @click="goLogin('login')">控制台</button>
        </span>
      </div>
    </footer>
  </div>
</template>

<style scoped>
/* ================= 基础 ================= */
.lp{position:relative;z-index:1;min-height:100dvh;
  padding-bottom:calc(var(--safe-b) + 8px)}
.lp-wrap{width:100%;max-width:1120px;margin:0 auto;padding:0 var(--pad)}

/* ================= 顶栏 ================= */
.lp-nav{position:sticky;top:0;z-index:50;transition:background-color 320ms var(--e-out-soft),
  border-color 320ms var(--e-out-soft),backdrop-filter 320ms}
.lp-nav::after{content:"";position:absolute;inset:0;z-index:-1;pointer-events:none;
  background:linear-gradient(180deg,rgba(8,8,10,.72),rgba(8,8,10,.18) 70%,transparent);
  -webkit-backdrop-filter:blur(16px) saturate(160%);backdrop-filter:blur(16px) saturate(160%);
  opacity:1;transition:opacity 300ms}
.lp-nav.solid::after{background:linear-gradient(180deg,rgba(8,8,10,.92),rgba(8,8,10,.72));
  box-shadow:0 1px 0 var(--line)}
.lp-nav-in{max-width:1120px;margin:0 auto;height:calc(var(--nav-h) + var(--safe-t));
  padding:var(--safe-t) var(--pad) 0;
  display:flex;align-items:center;gap:14px}
.lp-brand{display:flex;align-items:center;gap:9px;flex:none}
.lp-mark{color:var(--ac);display:grid;place-items:center;
  filter:drop-shadow(0 0 12px rgba(255,138,61,.3))}
.lp-bname{font-size:14px;font-weight:700;letter-spacing:-.02em;display:flex;align-items:center;gap:5px}
.lp-bname b{font-size:9.5px;color:var(--ac-2);font-weight:750;padding:1.5px 5px;border-radius:5px;
  background:var(--ac-dim);border:1px solid var(--ac-line)}
.lp-links{display:none;align-items:center;gap:22px;margin-left:14px;font-size:13px;color:var(--tx-3)}
.lp-links a{transition:color 200ms}
.lp-links a:hover{color:var(--tx-1)}
.lp-nav-end{margin-left:auto;display:flex;align-items:center;gap:8px}
.lp-hide-sm{display:none}

/* ================= Hero ================= */
.lp-hero{padding:clamp(34px,7vw,86px) 0 clamp(8px,2vw,20px)}
.lp-eyebrow{display:inline-flex;align-items:center;gap:8px;
  font-size:11.5px;font-weight:620;letter-spacing:.01em;color:var(--tx-3);
  padding:5px 11px;border-radius:var(--r-full);
  border:1px solid var(--line-2);background:rgba(255,255,255,.04);
  animation:k-fade-up 560ms var(--e-out) both}
.lp-live{width:6px;height:6px;border-radius:50%;background:var(--ok);flex:none;
  box-shadow:0 0 8px 1px rgba(84,201,138,.55);animation:k-breathe 2.4s var(--e-in-out) infinite}
.lp-live.off{background:var(--warn);box-shadow:0 0 8px 1px rgba(232,181,82,.5)}

.lp-h1{margin-top:16px;font-size:clamp(29px,7.4vw,54px);line-height:1.1;
  letter-spacing:-.042em;font-weight:730;
  animation:k-fade-up 640ms 60ms var(--e-out) both}
.lp-h1 b{color:var(--ac-2);font-weight:730}

.lp-lead{margin-top:16px;max-width:46ch;font-size:clamp(13.5px,3.6vw,15.5px);
  line-height:1.8;color:var(--tx-3);
  animation:k-fade-up 640ms 120ms var(--e-out) both}

.lp-cta{margin-top:24px;display:flex;flex-wrap:wrap;gap:10px;
  animation:k-fade-up 640ms 180ms var(--e-out) both}
.lp-cta.center{justify-content:center;margin-top:20px}
.lp-cta .btn{flex:0 0 auto}
.lp-cta-sub{font-size:10.5px;font-weight:600;color:#ffe0c9;opacity:.72;
  padding-left:8px;margin-left:2px;border-left:1px solid rgba(255,196,150,.3)}

/* 状态条 */
.lp-stats{margin:30px 0 0;display:grid;grid-template-columns:repeat(4,1fr);
  gap:2px;padding:0;border-top:1px solid var(--line);
  animation:k-fade-up 640ms 240ms var(--e-out) both}
.lp-stats > div{padding:14px 0 0;min-width:0}
.lp-stats dt{font-size:10.5px;color:var(--tx-4);letter-spacing:.02em;margin-bottom:3px}
.lp-stats dd{margin:0;font-size:clamp(16px,4.4vw,22px);font-weight:720;letter-spacing:-.03em;
  font-variant-numeric:tabular-nums;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.lp-stats dd i{font-style:normal;font-size:10.5px;font-weight:600;color:var(--tx-4);margin-left:2px}
.lp-stats dd.okc{color:#8fe8b6}

.lp-endpoint{margin-top:20px;display:flex;align-items:center;gap:9px;flex-wrap:wrap;
  font-size:11.5px;color:var(--tx-4);
  animation:k-fade-up 640ms 300ms var(--e-out) both}
.lp-endpoint .mono{letter-spacing:.06em;text-transform:uppercase;font-size:10px}
.lp-endpoint code{font-size:11.5px;color:var(--ac-2);background:var(--ac-dim);
  border:1px solid rgba(255,138,61,.18);padding:3px 8px;border-radius:6px}

/* ================= 区块 ================= */
.lp-sec{padding:clamp(38px,8vw,76px) 0}
.lp-shead{margin-bottom:22px}
.lp-shead h2{font-size:clamp(21px,5.2vw,30px);letter-spacing:-.03em;margin-top:8px}
.lp-slead{margin-top:9px;color:var(--tx-3);font-size:13.5px;line-height:1.75;max-width:50ch}

/* 滚动显现 */
.lp-rv{opacity:0;transform:translate3d(0,18px,0);
  transition:opacity 620ms var(--e-out) var(--d,0ms),transform 620ms var(--e-out) var(--d,0ms)}
.lp-rv.in{opacity:1;transform:none}

/* ================= 代码卡 ================= */
.lp-code-sec{padding-top:0}
.lp-codecard{border-radius:var(--r-lg);overflow:hidden}
.lp-codehead{display:flex;align-items:center;gap:10px;padding:9px 10px 9px 12px;
  border-bottom:1px solid var(--line)}
.lp-tabs{display:flex;gap:2px}
.lp-tabs button{height:28px;padding:0 11px;border-radius:7px;font-size:12px;font-weight:600;
  color:var(--tx-4);transition:color 200ms,background-color 200ms}
.lp-tabs button[data-on="1"]{color:var(--tx-1);background:rgba(255,255,255,.075)}
.lp-copy{margin-left:auto;display:inline-flex;align-items:center;gap:6px;
  height:28px;padding:0 9px;border-radius:7px;font-size:11.5px;color:var(--tx-3);
  border:1px solid transparent;transition:color 180ms,background-color 180ms,border-color 180ms}
.lp-copy:hover{color:var(--tx-1);background:rgba(255,255,255,.06);border-color:var(--line)}
.lp-copy:active{transform:scale(.96)}
.lp-pre{margin:0;padding:14px 14px 16px;overflow-x:auto;overscroll-behavior-x:contain;
  -webkit-overflow-scrolling:touch;scrollbar-width:none}
.lp-pre::-webkit-scrollbar{display:none}
.lp-pre code{font-size:11.5px;line-height:1.72;color:var(--tx-2);white-space:pre;display:block}

/* ================= 能力 ================= */
.lp-grid{display:grid;grid-template-columns:1fr;gap:10px}
.lp-feat{padding:17px 16px;border-radius:var(--r-md)}
.lp-fic{display:grid;place-items:center;width:31px;height:31px;border-radius:9px;
  background:var(--ac-dim);border:1px solid rgba(255,138,61,.2);color:var(--ac-2);margin-bottom:11px}
.lp-fic svg{width:16px;height:16px}
.lp-feat h3{font-size:14.5px;margin-bottom:5px}
.lp-feat p{font-size:12.5px;line-height:1.72;color:var(--tx-3)}

/* ================= 模型 ================= */
.lp-chips{display:flex;flex-wrap:wrap;gap:7px}
.lp-chips li{font-family:var(--mono);font-size:11.5px;color:var(--tx-2);
  padding:6px 10px;border-radius:8px;border:1px solid var(--line);
  background:rgba(255,255,255,.035);transition:border-color 200ms,color 200ms}
.lp-chips li.more{font-family:var(--sf);color:var(--tx-4);border-style:dashed}

/* ================= 三步 ================= */
.lp-steps{display:grid;grid-template-columns:1fr;gap:10px;counter-reset:s}
.lp-steps li{position:relative;padding:16px;border-radius:var(--r-md);
  border:1px solid var(--line);background:rgba(255,255,255,.028)}
.lp-n{font-size:10.5px;font-weight:700;color:var(--ac-2);letter-spacing:.1em}
.lp-steps h3{font-size:14.5px;margin:7px 0 5px}
.lp-steps p{font-size:12.5px;line-height:1.72;color:var(--tx-3)}

/* ================= 价格 ================= */
.lp-plans{display:grid;grid-template-columns:1fr;gap:10px}
.lp-plan{padding:18px 16px;border-radius:var(--r-md);display:flex;flex-direction:column;gap:9px}
.lp-plan.hot{border-color:rgba(255,138,61,.24)}
.lp-plan-top{display:flex;align-items:center;gap:8px}
.lp-plan-top b{font-size:14.5px;letter-spacing:-.01em}
.lp-price{display:flex;align-items:baseline;gap:2px;font-size:30px;font-weight:730;
  letter-spacing:-.04em;font-variant-numeric:tabular-nums}
.lp-price i{font-style:normal;font-size:15px;font-weight:600;color:var(--tx-3);margin-right:1px}
.lp-price em{font-style:normal;font-size:11.5px;font-weight:600;color:var(--tx-4);margin-left:3px}
.lp-plan-tag{font-size:12px;color:var(--tx-3);line-height:1.6;min-height:1.6em}
.lp-plan-li{display:flex;flex-direction:column;gap:5px;font-size:12.5px;color:var(--tx-2);
  padding-top:2px;margin-bottom:4px}
.lp-plan-li li{position:relative;padding-left:15px}
.lp-plan-li li::before{content:"";position:absolute;left:0;top:.52em;width:6px;height:6px;
  border-radius:2px;background:var(--ac-line)}
.lp-plan .btn{margin-top:auto;width:100%}
.lp-plan-more{margin-top:16px;font-size:12.5px;line-height:1.8;color:var(--tx-4)}
.lp-link{color:var(--ac-2);font-size:inherit;transition:opacity 180ms}
.lp-link:hover{opacity:.78}

/* ================= 收尾 ================= */
.lp-final{padding-bottom:clamp(30px,6vw,56px)}
.lp-finalcard{padding:clamp(24px,6vw,42px);border-radius:var(--r-xl);text-align:center}
.lp-finalcard h2{font-size:clamp(20px,5vw,28px);letter-spacing:-.03em}
.lp-finalcard p{margin-top:10px;color:var(--tx-3);font-size:13.5px;line-height:1.75}

/* ================= 页脚 ================= */
.lp-foot{border-top:1px solid var(--line);padding:18px 0 calc(20px + var(--safe-b))}
.lp-foot > .lp-wrap{display:flex;align-items:center;justify-content:space-between;gap:12px;
  font-size:11.5px;color:var(--tx-4)}
.lp-foot-end{display:inline-flex;align-items:center;gap:14px}

/* ================= 断点 ================= */
@media (min-width:720px){
  .lp-hide-sm{display:inline-flex}
  .lp-grid{grid-template-columns:repeat(2,1fr);gap:12px}
  .lp-plans{grid-template-columns:repeat(3,1fr);gap:12px}
  .lp-steps{grid-template-columns:repeat(3,1fr)}
}
@media (min-width:960px){
  .lp-links{display:flex}
  .lp-grid{grid-template-columns:repeat(3,1fr)}
  .lp-pre code{font-size:12.5px}
  .lp-hero{padding-top:clamp(58px,7vw,96px)}
}
@media (max-width:400px){
  .lp-stats{grid-template-columns:repeat(2,1fr);gap:0 10px}
  .lp-stats > div{padding-top:12px}
  .lp-cta .btn{flex:1 1 auto}
}
</style>

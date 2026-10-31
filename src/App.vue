<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch, defineAsyncComponent } from 'vue'
import GlassSidebar from './components/GlassSidebar.vue'
import PerfSwitch from './components/PerfSwitch.vue'
import Landing from './pages/Landing.vue'
import Dashboard from './pages/Dashboard.vue'
import Accounts from './pages/Accounts.vue'
import Invite from './pages/Invite.vue'
import Playground from './pages/Playground.vue'
import Pricing from './pages/Pricing.vue'
import Login from './pages/Login.vue'
import Keys from './pages/Keys.vue'
import Usage from './pages/Usage.vue'
import Lottery from './pages/Lottery.vue'
const Admin = defineAsyncComponent(() => import('./pages/Admin.vue'))
const Docs = defineAsyncComponent(() => import('./pages/Docs.vue'))
import { api, state, logout as doLogout, onAuthFail, setSession } from './lib/api.js'

/* ---------------- 导航 ---------------- */
const I = (d) => `<svg viewBox="0 0 24 24">${d}</svg>`
const items = [
  { id: 'dashboard', label: '运行概览', group: '总览', icon: I('<rect x="3.2" y="3.2" width="7.4" height="7.4" rx="2.2"/><rect x="13.4" y="3.2" width="7.4" height="7.4" rx="2.2"/><rect x="3.2" y="13.4" width="7.4" height="7.4" rx="2.2"/><rect x="13.4" y="13.4" width="7.4" height="7.4" rx="2.2"/>') },
  { id: 'pricing', label: '赞助与权益', group: '管理', icon: I('<path d="M12 3.6l2.5 5.4 5.9.8-4.3 4.1 1.1 5.9L12 17l-5.2 2.8 1.1-5.9-4.3-4.1 5.9-.8z"/>'), badge: '价' },
  { id: 'accounts', label: '账号池', group: '管理', icon: I('<circle cx="9" cy="8.4" r="3.1"/><path d="M3.6 19.4c0-2.9 2.4-5 5.4-5s5.4 2.1 5.4 5"/><path d="M16.4 6.2a2.7 2.7 0 010 5.2"/><path d="M18 14.9c2 .5 3.4 2.2 3.4 4.5"/>'), dot: true },
  { id: 'invite', label: '邀请返利', group: '管理', icon: I('<circle cx="9" cy="8.6" r="3"/><path d="M4 19.4c0-2.8 2.2-4.8 5-4.8s5 2 5 4.8"/><path d="M17.4 8.2v5M19.9 10.7h-5"/>') },
  { id: 'lottery', label: '抽奖', group: '管理', icon: I('<rect x="4" y="9.6" width="16" height="10.4" rx="2.2"/><path d="M4 14.2h16M12 9.6v10.4"/><path d="M8.6 9.6C7.2 9.6 6 8.6 6 7.3s1-2.1 2.3-2.1c1.9 0 3.7 4.4 3.7 4.4M15.4 9.6c1.4 0 2.6-1 2.6-2.3s-1-2.1-2.3-2.1c-1.9 0-3.7 4.4-3.7 4.4"/>'), badge: '新' },
  { id: 'keys', label: 'API 密钥', group: '管理', icon: I('<circle cx="8" cy="12" r="3.6"/><path d="M11.6 12H21"/><path d="M18 12v3.2"/><path d="M15 12v2.2"/>') },
  { id: 'usage', label: '用量与账单', group: '管理', icon: I('<path d="M4 19.2V10"/><path d="M9.3 19.2V5.4"/><path d="M14.7 19.2v-6.6"/><path d="M20 19.2V8"/>') },
  { id: 'playground', label: '对话测试', group: '开发', icon: I('<path d="M4.4 6.6a2 2 0 012-2h11.2a2 2 0 012 2v7.2a2 2 0 01-2 2H9.6l-4 3.4v-3.4a1 1 0 01-1.2-1z"/>') },
  { id: 'admin', label: '管理台', group: '管理', icon: I('<path d="M12 3.6l7 2.6v5.2c0 4.2-2.9 7.2-7 8.6-4.1-1.4-7-4.4-7-8.6V6.4l7-2.6z"/><path d="M9 12l2.2 2.2L15.4 10"/>') },
  { id: 'docs', label: '接口文档', group: '开发', icon: I('<path d="M5 4.4h9.2L19 9v10.6H5z"/><path d="M14 4.6V9h4.6"/><path d="M8.2 13h7.6M8.2 16.4h5"/>') },
]

const active = ref('dashboard')
const sbOpen = ref(false)
const user = ref(null)
const ready = ref(false)
const showLogin = ref(false)
const loginTab = ref('login')
function openLogin(tab) {
  loginTab.value = tab === 'register' ? 'register' : 'login'
  showLogin.value = true
}
const isAdmin = ref(false)
const dataRev = ref(0)
const authed = computed(() => !!user.value)

const PLAN_LABEL = { lv1:'LV1 支持者', lv2:'LV2 常驻', lv3:'LV3 共建', c1:'C1 贡献者', c2:'C2 主力贡献', c3:'C3 共建贡献', c4:'C4 核心贡献' }

async function refreshMe() {
  if (!state.token) { ready.value = true; return }
  try {
    const me = await api.me()
    user.value = {
      name: me.username,
      plan: me.plan ? (PLAN_LABEL[me.plan] || me.plan) : '游客',
      points: me.quota.left_today,
      daily: me.quota.daily,
      bonus: me.quota.bonus,
    }
    isAdmin.value = !!me.is_admin
    ready.value = true
  } catch (e) {
    if (e.status === 401) { doLogout(); showLogin.value = true }
    ready.value = true
  }
}

function afterLogin(u) {
  showLogin.value = false
  dataRev.value++
  refreshMe()
  active.value = 'dashboard'
}
/* 弹层打开时锁住背后滚动（iOS 上否则会穿透） */
watch(showLogin, (v) => {
  document.body.style.overflow = v ? 'hidden' : ''
})
function signOut() {
  doLogout()
  user.value = null
  showLogin.value = true
}
const isDesktop = ref(true)

const pageMeta = {
  dashboard: { t: '运行概览', s: '账号池、模型与实时流量' },
  pricing: { t: '赞助与权益', s: '赞助档位 · 授权折算 · 两条路可叠加' },
  accounts: { t: '账号池', s: '国内外 WorkBuddy / CodeBuddy 授权账号' },
  invite: { t: '邀请返利', s: '邀请码 · 返点比例 · 已邀请名单' },
  lottery: { t: '抽奖', s: '每日免费 1 次 · 赠点只能消费' },
  keys: { t: 'API 密钥', s: '签发、轮换与额度' },
  usage: { t: '用量与账单', s: '按模型统计的调用与积分' },
  playground: { t: '对话测试', s: '真实请求走同一账号池' },
  docs: { t: '接口文档', s: 'OpenAI 兼容接入' },
  admin: { t: '管理台', s: '用户 · 订单 · 卡密 · 账号审核' },
}
const meta = computed(() => pageMeta[active.value] || pageMeta.dashboard)
const visibleItems = computed(() => (isAdmin.value ? items : items.filter(i => i.id !== 'admin')))

function go(id) { active.value = id }

/* ---------------- 滚动：顶栏隐藏 + 进度 ---------------- */
const shellEl = ref(null)
const scrollEl = ref(null)
const hidden = ref(false)
const progress = ref(0)
let lastY = 0, acc = 0, ticking = false

function onScroll() {
  const el = scrollEl.value
  // 桌面：内部滚动容器；移动端：整页滚动（body）
  const mobile = !isDesktop.value
  const y = mobile ? window.scrollY : (el ? el.scrollTop : 0)
  const max = mobile
    ? document.documentElement.scrollHeight - window.innerHeight
    : (el ? el.scrollHeight - el.clientHeight : 0)
  progress.value = max > 0 ? Math.min(1, y / max) : 0

  const dy = y - lastY
  acc += dy
  if (y < 60) { hidden.value = false; acc = 0 }
  else if (acc > 26) { hidden.value = true; acc = 0 }
  else if (acc < -18) { hidden.value = false; acc = 0 }
  lastY = y
  ticking = false
}

const onScrollThrottled = () => {
  if (!ticking) { ticking = true; requestAnimationFrame(onScroll) }
}

function attachScroll() {
  const el = scrollEl.value
  if (el) el.addEventListener('scroll', onScrollThrottled, { passive: true })
  // 移动端滚的是窗口，也要挂上
  window.addEventListener('scroll', onScrollThrottled, { passive: true })
}

/* ---------------- 命令面板 ---------------- */
const cmdk = ref(false)
const cmdkQ = ref('')
const cmdkResults = computed(() => {
  const q = cmdkQ.value.trim().toLowerCase()
  return items.filter(it => !q || it.label.toLowerCase().includes(q) || it.id.includes(q))
})
function runCmd(id) { cmdk.value = false; cmdkQ.value = ''; go(id) }

/* ---------------- 键盘 ---------------- */
function onKey(e) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault(); cmdk.value = !cmdk.value; cmdkQ.value = ''
  }
  if (e.key === 'Escape') { cmdk.value = false; sbOpen.value = false }
}

/* GitHub OAuth 回跳：?gh_ticket=… 换本站令牌；?gh_err=… 打开登录层并提示 */
async function handleGhRedirect() {
  const q = new URLSearchParams(location.search)
  const ticket = q.get('gh_ticket')
  const err = q.get('gh_err')
  if (!ticket && !err) return
  q.delete('gh_ticket')
  q.delete('gh_err')
  const qs = q.toString()
  try {
    history.replaceState(null, '', location.pathname + (qs ? '?' + qs : '') + location.hash)
  } catch (_) {}
  if (err) {
    window.__ghErr = err
    loginTab.value = 'login'
    showLogin.value = true
    return
  }
  try {
    const d = await api.ghExchange(ticket)
    setSession(d.token, d.user)
    active.value = 'dashboard'
  } catch (e) {
    window.__ghErr = 'expired'
    loginTab.value = 'login'
    showLogin.value = true
  }
}

onMounted(async () => {
  onAuthFail(() => { showLogin.value = true })
  window.addEventListener('wb2-need-login', () => { showLogin.value = true })
  window.addEventListener('wb2-refresh-me', () => { dataRev.value++; refreshMe() })
  // 供外部/调试直接切页
  window.__goto = go
  window.__openDrawer = () => { sbOpen.value = true }
  isDesktop.value = window.innerWidth >= 1024
  attachScroll()
  window.addEventListener('keydown', onKey)
  window.addEventListener('resize', () => { isDesktop.value = window.innerWidth >= 1024 }, { passive: true })
  await handleGhRedirect()
  refreshMe()
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

watch(active, () => {
  if (scrollEl.value) scrollEl.value.scrollTo({ top: 0, behavior: 'auto' })
  window.scrollTo({ top: 0, behavior: 'auto' })
  hidden.value = false
  lastY = 0
})
</script>

<template>
  <PerfSwitch />
  <!-- ============ 背景光场 ============ -->
  <div class="bg" aria-hidden="true">
    <div class="aurora"><i/><i/><i/></div>
    <div class="grid" />
    <div class="vignette" />
    <div class="noise" />
  </div>

  <!-- ============ 未登录：落地页 ============ -->
  <Landing v-if="ready && !authed" @login="openLogin" />

  <template v-else-if="authed">
  <GlassSidebar
    :open="sbOpen" :items="visibleItems" :active="active" :user="user"
    @close="sbOpen = false" @select="go" />

  <!-- ============ 外壳 ============ -->
  <div id="shell" ref="shellEl" class="shell" :class="{ mobile: !isDesktop, 'menu-open': sbOpen }">
    <!-- 顶栏 -->
    <header class="topbar" :class="{ hide: hidden && !cmdk }">
      <div class="tb-in">
        <button v-if="!isDesktop" class="burger" aria-label="打开导航"
                @click="sbOpen = true" :class="{ x: sbOpen }">
          <span/><span/><span/>
        </button>
        <div v-else class="tb-brand">
          <span class="tb-mark" />
          <span class="tb-name">WorkBuddy<b>2.0</b></span>
        </div>

        <div class="tb-title">
          <b>{{ meta.t }}</b>
          <span class="hide-sm">{{ meta.s }}</span>
        </div>

        <div class="tb-end">
          <button class="search" @click="cmdk = true" aria-label="搜索">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="7"/><path d="M16.5 16.5L21 21" stroke-linecap="round"/>
            </svg>
            <span class="hide-sm">搜索</span>
            <kbd class="hide-sm">⌘K</kbd>
          </button>
          <span class="pill ok hide-sm"><i class="live" />在线</span>
          <button class="avatar" aria-label="账号" @click="signOut" :title="user ? user.name + ' · 点击退出' : ''">
            {{ (user?.name || 'W')[0].toUpperCase() }}
          </button>
        </div>
      </div>
      <!-- 滚动进度 -->
      <div class="prog" :style="{ transform: `scaleX(${progress})` }" />
    </header>

    <!-- 页面 -->
    <main ref="scrollEl" class="scroll">
      <div class="page-wrap">
        <Transition name="pg" mode="out-in" :duration="{ enter: 420, leave: 180 }">
          <Dashboard v-if="active === 'dashboard'" :key="'d' + dataRev" @go="go" />
          <Pricing v-else-if="active === 'pricing'" key="pr" />
          <Invite v-else-if="active === 'invite'" key="iv" />
          <Lottery v-else-if="active === 'lottery'" :key="'lo' + dataRev" @need-login="showLogin = true" />
          <Accounts v-else-if="active === 'accounts'" :key="'a' + dataRev" />
          <Playground v-else-if="active === 'playground'" :key="'p' + dataRev" @need-login="showLogin = true" />
          <Keys v-else-if="active === 'keys'" :key="'k' + dataRev" @need-login="showLogin = true" />
          <Usage v-else-if="active === 'usage'" :key="'u' + dataRev" />
          <Admin v-else-if="active === 'admin'" :key="'ad' + dataRev" />
          <Docs v-else-if="active === 'docs'" key="dc" />
          <section v-else key="x" class="container" style="padding-top:34px">
            <div class="glass glass-2 empty-page">
              <p class="eyebrow">COMING NEXT</p>
              <h2>{{ meta.t }}</h2>
              <p class="sub">{{ meta.s }} — 这一页在下一批里做，先把外壳和动效定下来。</p>
            </div>
          </section>
        </Transition>
      </div>
    </main>
  </div>
  </template>

  <Login v-if="showLogin" :initial="loginTab" @done="afterLogin" @close="showLogin = false" />

  <!-- ============ 命令面板 ============ -->
  <Transition name="cmd">
    <div v-if="cmdk" class="cmd-scrim" @click.self="cmdk = false">
      <div class="cmd glass glass-3" role="dialog" aria-label="命令面板">
        <div class="cmd-in">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="7"/><path d="M16.5 16.5L21 21" stroke-linecap="round"/>
          </svg>
          <input v-model="cmdkQ" autofocus placeholder="跳转到…" @keydown.enter="cmdkResults[0] && runCmd(cmdkResults[0].id)" />
          <kbd>ESC</kbd>
        </div>
        <div class="cmd-list no-sb">
          <button v-for="(it, i) in cmdkResults" :key="it.id" class="cmd-it"
                  :style="{ '--i': i }" @click="runCmd(it.id)">
            <span class="ci" v-html="it.icon" />
            <span class="grow">{{ it.label }}</span>
            <span class="tiny dim mono">{{ it.id }}</span>
          </button>
          <p v-if="!cmdkResults.length" class="cmd-none">没有匹配项</p>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
/* ---------------- 背景 ---------------- */
.bg{position:fixed;inset:0;z-index:-1;overflow:hidden;background:var(--bg-0);
  transform:translateZ(0)}
.grid{
  position:absolute;inset:0;opacity:.16;
  background-image:
    linear-gradient(rgba(255,255,255,.055) 1px,transparent 1px),
    linear-gradient(90deg,rgba(255,255,255,.055) 1px,transparent 1px);
  background-size:56px 56px;
  mask-image:radial-gradient(90% 70% at 50% 0%,#000 20%,transparent 78%);
  -webkit-mask-image:radial-gradient(90% 70% at 50% 0%,#000 20%,transparent 78%);
}
.vignette{position:absolute;inset:0;
  background:radial-gradient(120% 90% at 50% 0%,transparent 30%,rgba(0,0,0,.55) 100%)}
.noise{position:absolute;inset:-10%;opacity:.30;background:var(--noise);
  animation:k-noise 9s steps(4,end) infinite;mix-blend-mode:overlay}

/* ---------------- 外壳 ---------------- */
.shell{
  min-height:100dvh;
  transition:transform 520ms var(--e-out),border-radius 520ms var(--e-out);
  transform-origin:50% 30%;
  /* ⚠️ will-change 会为后代 position:fixed 制造包含块（弹窗会定位到整页底部），
     只在抽屉推挤动画进行时开，平时必须保持 auto */
}
.shell.menu-open{will-change:transform}
@media (min-width:1024px){
  .shell{padding-left:calc(var(--sb-w,268px) + 24px)}
}
.topbar{
  position:sticky;top:0;z-index:60;
  transition:transform 340ms var(--e-out);
}
.topbar.hide{transform:translate3d(0,-100%,0)}
.tb-in{
  height:var(--nav-h);display:flex;align-items:center;gap:12px;
  padding:0 14px;
  background:linear-gradient(180deg,rgba(8,8,11,.78),rgba(8,8,11,.52));
  -webkit-backdrop-filter:blur(24px) saturate(175%);
  backdrop-filter:blur(24px) saturate(175%);
  border-bottom:1px solid var(--line);
}
@media (min-width:1024px){
  .tb-in{padding:0 22px;background:linear-gradient(180deg,rgba(8,8,11,.62),rgba(8,8,11,.34));
    border-bottom-color:rgba(255,255,255,.05)}
}
.prog{
  position:absolute;left:0;right:0;bottom:-1px;height:1.5px;
  background:linear-gradient(90deg,var(--ac),var(--ac-2));
  transform-origin:left center;
  transition:transform 90ms linear;
  opacity:.85;
}

/* 汉堡 → X */
.burger{
  width:36px;height:36px;flex:none;display:grid;place-content:center;gap:4px;
  border-radius:11px;background:rgba(255,255,255,.055);border:1px solid var(--line);
  transition:background-color 180ms,transform 260ms var(--e-out);
}
.burger:active{transform:scale(.93)}
.burger span{
  display:block;width:16px;height:1.6px;border-radius:2px;background:var(--tx-1);
  transition:transform 340ms var(--e-out),opacity 180ms,width 340ms var(--e-out);
}
.burger span:nth-child(2){width:12px;margin-left:-4px}
.burger.x span:nth-child(1){transform:translateY(5.6px) rotate(45deg)}
.burger.x span:nth-child(2){opacity:0;transform:translateX(-8px)}
.burger.x span:nth-child(3){transform:translateY(-5.6px) rotate(-45deg)}

.tb-brand{display:flex;align-items:center;gap:9px;flex:none}
.tb-mark{
  width:22px;height:22px;border-radius:50%;
  border:2.6px solid var(--ac);border-right-color:transparent;border-bottom-color:transparent;
  transform:rotate(-45deg);
}
.tb-name{font-size:14px;font-weight:700;letter-spacing:-.02em;display:flex;align-items:baseline;gap:5px}
.tb-name b{font-size:9.5px;color:var(--ac-2);font-weight:750;
  padding:1.5px 5px;border-radius:5px;background:var(--ac-dim);border:1px solid var(--ac-line)}

.tb-title{display:flex;flex-direction:column;min-width:0;line-height:1.2;margin-left:2px}
.tb-title b{font-size:13.5px;font-weight:640;letter-spacing:-.01em}
.tb-title span{font-size:11px;color:var(--tx-4);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
@media (max-width:1023px){ .tb-title{display:none} }

.tb-end{margin-left:auto;display:flex;align-items:center;gap:8px}
.search{
  display:flex;align-items:center;gap:7px;height:34px;padding:0 10px;
  border-radius:10px;border:1px solid var(--line);
  background:rgba(255,255,255,.045);color:var(--tx-3);font-size:12.5px;
  transition:background-color 180ms,border-color 180ms,color 180ms,transform 240ms var(--e-out);
}
.search:hover{background:rgba(255,255,255,.08);color:var(--tx-2);border-color:var(--line-2)}
.search:active{transform:scale(.97)}
.search kbd{height:19px;font-size:10px;color:var(--tx-4);background:rgba(255,255,255,.05)}
@media (max-width:767px){ .search{padding:0 8px;height:32px} }

.live{width:6px;height:6px;border-radius:50%;background:var(--ok);
  box-shadow:0 0 8px 1px rgba(84,201,138,.6);animation:k-breathe 2.4s var(--e-in-out) infinite}

.avatar{
  width:32px;height:32px;flex:none;border-radius:50%;display:grid;place-items:center;
  font-size:12px;font-weight:700;color:#ffe8d8;
  background:linear-gradient(160deg,rgba(255,138,61,.34),rgba(255,138,61,.13));
  border:1px solid var(--ac-line);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.24);
  transition:transform 240ms var(--e-out);
}
.avatar:active{transform:scale(.92)}

/* ---------------- 滚动区 ---------------- */
.scroll{
  height:calc(100dvh - var(--nav-h));
  overflow-y:auto;overflow-x:hidden;
  -webkit-overflow-scrolling:touch;
  overscroll-behavior-y:contain;
}
@media (max-width:1023px){ .scroll{height:auto;overflow:visible} .shell{min-height:100dvh} }
.page-wrap{padding:22px 0 calc(var(--safe-b) + 56px)}
@media (min-width:768px){ .page-wrap{padding-top:28px} }

/* 页面转场：位移 + 轻缩放 + 模糊，420ms */
.pg-enter-active{animation:pg-in 420ms var(--e-out) both}
.pg-leave-active{animation:pg-out 180ms var(--e-in-out) both}
@keyframes pg-in{
  0%{opacity:0;transform:translate3d(0,12px,0) scale(.992);filter:blur(5px)}
  100%{opacity:1;transform:none;filter:blur(0)}
}
@keyframes pg-out{
  0%{opacity:1;transform:none;filter:blur(0)}
  100%{opacity:0;transform:translate3d(0,-6px,0) scale(.997);filter:blur(3px)}
}

.empty-page{padding:36px 30px;display:flex;flex-direction:column;gap:10px}
.gate{display:grid;place-items:center;min-height:min(62vh,480px);padding:20px}
.gate-card{padding:30px 26px;display:flex;flex-direction:column;align-items:center;gap:11px;
  text-align:center;max-width:340px;animation:k-fade-up 520ms var(--e-out) both}
.gate-mark{color:var(--ac);display:grid;place-items:center;
  filter:drop-shadow(0 0 16px rgba(255,138,61,.32))}
.gate-card h2{font-size:18px}
.gate-card .sub{line-height:1.7;max-width:30ch}
.gate-card .btn{margin-top:5px}
.gate-spin{width:20px;height:20px;border-radius:50%;margin-top:8px;
  border:2px solid rgba(255,255,255,.16);border-top-color:var(--ac);
  animation:spin 800ms linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}

/* ---------------- 命令面板 ---------------- */
.cmd-scrim{
  position:fixed;inset:0;z-index:200;
  background:rgba(0,0,0,.5);
  -webkit-backdrop-filter:blur(6px);backdrop-filter:blur(6px);
  display:flex;align-items:flex-start;justify-content:center;
  padding:14vh 16px 16px;
}
.cmd{
  width:100%;max-width:520px;padding:6px;overflow:hidden;
  box-shadow:inset 0 1px 0 rgba(255,255,255,.22),var(--sh-xl);
}
.cmd-in{display:flex;align-items:center;gap:10px;padding:10px 12px;border-bottom:1px solid var(--line)}
.cmd-in svg{color:var(--tx-4);flex:none}
.cmd-in input{
  flex:1;height:26px;border:0;background:none;outline:none;font-size:14px;color:var(--tx-1);
}
.cmd-in input::placeholder{color:var(--tx-4)}
.cmd-list{max-height:46vh;overflow-y:auto;padding:6px}
.cmd-it{
  display:flex;align-items:center;gap:11px;width:100%;height:40px;padding:0 10px;
  border-radius:10px;font-size:13.5px;color:var(--tx-2);
  animation:k-fade-up 320ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 26ms);
  transition:background-color 150ms,color 150ms;
}
.cmd-it:hover,.cmd-it:focus-visible{background:rgba(255,255,255,.07);color:var(--tx-1)}
.ci{display:grid;place-items:center;width:17px;height:17px;opacity:.8}
.ci :deep(svg){width:15px;height:15px;stroke:currentColor;fill:none;stroke-width:1.7;
  stroke-linecap:round;stroke-linejoin:round}
.cmd-none{padding:18px;text-align:center;color:var(--tx-4);font-size:13px}

.cmd-enter-active{animation:k-fade-in 200ms var(--e-out) both}
.cmd-leave-active{animation:k-fade-in 140ms var(--e-in-out) reverse both}
.cmd-enter-active .cmd{animation:k-scale-spring 460ms cubic-bezier(.34,1.38,.64,1) both}
</style>

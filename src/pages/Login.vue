<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { api, setSession } from '../lib/api.js'

const emit = defineEmits(['done', 'close'])
const props = defineProps({ initial: { type: String, default: 'login' } })

/* ---------- 视图状态 ----------
   tab  : login | register        顶部两个大分栏
   way  : password | email     登录方式（仅 tab=login 时）
   view : auth | forgot        忘记密码单独一屏
--------------------------------- */
const tab = ref(props.initial === 'register' ? 'register' : 'login')
const way = ref('password')
const view = ref('auth')

const loading = ref(false)
const sending = ref(false)
const msg = ref('')
const msgType = ref('err')
const shakeKey = ref(0)

const inviteFromUrl = (new URLSearchParams(location.search).get('invite') || '').trim().slice(0, 16)
const form = ref({ username: '', password: '', email: '', code: '', invite: inviteFromUrl })

const emailOk = (v) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(String(v || '').trim())

const canSubmit = computed(() => {
  if (view.value === 'forgot') return emailOk(form.value.email) && form.value.code.length === 6 && form.value.password.length >= 6
  if (tab.value === 'login') {
    if (way.value === 'email') return emailOk(form.value.email) && form.value.code.length === 6
    return !!form.value.username && !!form.value.password
  }
  return emailOk(form.value.email) && form.value.code.length === 6 &&
    form.value.username.length >= 3 && form.value.password.length >= 6
})

/* ---------- 验证码倒计时 ---------- */
const cd = ref(0)
let timer = null
function startCountdown(sec = 60) {
  cd.value = sec
  clearInterval(timer)
  timer = setInterval(() => {
    cd.value -= 1
    if (cd.value <= 0) { clearInterval(timer); cd.value = 0 }
  }, 1000)
}
onUnmounted(() => clearInterval(timer))

const codePurpose = computed(() => {
  if (view.value === 'forgot') return 'reset'
  return tab.value === 'login' ? 'login' : 'register'
})

/* 切换用途（注册↔登录↔重置）时清掉倒计时与已填验证码，
   否则会被上一屏的冷却卡住，或者复用错用途的验证码 */
watch(codePurpose, () => {
  clearInterval(timer)
  cd.value = 0
  form.value.code = ''
  msg.value = ''
})

async function sendCode() {
  if (sending.value || cd.value > 0) return
  if (!emailOk(form.value.email)) { notice('请先填写正确的邮箱'); return }
  sending.value = true
  msg.value = ''
  try {
    const d = await api.sendCode(form.value.email.trim(), codePurpose.value)
    startCountdown(d.resend_after || 60)
    if (d.temp_mail && codePurpose.value === 'register') {
      notice('临时邮箱可以注册，但不发放注册礼包', 'ok')
    } else {
      notice(`验证码已发送至 ${d.email}，10 分钟内有效`, 'ok')
    }
  } catch (e) {
    notice(e.message || '发送失败')
  } finally {
    sending.value = false
  }
}

const notice = (t, k = 'err') => { msg.value = t; msgType.value = k }
const fail = (t) => { notice(t); shakeKey.value++ }

const switchTab = (t) => {
  tab.value = t
  view.value = 'auth'
  msg.value = ''
  form.value.code = ''
  if (t === 'register') way.value = 'password'
}

const switchWay = (w) => { way.value = w; msg.value = ''; form.value.code = '' }

/* ---------- 提交 ---------- */
async function submit() {
  if (!canSubmit.value || loading.value) return
  loading.value = true
  msg.value = ''
  try {
    let d
    if (view.value === 'forgot') {
      d = await api.resetPassword(form.value.email.trim(), form.value.code, form.value.password)
      done(d, '密码已重置，正在进入控制台…')
      return
    }
    if (tab.value === 'login') {
      d = way.value === 'email'
        ? await api.emailLogin(form.value.email.trim(), form.value.code)
        : await api.login({ username: form.value.username, password: form.value.password })
    } else {
      d = await api.register({
        username: form.value.username,
        password: form.value.password,
        email: form.value.email.trim(),
        code: form.value.code,
        invite: form.value.invite,
      })
    }
    done(d)
  } catch (e) {
    fail(e.message || '操作失败')
  } finally {
    loading.value = false
  }
}

function done(d, tip) {
  setSession(d.token, d.user)
  if (d.notice) {
    notice(d.notice, 'ok')
    setTimeout(() => emit('done', d.user), 1700)
  } else if (tip) {
    notice(tip, 'ok')
    setTimeout(() => emit('done', d.user), 700)
  } else {
    emit('done', d.user)
  }
}

/* 自动把邮箱前缀填成用户名（用户还可以改） */
function fillUsername() {
  if (tab.value !== 'register' || form.value.username) return
  const p = String(form.value.email || '').split('@')[0].replace(/[^a-zA-Z0-9_]/g, '')
  if (p.length >= 3) form.value.username = p.slice(0, 24)
}

/* ---------- 侧栏信息 ---------- */
const host = ref((window.location && window.location.host) || 'localhost:8078')
const stats = ref(null)
onMounted(async () => {
  try { stats.value = await api.status() } catch (_) {}
})
const modelCount = computed(() => stats.value?.model_count || 0)
const poolOk = computed(() => stats.value?.upstream?.ok)

/* ---------- GitHub 一键登录 / 注册 ---------- */
const ghEnabled = computed(() => !!stats.value?.github_login)
const ghBusy = ref(false)

const GH_ERR = {
  denied: '已取消 GitHub 授权',
  state: '授权状态校验失败，请重新点击',
  token: '换取 GitHub 凭据失败，请重试一次',
  user: '读取 GitHub 账号失败，请重试',
  banned: '该 GitHub 对应的本站账号已被停用',
  email_taken: '该邮箱已绑定其他 GitHub 账号',
  create: '创建账号失败，请稍后再试',
  expired: '登录链接已过期，请重新点击',
  nocode: 'GitHub 未返回授权码',
}

async function ghLogin() {
  if (ghBusy.value) return
  ghBusy.value = true
  msg.value = ''
  try {
    const d = await api.ghStart(form.value.invite || '')
    if (!d || !d.url) throw new Error('GitHub 登录暂不可用')
    location.href = d.url
  } catch (e) {
    ghBusy.value = false
    fail(e.message || 'GitHub 登录失败')
  }
}

/* App.vue 处理回跳失败时把原因塞进 window.__ghErr，这里提示一次 */
onMounted(() => {
  const k = window.__ghErr
  if (k) {
    delete window.__ghErr
    notice(GH_ERR[k] || 'GitHub 登录失败，请重试')
  }
})
</script>

<template>
  <div class="auth">
    <button class="lg-close" aria-label="关闭" @click="emit('close')">
      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor"
           stroke-width="2.1" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
    </button>

    <div class="lg-bg" aria-hidden="true">
      <div class="aurora"><i /><i /><i /></div>
      <div class="grid" />
    </div>

    <div class="lg-wrap">
      <!-- 左：文案 -->
      <section class="lg-left">
        <div class="brand">
          <span class="mark">
            <svg viewBox="0 0 32 32" width="28" height="28" fill="none">
              <circle cx="16" cy="16" r="10.5" stroke="currentColor" stroke-width="2.6"
                      stroke-linecap="round" stroke-dasharray="46 20" transform="rotate(-45 16 16)" />
              <circle cx="16" cy="16" r="3.1" fill="currentColor" />
            </svg>
          </span>
          <span class="bname">WorkBuddy <b>2.0</b></span>
        </div>

        <h1>一个接口<br />通向整个账号池。</h1>
        <p class="lead">
          群友授权的 WorkBuddy / CodeBuddy 账号在这里汇成池子。
          你用标准的 OpenAI 协议调用，路由、冷却、轮换都在服务端完成。
        </p>

        <div class="facts">
          <div class="fact">
            <b>{{ modelCount || '—' }}</b>
            <span>可用模型</span>
          </div>
          <div class="fact">
            <b class="ok">{{ poolOk ? '在线' : '待连接' }}</b>
            <span>账号池</span>
          </div>
          <div class="fact">
            <b>4 点</b>
            <span>一次对话约</span>
          </div>
        </div>

        <div class="keys">
          <div class="krow"><span>Endpoint</span><code>{{ host }}/v1</code></div>
          <div class="krow"><span>注册礼</span><code>3000 点 · 7 天</code></div>
          <div class="krow"><span>登录</span><code>{{ ghEnabled ? '密码 / 验证码 / GitHub' : '密码 / 邮箱验证码' }}</code></div>
        </div>
      </section>

      <!-- 右：表单 -->
      <section class="lg-card glass glass-3">
        <!-- 忘记密码 -->
        <template v-if="view === 'forgot'">
          <button class="ec-back" @click="view = 'auth'; msg = ''">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 5l-7 7 7 7"/></svg>
            返回登录
          </button>
          <h2 class="ec-title">重置密码</h2>
          <p class="ec-sub">用注册邮箱收验证码，验证通过后直接设置新密码。</p>
        </template>

        <!-- 正常登录 / 注册 -->
        <template v-else>
          <div class="seg">
            <span class="seg-ink" :style="{ transform: `translateX(${tab === 'login' ? 0 : 100}%)` }" />
            <button :data-on="tab === 'login' ? 1 : 0" @click="switchTab('login')">登录</button>
            <button :data-on="tab === 'register' ? 1 : 0" @click="switchTab('register')">注册</button>
          </div>

          <!-- 登录方式切换 -->
          <div v-if="tab === 'login'" class="way">
            <button :data-on="way === 'password' ? 1 : 0" @click="switchWay('password')">密码登录</button>
            <button :data-on="way === 'email' ? 1 : 0" @click="switchWay('email')">邮箱验证码</button>
          </div>
        </template>

        <form class="fields" @submit.prevent="submit">
          <!-- GitHub 一键登录 / 注册（后端配好 Secret 才出现） -->
          <template v-if="view === 'auth' && ghEnabled">
            <button type="button" class="gh-btn" :disabled="ghBusy" @click="ghLogin">
              <svg viewBox="0 0 16 16" width="17" height="17" aria-hidden="true">
                <path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.4 7.4 0 012-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z" />
              </svg>
              <span>{{ ghBusy ? '正在前往 GitHub…' : '使用 GitHub 继续' }}</span>
            </button>
            <div class="gh-sep" aria-hidden="true"><i /><span>或</span><i /></div>
          </template>

          <!-- 用户名 / 密码（邮箱验证码登录不需要） -->
          <template v-if="view === 'auth' && !(tab === 'login' && way === 'email')">
            <div class="field">
              <label>用户名</label>
              <input v-model.trim="form.username" class="input" autocomplete="username"
                     placeholder="3–24 个字符" />
            </div>
            <div class="field">
              <label>
                密码
                <button v-if="tab === 'login'" type="button" class="ec-link"
                        @click="view = 'forgot'; msg = ''; form.code = ''">忘记密码？</button>
              </label>
              <input v-model="form.password" type="password" class="input"
                     :autocomplete="tab === 'login' ? 'current-password' : 'new-password'"
                     placeholder="至少 6 位" />
            </div>
          </template>

          <!-- 邮箱 + 验证码（注册 / 邮箱登录 / 重置密码都需要） -->
          <template v-if="view === 'forgot' || tab === 'register' || way === 'email'">
            <div class="field">
              <label>
                邮箱
                <i v-if="tab === 'register' && view === 'auth'" class="opt">注册需验证</i>
              </label>
              <input v-model.trim="form.email" type="email" class="input" autocomplete="email"
                     placeholder="you@example.com" @blur="fillUsername" />
            </div>
            <div class="field">
              <label>邮箱验证码</label>
              <div class="ec-row">
                <input v-model.trim="form.code" class="input" inputmode="numeric" maxlength="6"
                       autocomplete="one-time-code" placeholder="6 位数字" />
                <button type="button" class="ec-send" :disabled="sending || cd > 0"
                        :data-wait="cd > 0 ? 1 : 0" @click="sendCode">
                  {{ sending ? '发送中…' : cd > 0 ? `${cd}s 后重发` : '获取验证码' }}
                </button>
              </div>
            </div>
          </template>

          <!-- 重置密码：新密码 -->
          <div v-if="view === 'forgot'" class="field">
            <label>新密码</label>
            <input v-model="form.password" type="password" class="input" autocomplete="new-password"
                   placeholder="至少 6 位" />
          </div>

          <!-- 注册时的邀请码 -->
          <div v-if="view === 'auth' && tab === 'register'" class="field">
            <label>邀请码 <i class="opt">选填 · 双方各得 500 点</i></label>
            <input v-model.trim="form.invite" class="input" placeholder="群友的邀请码" />
          </div>

          <Transition name="msg">
            <p v-if="msg" class="err-msg" :key="shakeKey" :class="[msgType === 'ok' ? 'is-ok' : '', shakeKey > 0 ? 'shake' : '']">{{ msg }}</p>
          </Transition>

          <button type="submit" class="btn pri lg submit" :disabled="!canSubmit || loading">
            <span v-if="loading" class="spin" />
            {{ loading ? '处理中…'
              : view === 'forgot' ? '重置并登录'
              : tab === 'login' ? (way === 'email' ? '验证码登录' : '进入控制台')
              : '创建账号' }}
          </button>

          <p class="tiny dim terms">
            {{ view === 'forgot'
              ? '验证码 10 分钟内有效；重置成功后会用新密码自动登录。'
              : tab === 'register'
                ? '注册即送 3000 算力点（7 天有效）；使用临时邮箱注册不发放礼包与邀请奖励。'
                : '本站为公益性质，请勿转卖或共享密钥。' }}
          </p>
        </form>
      </section>
    </div>
  </div>
</template>

<style scoped>
.auth{position:fixed;inset:0;z-index:300;overflow-y:auto;background:var(--bg-0);
  -webkit-overflow-scrolling:touch;overscroll-behavior:contain}
.lg-close{
  position:fixed;z-index:3;top:calc(13px + var(--safe-t));right:calc(13px + var(--pad));
  width:36px;height:36px;border-radius:50%;display:grid;place-items:center;
  color:var(--tx-2);background:rgba(255,255,255,.06);border:1px solid var(--line-2);
  -webkit-backdrop-filter:blur(16px) saturate(160%);backdrop-filter:blur(16px) saturate(160%);
  transition:transform 240ms var(--e-out),background-color 180ms,color 180ms;
}
.lg-close:hover{color:var(--tx-1);background:rgba(255,255,255,.11)}
.lg-close:active{transform:scale(.92)}
.lg-bg{position:fixed;inset:0;pointer-events:none;overflow:hidden}
.grid{
  position:absolute;inset:0;opacity:.14;
  background-image:
    linear-gradient(rgba(255,255,255,.05) 1px,transparent 1px),
    linear-gradient(90deg,rgba(255,255,255,.05) 1px,transparent 1px);
  background-size:54px 54px;
  mask-image:radial-gradient(80% 60% at 50% 20%,#000,transparent 76%);
  -webkit-mask-image:radial-gradient(80% 60% at 50% 20%,#000,transparent 76%);
}
.lg-wrap{
  position:relative;z-index:1;min-height:100%;
  max-width:1080px;margin:0 auto;padding:clamp(24px,6vh,80px) var(--pad) 48px;
  display:grid;grid-template-columns:1.05fr .95fr;gap:clamp(28px,5vw,64px);align-items:center;
}
@media (max-width:860px){
  .lg-wrap{grid-template-columns:1fr;gap:16px;align-content:start;
    padding:calc(var(--safe-t) + 20px) var(--pad) calc(var(--safe-b) + 40px);
    align-items:start}
  .brand{margin-bottom:0}
  .lg-left h1,.lg-left .lead,.lg-left .facts,.lg-left .keys{display:none}
  .lg-card{padding:16px;border-radius:24px;animation-duration:460ms}
  .seg{margin-bottom:14px}
  .input{height:46px}
  .submit{height:48px}
}

/* 左 */
.brand{display:flex;align-items:center;gap:10px;margin-bottom:26px;
  animation:k-fade-up 600ms var(--e-out) both}
.mark{color:var(--ac);display:grid;place-items:center;filter:drop-shadow(0 0 16px rgba(255,138,61,.3))}
.bname{font-size:15px;font-weight:700;letter-spacing:-.02em;display:flex;align-items:center;gap:6px}
.bname b{font-size:9.5px;color:var(--ac-2);font-weight:750;padding:2px 6px;border-radius:5px;
  background:var(--ac-dim);border:1px solid var(--ac-line)}

.lg-left h1{font-size:clamp(30px,4.6vw,46px);line-height:1.12;letter-spacing:-.04em;
  animation:k-fade-up 640ms 60ms var(--e-out) both}
.lead{margin-top:16px;max-width:44ch;color:var(--tx-3);font-size:14px;line-height:1.8;
  animation:k-fade-up 640ms 120ms var(--e-out) both}

.facts{display:flex;gap:26px;margin-top:26px;
  animation:k-fade-up 640ms 180ms var(--e-out) both}
.fact{display:flex;flex-direction:column;gap:2px}
.fact b{font-size:20px;font-weight:720;letter-spacing:-.03em;font-variant-numeric:tabular-nums}
.fact b.ok{color:#8fe8b6;font-size:17px}
.fact span{font-size:10.5px;color:var(--tx-4)}

.keys{display:flex;flex-direction:column;gap:6px;margin-top:28px;
  animation:k-fade-up 640ms 240ms var(--e-out) both}
.krow{display:flex;align-items:center;gap:10px;font-size:11px;color:var(--tx-4)}
.krow span{width:64px;flex:none}
.krow code{font-size:10.5px;color:var(--ac-2);background:var(--ac-dim);
  padding:2px 7px;border-radius:5px;border:1px solid rgba(255,138,61,.18)}

/* 右 */
.lg-card{
  padding:20px;border-radius:var(--r-xl);
  animation:k-scale-spring 680ms 120ms cubic-bezier(.34,1.3,.64,1) both;
}
.seg{
  position:relative;display:flex;padding:3px;border-radius:11px;margin-bottom:14px;
  background:rgba(255,255,255,.028);border:1px solid var(--line);
}
.seg-ink{
  position:absolute;top:3px;left:3px;height:calc(100% - 6px);width:calc((100% - 6px) / 2);
  border-radius:8px;
  background:linear-gradient(160deg,rgba(255,255,255,.095),rgba(255,255,255,.038));
  border:1px solid rgba(255,255,255,.10);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.10);
  transition:transform 420ms var(--e-out);
}
.seg button{
  position:relative;z-index:1;flex:1;height:34px;border-radius:8px;
  font-size:13px;font-weight:620;color:var(--tx-3);
  transition:color 240ms;
}
.seg button[data-on="1"]{color:var(--tx-1)}

/* 登录方式小胶囊 */
.way{display:flex;gap:6px;margin:0 0 14px}
.way button{
  height:27px;padding:0 11px;border-radius:999px;font-size:11.5px;font-weight:600;
  color:var(--tx-3);background:rgba(255,255,255,.03);border:1px solid var(--line);
  transition:color 200ms,background-color 200ms,border-color 200ms,transform 200ms var(--e-out);
}
.way button:hover{color:var(--tx-2);background:rgba(255,255,255,.055)}
.way button:active{transform:scale(.96)}
.way button[data-on="1"]{
  color:var(--ac-2);background:var(--ac-dim);border-color:var(--ac-line);
}

/* 忘记密码入口 */
.ec-link{
  float:right;font-size:11px;font-weight:500;color:var(--tx-4);
  transition:color 180ms;
}
.ec-link:hover{color:var(--ac-2)}

.ec-back{
  display:inline-flex;align-items:center;gap:5px;height:28px;padding:0 10px 0 7px;
  margin-bottom:12px;border-radius:999px;font-size:11.5px;font-weight:600;color:var(--tx-3);
  background:rgba(255,255,255,.035);border:1px solid var(--line);
  transition:color 180ms,background-color 180ms,transform 200ms var(--e-out);
}
.ec-back:hover{color:var(--tx-1);background:rgba(255,255,255,.065)}
.ec-back:active{transform:scale(.96)}
.ec-title{font-size:18px;font-weight:700;letter-spacing:-.02em}
.ec-sub{margin:6px 0 16px;font-size:12px;line-height:1.7;color:var(--tx-4)}

.fields{display:flex;flex-direction:column;gap:13px}
.opt{font-style:normal;font-weight:500;color:var(--tx-4);margin-left:5px;font-size:10.5px}
.submit{margin-top:5px;width:100%;height:46px}

/* GitHub 一键登录 / 注册 */
.gh-btn{display:flex;align-items:center;justify-content:center;gap:9px;width:100%;height:44px;
  border-radius:12px;border:1px solid var(--line-2);background:rgba(255,255,255,.045);
  color:var(--tx-1);font-size:14px;font-weight:520;letter-spacing:-.01em;cursor:pointer;
  transition:background-color .18s,border-color .18s,transform .12s var(--e-out)}
.gh-btn:hover{background:rgba(255,255,255,.075);border-color:rgba(255,255,255,.18)}
.gh-btn:active{transform:scale(.985)}
.gh-btn:disabled{opacity:.55;cursor:default;transform:none}
.gh-btn svg{flex:none;opacity:.92}
.gh-sep{display:flex;align-items:center;gap:10px;color:var(--tx-4);font-size:11px;letter-spacing:.1em}
.gh-sep i{flex:1;height:1px;background:var(--line-2)}
.terms{line-height:1.6;margin-top:2px}

/* 邮箱 + 获取验证码 */
.ec-row{display:flex;gap:8px;align-items:center}
.ec-row .input{flex:1;min-width:0}
.ec-send{
  flex:none;height:42px;padding:0 13px;border-radius:11px;
  font-size:12px;font-weight:640;letter-spacing:-.01em;white-space:nowrap;
  color:var(--ac-2);background:var(--ac-dim);border:1px solid var(--ac-line);
  transition:transform 220ms var(--e-out),background-color 180ms,color 180ms,opacity 180ms;
}
.ec-send:hover:not(:disabled){background:rgba(255,138,61,.16)}
.ec-send:active:not(:disabled){transform:scale(.96)}
.ec-send:disabled{opacity:.5;cursor:default}
.ec-send[data-wait="1"]{color:var(--tx-4);background:rgba(255,255,255,.035);border-color:var(--line)}
@media (max-width:860px){ .ec-send{height:46px} }

.err-msg{
  font-size:12px;color:#f6b1ac;background:rgba(240,103,95,.10);
  border:1px solid rgba(240,103,95,.22);border-radius:9px;padding:8px 11px;
  line-height:1.6;
}
.err-msg.is-ok{color:#a9e8c4;background:rgba(96,200,150,.09);border-color:rgba(120,220,170,.22)}
.err-msg.shake{animation:k-shake 460ms var(--e-in-out)}
.msg-enter-active{animation:k-fade-up 260ms var(--e-out) both}
.msg-leave-active{animation:k-fade-in 140ms reverse both}

.spin{
  width:13px;height:13px;border-radius:50%;flex:none;
  border:1.8px solid rgba(255,255,255,.3);border-top-color:#fff;
  animation:spin .7s linear infinite;
}
@keyframes spin{to{transform:rotate(360deg)}}
</style>

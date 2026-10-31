<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { api, fmt } from '../lib/api.js'

const realm = ref('intl')
const accounts = ref([])
const ratio = ref(0)
const tier = ref('')
const plan = ref({ contrib: '', contribDaily: 0, okAccounts: 0, sponsor: '', sponsorUntil: '', daily: 0, stacked: false })
const loading = ref(true)
const err = ref('')
const okMsg = ref('')
const rawInput = ref('')
const label = ref('')
const submitting = ref(false)
const showForm = ref(false)
const showPaste = ref(false)
const oauthState = ref('')
const oauthBusy = ref(false)
const oauthMsg = ref('')
let pollTimer = null

const REALM_LABEL = { intl: '国际版 WorkBuddy', cn: '国内版 CodeBuddy' }
const STATE_LABEL = { pending: '待审核', ok: '可用', cool: '冷却', rejected: '未通过', revoked: '已撤回' }
const STATE_PILL = { pending: 'warn', ok: 'ok', cool: 'warn', rejected: 'err', revoked: 'err' }

async function load() {
  loading.value = true
  try {
    const d = await api.accounts()
    accounts.value = (d.accounts || []).map(a => ({
      id: a.id,
      name: a.label || '账号',
      realmLabel: REALM_LABEL[a.realm] || a.realm,
      uid: (a.uid_ref || '').slice(0, 14) || '—',
      credits: a.credits || 0,
      lat: a.latency_ms || 0,
      state: a.state,
      note: a.note,
    }))
    ratio.value = d.ratio || 0
    tier.value = d.tier || ''
    plan.value = {
      contrib: d.contrib_plan || '',
      contribDaily: d.contrib_daily || 0,
      okAccounts: d.ok_accounts || 0,
      sponsor: d.sponsor_plan || '',
      sponsorUntil: d.sponsor_until || '',
      daily: d.daily || 0,
      stacked: !!d.stacked,
    }
    err.value = ''
  } catch (e) {
    err.value = e.message
  } finally { loading.value = false }
}
onMounted(load)
onBeforeUnmount(stopPoll)

function stopPoll() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

async function startOAuth() {
  if (oauthBusy.value) return
  err.value = ''
  okMsg.value = ''
  oauthMsg.value = ''
  oauthBusy.value = true
  try {
    const d = await api.oauthStart(realm.value)
    oauthState.value = d.state
    oauthMsg.value = '已打开授权页，请在新标签里登录并完成扫码/验证码…'
    if (d.authUrl) window.open(d.authUrl, '_blank', 'noopener')
    stopPoll()
    let tries = 0
    pollTimer = setInterval(async () => {
      tries += 1
      if (tries > 120) { stopPoll(); oauthBusy.value = false; oauthMsg.value = '授权超时，重新点一次按钮'; return }
      try {
        const r = await api.oauthPoll(oauthState.value)
        if (r.status === 'ok') {
          stopPoll()
          oauthBusy.value = false
          okMsg.value = '授权成功，账号已接入池子' + (r.nickname ? '：' + r.nickname : '')
          oauthMsg.value = ''
          await load()
        } else if (r.status === 'rejected') {
          stopPoll()
          oauthBusy.value = false
          oauthMsg.value = ''
          err.value = r.message || '账号自检没通过，已打回'
          await load()
        } else if (r.status === 'pending') {
          oauthMsg.value = '等待上游确认…（' + (r.message || '请在新标签完成登录') + '）'
        } else {
          stopPoll(); oauthBusy.value = false
          oauthMsg.value = r.message || ('授权未完成：' + r.status)
        }
      } catch (e) {
        stopPoll(); oauthBusy.value = false; err.value = e.message
      }
    }, 2500)
  } catch (e) {
    oauthBusy.value = false
    err.value = e.message
  }
}

async function submit() {
  if (submitting.value) return
  if (!rawInput.value.trim()) { err.value = '请粘贴账号凭据 JSON'; return }
  submitting.value = true
  err.value = ''
  okMsg.value = ''
  try {
    await api.submitAccount(realm.value, label.value.trim(), rawInput.value.trim())
    okMsg.value = '已提交，管理员审核通过后开始折算额度'
    rawInput.value = ''
    label.value = ''
    showForm.value = false
    await load()
  } catch (e) {
    err.value = e.message
  } finally { submitting.value = false }
}

async function remove(id) {
  if (!confirm('撤回这个账号？之后不再参与折算。')) return
  const el = document.querySelector(`[data-acc="${id}"]`)
  if (el) el.animate(
    [{ opacity: 1, transform: 'scale(1)' }, { opacity: 0, transform: 'scale(.94)' }],
    { duration: 240, easing: 'cubic-bezier(.65,.05,.36,1)', fill: 'forwards' }
  )
  try {
    await api.deleteAccount(id)
    setTimeout(load, 240)
  } catch (e) {
    err.value = e.message
    load()
  }
}

const total = computed(() => accounts.value.reduce((a, b) => a + (b.credits || 0), 0))
const healthy = computed(() => accounts.value.filter(a => a.state === 'ok').length)
const pending = computed(() => accounts.value.filter(a => a.state === 'pending').length)

const TIER_HINT = {
  '': '还没有贡献档位',
  c1: 'C1 · 折算 30%',
  c2: 'C2 · 折算 45%',
  c3: 'C3 · 折算 55%',
  c4: 'C4 · 折算 60%',
  lv1: '当前是赞助档，再交账号可叠加 1.5 倍',
  lv2: '当前是赞助档，再交账号可叠加 1.5 倍',
  lv3: '当前是赞助档，再交账号可叠加 1.5 倍',
}

/* 授权页地址：点一下直接打开，登录完复制 JSON 回来提交 */
const AUTH_URL = 'https://www.workbuddy.ai/'
function openAuth() {
  window.open(AUTH_URL, '_blank', 'noopener')
}
</script>

<template>
  <section class="acc container">
    <header class="ahead">
      <div>
        <p class="eyebrow">ACCOUNTS</p>
        <h1>授权账号</h1>
        <p class="sub" style="margin-top:8px">
          交一个账号，按余额折算成你的额度。账号随时可撤回，对话内容不落库。
        </p>
      </div>
      <div class="aglance glass glass-1">
        <div><b>{{ accounts.length }}</b><span>账号</span></div>
        <i />
        <div><b>{{ healthy }}</b><span>可用</span></div>
        <i />
        <div><b>{{ fmt.pts(total) }}</b><span>积分</span></div>
      </div>
    </header>

    <!-- 当前折算 -->
    <article class="ratio glass glass-2">
      <div class="r-left">
        <p class="eyebrow">CONTRIBUTION</p>
        <h3>
          <template v-if="plan.contrib">
            贡献档 {{ plan.contrib.toUpperCase() }} · {{ plan.okAccounts }} 个有效号 · {{ fmt.pts(plan.contribDaily) }} 点/日
          </template>
          <template v-else>还没有贡献档：交 1 个号就有 C1</template>
        </h3>
        <p class="tiny dim">
          <template v-if="plan.sponsor">
            赞助档 {{ plan.sponsor.toUpperCase() }}<template v-if="plan.sponsorUntil">（到 {{ String(plan.sponsorUntil).slice(0, 10) }}）</template>
          </template>
          <template v-else>未购买赞助档</template>
          —— 今日额度合计 <b>{{ fmt.pts(plan.daily) }}</b> 点/日<template v-if="plan.stacked">（贡献 + 赞助 叠加 1.5 倍）</template>
        </p>
      </div>
      <div class="r-right">
        <span class="pill" v-if="pending">{{ pending }} 个待审核</span>
        <button class="btn pri lg" @click="showForm = !showForm">
          {{ showForm ? '收起' : '＋ 提交账号' }}
        </button>
      </div>
    </article>

    <!-- 提交表单 -->
    <Transition name="form">
      <article v-if="showForm" class="form glass glass-2">
        <div class="authrow">
          <div class="realm-sw oauth-realm">
            <button :data-on="realm === 'intl' ? 1 : 0" :disabled="oauthBusy" @click="realm = 'intl'">国际版 WorkBuddy</button>
            <button :data-on="realm === 'cn' ? 1 : 0" :disabled="oauthBusy" @click="realm = 'cn'">国内版 CodeBuddy</button>
          </div>
          <button class="btn pri lg" :disabled="oauthBusy" @click="startOAuth">
            {{ oauthBusy ? '等待授权完成…' : 'OAuth 一键授权' }}
          </button>
        </div>
        <p class="tiny dim authhint">
          {{ oauthMsg || '先选国内/国际版，再点授权 → 新标签里登录 → 后台自动自检，能跑才接进池子（跑不通直接打回）' }}
        </p>
        <button v-if="!oauthBusy" class="linkbtn" @click="showPaste = !showPaste">
          {{ showPaste ? '收起手动粘贴' : '授权页打不开？手动粘贴账号 JSON' }}
        </button>
        <template v-if="showPaste">

        <div class="f-row">
          <input v-model.trim="label" class="input lbl-in" placeholder="备注名（选填，例如：我的小号）" />
        </div>

        <div class="field">
          <label>账号凭据 JSON</label>
          <textarea v-model="rawInput" class="input ta mono" rows="6"
                    placeholder='粘贴形如 {"uid":"...","nickname":"...","accessToken":"..."} 的完整 JSON' />
        </div>

        <div class="f-foot">
          <p class="tiny dim">
            凭据会用 AES-256-GCM 加密后存本机，只有网关进程能读。随时可以在下面撤回。
          </p>
          <button class="btn pri lg" :disabled="submitting" @click="submit">
            <span v-if="submitting" class="spin" />
            {{ submitting ? '提交中…' : '提交审核' }}
          </button>
        </div>
        </template>
      </article>
    </Transition>

    <p v-if="err" class="errline">{{ err }}</p>
    <p v-if="okMsg" class="okline">{{ okMsg }}</p>

    <!-- 列表 -->
    <div class="list-head">
      <h2>我的账号</h2>
      <span class="tiny dim">状态每 15 秒刷新</span>
    </div>

    <div class="acards">
      <article v-for="(a, i) in accounts" :key="a.id" class="acard glass glass-1"
               :data-acc="a.id" :style="{ '--i': i }">
        <div class="atop">
          <span class="astate" :class="a.state === 'ok' ? 'ok' : a.state === 'pending' ? 'warn' : 'off'" />
          <div class="aname">
            <b class="trunc">{{ a.name }}</b>
            <span class="tiny dim mono">{{ a.uid }}</span>
          </div>
          <span class="pill" :class="STATE_PILL[a.state] || ''">{{ STATE_LABEL[a.state] || a.state }}</span>
        </div>

        <p class="arealm tiny dim">{{ a.realmLabel }}</p>

        <div class="ametrics">
          <div><span>积分</span><b>{{ fmt.n(a.credits) }}</b></div>
          <div><span>延迟</span><b>{{ a.lat ? a.lat + 'ms' : '—' }}</b></div>
          <div><span>折算</span><b class="g">{{ ratio ? (ratio * 100).toFixed(0) + '%' : '—' }}</b></div>
        </div>

        <p v-if="a.note" class="anote tiny">{{ a.note }}</p>

        <div class="afoot">
          <button class="btn sm ghost danger" @click="remove(a.id)">撤回账号</button>
        </div>
      </article>

      <button class="acard add-tile" @click="showForm = true">
        <span class="plus">＋</span>
        <b>提交账号</b>
        <span class="tiny dim">OAuth 授权后即时生效</span>
      </button>
    </div>

    <!-- 规则 -->
    <article class="rules glass glass-1">
      <div class="between">
        <div>
          <p class="eyebrow">RULES</p>
          <h3>折算怎么算</h3>
        </div>
        <span class="pill ac">每月 1 日结算</span>
      </div>
      <div class="rulegrid">
        <div class="rule">
          <b>C1</b>
          <p>1 个账号 · 积分 ≥ 500 · 7 天在线率 ≥ 90%</p>
          <em>余额 30% 折算 · 保底 1.5 万点/日</em>
        </div>
        <div class="rule">
          <b>C2</b>
          <p>2 个账号 或 单账号积分 ≥ 2000</p>
          <em>余额 45% 折算 · 保底 6 万点/日</em>
        </div>
        <div class="rule">
          <b>C3</b>
          <p>3 个账号 · 30 天在线率 ≥ 95%</p>
          <em>余额 55% 折算 · 保底 18 万点/日</em>
        </div>
      </div>
      <p class="tiny dim fine">
        账号掉线超过 24 小时折算暂停；同一个账号不能重复提交；转卖或共享账号直接封号。
      </p>
    </article>
  </section>
</template>

<style scoped>
.acc{padding-top:26px;display:flex;flex-direction:column;gap:13px}
.ahead{display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap;min-width:0}
.ahead > div:first-child{min-width:0}
.ahead h1{margin-top:6px}
.aglance{display:flex;align-items:center;gap:16px;padding:12px 18px;border-radius:16px;
  animation:k-fade-up 520ms 100ms var(--e-out) both;flex:none}
.aglance div{display:flex;flex-direction:column;align-items:flex-end;line-height:1.2}
.aglance b{font-size:17px;font-weight:700;letter-spacing:-.03em;font-variant-numeric:tabular-nums}
.aglance span{font-size:10px;color:var(--tx-4)}
.aglance i{width:1px;height:22px;background:var(--line)}

/* 折算卡 */
.ratio{display:flex;align-items:center;justify-content:space-between;gap:16px;
  padding:15px 18px;animation:k-fade-up 560ms 60ms var(--e-out) both;flex-wrap:wrap}
.r-left h3{margin:4px 0 5px}
.r-right{display:flex;align-items:center;gap:9px;flex-wrap:wrap}

/* 表单 */
.form{padding:16px 18px;display:flex;flex-direction:column;gap:14px;
  animation:k-fade-up 420ms var(--e-out) both}
.steps{display:grid;grid-template-columns:repeat(4,1fr);gap:8px}
@media (max-width:760px){ .steps{grid-template-columns:1fr 1fr} }
.step{display:flex;gap:8px;padding:9px 10px;border-radius:10px;cursor:pointer;
  border:1px solid var(--line);background:rgba(255,255,255,.025);
  transition:border-color 240ms,background-color 240ms}
.step.on{border-color:var(--ac-line);background:rgba(255,138,61,.07)}
.sno{width:18px;height:18px;flex:none;border-radius:50%;display:grid;place-items:center;
  font-size:9.5px;font-weight:700;color:var(--tx-4);
  border:1px solid var(--line-2);background:rgba(0,0,0,.2)}
.step.on .sno{color:#ffe6d4;background:var(--ac-dim);border-color:var(--ac-line)}
.step b{font-size:11px;font-weight:640;display:block}
.step i{font-style:normal;font-size:9.5px;color:var(--tx-4);line-height:1.4;display:block;margin-top:2px}

.f-row{display:flex;gap:10px;flex-wrap:wrap}
.authrow{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-bottom:6px}
.authhint{margin:0 0 8px;line-height:1.55}
.oauth-realm{flex:1 1 100%;max-width:none}
.linkbtn{appearance:none;background:none;border:0;padding:4px 0 10px;cursor:pointer;
  color:var(--tx-3);font-size:11.5px;text-decoration:underline;text-underline-offset:3px}
.linkbtn:hover{color:var(--ac-1)}
.authrow .tiny{flex:1;min-width:180px}
.realm-sw{display:flex;gap:6px;flex:1 1 150px;min-width:0}
.realm-sw button{flex:1;min-width:0;height:36px;border-radius:10px;border:1px solid var(--line);overflow:hidden;text-overflow:ellipsis;
  background:rgba(255,255,255,.03);color:var(--tx-3);font-size:11.5px;font-weight:600;
  transition:all 220ms;white-space:nowrap}
.realm-sw button[data-on="1"]{color:var(--tx-1);border-color:var(--ac-line);
  background:linear-gradient(180deg,rgba(255,138,61,.18),rgba(255,138,61,.07))}
.lbl-in{flex:1 1 100%;min-width:0;height:36px;font-size:12px}
.ta{font-size:11px;line-height:1.6}
.f-foot{display:flex;align-items:center;justify-content:space-between;gap:14px;flex-wrap:wrap}
.f-foot .tiny{max-width:56ch;line-height:1.6}

.form-enter-active{animation:k-fade-up 400ms var(--e-out) both}
.form-leave-active{animation:k-fade-in 180ms reverse both}

.errline{font-size:12px;color:#f6b1ac;background:rgba(240,103,95,.10);
  border:1px solid rgba(240,103,95,.22);border-radius:9px;padding:8px 11px}
.okline{font-size:12px;color:#a9e8c5;background:rgba(84,201,138,.09);
  border:1px solid rgba(84,201,138,.2);border-radius:9px;padding:8px 11px}

.list-head{display:flex;align-items:baseline;justify-content:space-between;margin:8px 0 0}
.acards{display:grid;grid-template-columns:repeat(auto-fill,minmax(268px,1fr));gap:11px}
.acard{padding:14px 15px 12px;display:flex;flex-direction:column;gap:9px;
  animation:k-fade-up 540ms var(--e-out) both;animation-delay:calc(var(--i,0) * 70ms + 120ms);
  transition:transform 320ms var(--e-out),border-color 280ms}
.acard:hover{transform:translateY(-2px);border-color:rgba(255,255,255,.15)}
.atop{display:flex;align-items:center;gap:8px}
.astate{width:6px;height:6px;border-radius:50%;flex:none}
.astate.ok{background:var(--ok);box-shadow:0 0 8px 1px rgba(84,201,138,.5);
  animation:k-breathe 2.4s var(--e-in-out) infinite}
.astate.warn{background:var(--warn);box-shadow:0 0 8px 1px rgba(232,181,82,.4)}
.astate.off{background:var(--tx-4)}
.aname{display:flex;flex-direction:column;min-width:0;flex:1;line-height:1.3}
.aname b{font-size:12.5px;font-weight:640;letter-spacing:-.01em}
.arealm{margin-top:-4px}
.ametrics{display:grid;grid-template-columns:repeat(3,1fr);gap:2px;padding:9px 0;
  border-top:1px solid var(--line);border-bottom:1px solid var(--line)}
.ametrics div{display:flex;flex-direction:column;gap:2px}
.ametrics span{font-size:9.5px;color:var(--tx-4)}
.ametrics b{font-size:12px;font-weight:640;font-variant-numeric:tabular-nums}
.ametrics b.g{color:#8fe8b6}
.anote{color:#f0d29a;line-height:1.5}
.afoot{display:flex;gap:6px;margin-top:auto}
.afoot .btn{flex:1;height:29px;font-size:11px}
.danger:hover{color:#f6b1ac;background:rgba(240,103,95,.10)}

.add-tile{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:4px;
  min-height:152px;border-radius:var(--r-lg);border:1px dashed rgba(255,255,255,.16);
  background:rgba(255,255,255,.014);color:var(--tx-3);
  animation:k-fade-up 540ms var(--e-out) both;animation-delay:320ms;
  transition:background-color 240ms,border-color 240ms,transform 300ms var(--e-out)}
.add-tile:hover{background:rgba(255,255,255,.04);border-color:rgba(255,255,255,.26);transform:translateY(-2px)}
.plus{font-size:22px;font-weight:300;color:var(--tx-4);line-height:1}
.add-tile b{font-size:12px;font-weight:600}

/* 规则 */
.rules{padding:15px 18px;display:flex;flex-direction:column;gap:11px;margin-top:2px}
.rules h3{margin-top:3px}
.rulegrid{display:grid;grid-template-columns:repeat(3,1fr);gap:9px}
@media (max-width:760px){ .rulegrid{grid-template-columns:1fr} }
.rule{padding:11px 12px;border-radius:11px;background:rgba(255,255,255,.03);
  border:1px solid var(--line);display:flex;flex-direction:column;gap:4px}
.rule b{font-size:11px;font-weight:700;color:var(--ac-2);letter-spacing:.04em}
.rule p{font-size:10.5px;color:var(--tx-3);line-height:1.5}
.rule em{font-style:normal;font-size:10px;color:var(--tx-4)}
.fine{line-height:1.6}

.spin{width:13px;height:13px;border-radius:50%;border:1.8px solid rgba(255,255,255,.3);
  border-top-color:#fff;animation:spin .7s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
</style>

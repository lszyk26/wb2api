<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, fmt } from '../lib/api.js'

const tab = ref('overview')
const tabs = [
  { id: 'overview', label: '概览' },
  { id: 'users', label: '用户' },
  { id: 'orders', label: '订单' },
  { id: 'codes', label: '卡密' },
  { id: 'accounts', label: '账号审核' },
  { id: 'health', label: '账号健康' },
  { id: 'settle', label: '月度结算' },
  { id: 'pay', label: '支付流水' },
  { id: 'plans', label: '档位' },
  { id: 'lottery', label: '抽奖' },
]

const stats = ref(null)
const users = ref([])
const userQ = ref('')
const orders = ref([])
const codes = ref([])
const accounts = ref([])
const plans = ref([])
const health = ref([])
const settleRecords = ref([])
const settleReport = ref(null)
const settleMonth = ref('')
const settleBusy = ref(false)
const paylogs = ref([])
const payChannels = ref([])
const pool = ref(null)

/* ---------------- 抽奖 ---------------- */
const lpPool = ref('paid')
const lpPrizes = ref([])
const lpSums = ref({ paid: 0, free: 0 })
const lpDraws = ref([])
const lpTotal = ref(0)
const lpPage = ref(1)
const lpFilterPool = ref('')
const lpFilterUser = ref('')
const lpStats = ref(null)
const lpSet = ref({})
const lpWeightOk = ref(true)

async function loadLotteryPrizes() {
  try {
    const d = await api.admin.lotteryPrizes()
    lpPrizes.value = (d.prizes || []).filter(p => p.pool === lpPool.value)
    lpSums.value = { paid: d.paid_weight_sum || 0, free: d.free_weight_sum || 0 }
  } catch (e) { err.value = e.message }
}
async function switchLpPool(p) { lpPool.value = p; await loadLotteryPrizes() }
async function saveLpPrize(p) {
  err.value = ''; okMsg.value = ''
  try {
    await api.admin.lotterySavePrize({
      id: p.id, pool: p.pool, name: p.name, kind: p.kind,
      value: p.value, weight: p.weight, daily_stock: p.daily_stock,
      enabled: p.enabled, sort: p.sort,
    })
    okMsg.value = '奖品已保存（权重合计必须 10000，否则整笔回滚）'
    await loadLotteryPrizes()
  } catch (e) { err.value = e.message }
}
async function toggleLpPrize(p) {
  err.value = ''; okMsg.value = ''
  try {
    await api.admin.lotteryTogglePrize(p.id, !p.enabled)
    okMsg.value = p.enabled ? '已停用' : '已启用'
    await loadLotteryPrizes()
  } catch (e) { err.value = e.message }
}
async function resetLpStock() {
  err.value = ''; okMsg.value = ''
  try {
    const d = await api.admin.lotteryResetStock(lpPool.value)
    okMsg.value = `已重置 ${lpPool.value} 今日库存占用 ${d.cleared || 0} 条`
    await loadLotteryPrizes()
  } catch (e) { err.value = e.message }
}
async function loadLpDraws() {
  try {
    const q = ['page=' + lpPage.value, 'page_size=50']
    if (lpFilterPool.value) q.push('pool=' + lpFilterPool.value)
    if (lpFilterUser.value) q.push('user_id=' + lpFilterUser.value)
    const d = await api.admin.lotteryDraws('?' + q.join('&'))
    lpDraws.value = d.draws || []
    lpTotal.value = d.total || 0
  } catch (e) { err.value = e.message }
}
async function loadLpStats() {
  try { lpStats.value = await api.admin.lotteryStats() } catch (e) { err.value = e.message }
}
async function loadLpSet() {
  try {
    const d = await api.admin.lotterySettings()
    lpSet.value = d.settings || {}
    lpWeightOk.value = !!d.weight_ok
  } catch (e) { err.value = e.message }
}
async function saveLpSet() {
  err.value = ''; okMsg.value = ''
  try {
    await api.admin.lotterySaveSettings({ ...lpSet.value })
    okMsg.value = '抽奖设置已保存，立即生效'
    await loadLpSet(); await loadLotteryPrizes()
  } catch (e) { err.value = e.message }
}
async function doClawback(row) {
  const note = prompt('扣回原因（会写进流水）：', '管理员扣回')
  if (note === null) return
  err.value = ''; okMsg.value = ''
  try {
    const d = await api.admin.lotteryClawback(row.draw_id, note)
    okMsg.value = `已扣回 #${row.draw_id}，用户赠点余额 ${fmt.n(d.lottery_points)}`
    await loadLpDraws(); await loadLpStats()
  } catch (e) { err.value = e.message }
}
const lpPages = computed(() => Math.max(1, Math.ceil(lpTotal.value / 50)))

const err = ref('')
const okMsg = ref('')

async function load() {
  err.value = ''
  try {
    if (tab.value === 'overview') {
      stats.value = await api.admin.stats()
      pool.value = await api.admin.pool()
    } else if (tab.value === 'users') {
      const d = await api.admin.users(userQ.value)
      users.value = d.users || []
    } else if (tab.value === 'orders') {
      const d = await api.admin.orders()
      orders.value = d.orders || []
    } else if (tab.value === 'codes') {
      const d = await api.admin.codes()
      codes.value = d.codes || []
    } else if (tab.value === 'accounts') {
      const d = await api.admin.accounts()
      accounts.value = d.accounts || []
    } else if (tab.value === 'plans') {
      const d = await api.admin.plans()
      plans.value = d.plans || []
    } else if (tab.value === 'health') {
      const d = await api.admin.health()
      health.value = d.accounts || []
    } else if (tab.value === 'settle') {
      const d = await api.admin.settle()
      settleRecords.value = d.records || []
      if (!settleMonth.value) settleMonth.value = d.last_month || ''
    } else if (tab.value === 'lottery') {
      await Promise.all([loadLotteryPrizes(), loadLpDraws(), loadLpStats(), loadLpSet()])
    } else if (tab.value === 'pay') {
      const d = await api.admin.paylogs()
      paylogs.value = d.logs || []
      payChannels.value = d.channels || []
    }
  } catch (e) { err.value = e.message }
}

onMounted(load)
function switchTab(t) { tab.value = t; load() }

/* ---------- 用户操作 ---------- */
const PLAN_OPTS = ['', 'lv1', 'lv2', 'lv3', 'c1', 'c2', 'c3', 'c4']
async function setPlan(u, plan) {
  try {
    await api.admin.setPlan(u.id, plan, 1)
    okMsg.value = `${u.username} → ${plan || '游客'}`
    load()
  } catch (e) { err.value = e.message }
}
async function toggleBan(u) {
  try {
    await api.admin.ban(u.id, !u.banned, u.banned ? '' : '违反使用规则')
    load()
  } catch (e) { err.value = e.message }
}
const pointsTarget = ref(null)
const pointsVal = ref(0)
async function grantPoints() {
  if (!pointsTarget.value) return
  try {
    await api.admin.points(pointsTarget.value.id, Number(pointsVal.value), '管理员调整')
    okMsg.value = `已调整 ${pointsVal.value} 点`
    pointsTarget.value = null
    pointsVal.value = 0
    load()
  } catch (e) { err.value = e.message }
}

/* ---------- 订单 ---------- */
async function reviewOrder(o, action) {
  try {
    await api.admin.reviewOrder(o.id, action, '')
    load()
  } catch (e) { err.value = e.message }
}

/* ---------- 卡密 ---------- */
const gen = ref({ kind: 'points', points: 30000, plan: 'lv1', months: 1, count: 5, expire_days: 90, note: '' })
const generated = ref([])
async function genCode() {
  try {
    const d = await api.admin.genCode(gen.value)
    generated.value = d.codes || []
    load()
  } catch (e) { err.value = e.message }
}
function copyCodes() {
  const t = generated.value.join('\n')
  navigator.clipboard?.writeText(t)
  okMsg.value = '已复制全部卡密'
}

/* ---------- 账号审核 ---------- */
async function reviewAccount(a, action) {
  try {
    await api.admin.reviewAccount(a.id, action, '')
    load()
  } catch (e) { err.value = e.message }
}

/* ---------- 档位编辑 ---------- */
async function savePlan(p) {
  try {
    await api.admin.savePlan({
      code: p.code, name: p.Name, tagline: p.Tagline,
      price: p.Price, year_price: p.YearPrice, daily: p.Daily,
      conc: p.Conc, rpm: p.RPM, log_days: p.LogDays,
      ratio: p.Ratio, hot: p.Hot, enabled: p.Enabled, perks: '',
    })
    okMsg.value = `${p.code.toUpperCase()} 已保存`
    load()
  } catch (e) { err.value = e.message }
}

async function dryRun() {
  settleBusy.value = true
  err.value = ''
  try {
    const d = await api.admin.settle(settleMonth.value, true)
    settleReport.value = d.report
  } catch (e) { err.value = e.message }
  finally { settleBusy.value = false }
}

async function doSettle() {
  if (!confirm(`确认按 ${settleMonth.value} 的账号消耗正式结算？入账后不可撤销。`)) return
  settleBusy.value = true
  err.value = ''
  try {
    const d = await api.admin.settleRun(settleMonth.value)
    settleReport.value = d.report
    okMsg.value = `已结算 ${d.report.users} 人，共 ${d.report.total_points} 点`
    load()
  } catch (e) { err.value = e.message }
  finally { settleBusy.value = false }
}

const pendingOrders = computed(() => orders.value.filter(o => o.status === 'pending'))
const pendingAccounts = computed(() => accounts.value.filter(a => a.state === 'pending'))
</script>

<template>
  <section class="admin container">
    <header class="ahead2">
      <div>
        <p class="eyebrow">ADMIN</p>
        <h1>管理台</h1>
        <p class="sub" style="margin-top:8px">用户、订单、卡密、账号审核。所有改动即时生效。</p>
      </div>
    </header>

    <div class="tabs">
      <span class="ink" :style="{ width: `calc((100% - 6px) / ${tabs.length})`, transform: `translateX(${tabs.findIndex(t => t.id === tab) * 100}%)` }" />
      <button v-for="t in tabs" :key="t.id" :data-on="tab === t.id ? 1 : 0" @click="switchTab(t.id)">
        {{ t.label }}
        <i v-if="t.id === 'orders' && pendingOrders.length" class="badge">{{ pendingOrders.length }}</i>
        <i v-else-if="t.id === 'accounts' && pendingAccounts.length" class="badge">{{ pendingAccounts.length }}</i>
      </button>
    </div>

    <p v-if="err" class="errline">{{ err }}</p>
    <p v-if="okMsg" class="okline">{{ okMsg }}</p>

    <!-- ============ 概览 ============ -->
    <template v-if="tab === 'overview'">
      <div class="stats" v-if="stats">
        <article class="s glass glass-1"><span>用户</span><b>{{ stats.users }}</b><i>{{ stats.banned }} 封禁</i></article>
        <article class="s glass glass-1"><span>有效密钥</span><b>{{ stats.keys }}</b><i>全站</i></article>
        <article class="s glass glass-1"><span>今日调用</span><b>{{ fmt.n(stats.today_calls) }}</b><i>{{ fmt.pts(stats.today_points) }} 点</i></article>
        <article class="s glass glass-1"><span>累计收入</span><b>¥{{ fmt.n(stats.revenue, 2) }}</b><i>已确认订单</i></article>
        <article class="s glass glass-1 warncard"><span>待处理</span><b>{{ stats.pending_orders + stats.pending_accounts }}</b>
          <i>{{ stats.pending_orders }} 订单 · {{ stats.pending_accounts }} 账号</i></article>
      </div>

      <article class="panel glass glass-1" v-if="pool">
        <div class="phead"><div><p class="eyebrow">POOL</p><h3>上游池状态</h3></div></div>
        <div class="pool">
          <div class="prow"><span>上游</span><b>{{ pool.upstream?.ok ? '在线' : '异常' }}</b></div>
          <div class="prow"><span>账号数</span><b>{{ pool.upstream?.accounts ?? '—' }} / 可用 {{ pool.upstream?.accounts_ready ?? '—' }}</b></div>
          <div class="prow"><span>Realm</span><b>{{ pool.upstream?.realm || '—' }}</b></div>
          <div class="prow"><span>模型缓存</span><b class="tiny">{{ pool.models || '未刷新' }}</b></div>
        </div>
        <div class="usage-sum" v-if="pool.usage">
          <div class="urow"><span>近 7 天上游调用</span><b>{{ fmt.n(pool.usage.total_calls) }} 次</b></div>
          <div class="urow"><span>上游 credit 消耗</span><b>{{ pool.usage.total_credit }}</b></div>
        </div>
      </article>
    </template>

    <!-- ============ 用户 ============ -->
    <template v-else-if="tab === 'users'">
      <div class="searchbar">
        <input v-model.trim="userQ" class="input" placeholder="搜索用户名 / 邮箱 / 邀请码" @keydown.enter="load" />
        <button class="btn" @click="load">搜索</button>
      </div>

      <div class="table glass glass-1">
        <div class="tr th">
          <span>用户</span><span>档位</span><span class="num">今日</span><span class="num">加油包</span><span>状态</span><span>操作</span>
        </div>
        <div v-for="u in users" :key="u.id" class="tr">
          <span class="who">
            <b>{{ u.username }}</b>
            <i class="tiny dim">{{ u.email || u.invite_code }}</i>
            <i v-if="u.email_is_temp" class="pill err" style="font-size:9.5px;padding:1px 5px">临时邮箱</i>
          </span>
          <span>
            <select class="sel" :value="u.plan" @change="setPlan(u, $event.target.value)">
              <option v-for="p in PLAN_OPTS" :key="p" :value="p">{{ p ? p.toUpperCase() : '游客' }}</option>
            </select>
          </span>
          <span class="num">{{ fmt.pts(u.points_today) }}</span>
          <span class="num">{{ fmt.pts(u.bonus_points) }}</span>
          <span>
            <i class="pill" :class="u.banned ? 'err' : u.is_admin ? 'ac' : 'ok'">
              {{ u.banned ? '封禁' : u.is_admin ? '管理员' : '正常' }}
            </i>
          </span>
          <span class="ops">
            <button class="btn sm ghost" @click="pointsTarget = u">加点</button>
            <button class="btn sm ghost" :class="{ danger: !u.banned }" @click="toggleBan(u)">
              {{ u.banned ? '解封' : '封禁' }}
            </button>
          </span>
        </div>
        <p v-if="!users.length" class="empty-line">没有匹配的用户</p>
      </div>

      <Transition name="form">
        <article v-if="pointsTarget" class="panel glass glass-2">
          <div class="phead">
            <h3>给 {{ pointsTarget.username }} 调额度</h3>
            <button class="btn sm ghost" @click="pointsTarget = null">取消</button>
          </div>
          <div class="searchbar">
            <input v-model.number="pointsVal" type="number" class="input" placeholder="正数增加，负数扣减" />
            <button class="btn pri" @click="grantPoints">确认</button>
          </div>
          <p class="tiny dim">增加会写入加油包（90 天有效），扣减优先扣加油包。</p>
        </article>
      </Transition>
    </template>

    <!-- ============ 订单 ============ -->
    <template v-else-if="tab === 'orders'">
      <div class="table glass glass-1">
        <div class="tr th">
          <span>用户</span><span>档位</span><span class="num">金额</span><span>方式</span><span>状态</span><span>操作</span>
        </div>
        <div v-for="o in orders" :key="o.id" class="tr">
          <span class="who"><b>{{ o.username }}</b><i class="tiny dim">{{ fmt.date(o.created_at) }}</i></span>
          <span>{{ o.plan.toUpperCase() }} · {{ o.months }} 个月</span>
          <span class="num">¥{{ o.amount }}</span>
          <span class="tiny dim">{{ o.method }}</span>
          <span><i class="pill" :class="o.status === 'paid' ? 'ok' : o.status === 'pending' ? 'warn' : 'err'">
            {{ o.status === 'paid' ? '已确认' : o.status === 'pending' ? '待确认' : '已取消' }}</i></span>
          <span class="ops">
            <template v-if="o.status === 'pending'">
              <button class="btn sm pri" @click="reviewOrder(o, 'pay')">确认收款</button>
              <button class="btn sm ghost danger" @click="reviewOrder(o, 'cancel')">取消</button>
            </template>
          </span>
        </div>
        <p v-if="!orders.length" class="empty-line">还没有订单</p>
      </div>
    </template>

    <!-- ============ 卡密 ============ -->
    <template v-else-if="tab === 'codes'">
      <article class="panel glass glass-2">
        <div class="phead"><div><p class="eyebrow">GENERATE</p><h3>生成卡密</h3></div></div>
        <div class="gen">
          <select v-model="gen.kind" class="sel">
            <option value="points">点数卡</option>
            <option value="plan">档位卡</option>
          </select>
          <template v-if="gen.kind === 'points'">
            <input v-model.number="gen.points" type="number" class="input" placeholder="点数，如 30000" />
          </template>
          <template v-else>
            <select v-model="gen.plan" class="sel">
              <option v-for="p in ['lv1','lv2','lv3']" :key="p" :value="p">{{ p.toUpperCase() }}</option>
            </select>
            <input v-model.number="gen.months" type="number" class="input" placeholder="月数" />
          </template>
          <input v-model.number="gen.count" type="number" class="input" placeholder="数量" />
          <input v-model.number="gen.expire_days" type="number" class="input" placeholder="有效天数" />
          <button class="btn pri" @click="genCode">生成</button>
        </div>
        <div v-if="generated.length" class="genout">
          <pre class="codes">{{ generated.join('\n') }}</pre>
          <button class="btn sm" @click="copyCodes">复制全部</button>
        </div>
      </article>

      <div class="table glass glass-1">
        <div class="tr th">
          <span>卡密</span><span>类型</span><span class="num">面值</span><span>状态</span><span>创建</span>
        </div>
        <div v-for="c in codes" :key="c.ID || c.id" class="tr">
          <span class="mono tiny">{{ c.Code || c.code }}</span>
          <span class="tiny">{{ (c.Kind || c.kind) === 'points' ? '点数' : '档位 ' + (c.Plan || c.plan || '').toUpperCase() }}</span>
          <span class="num tiny">{{ (c.Kind || c.kind) === 'points' ? fmt.n(c.Points || c.points) + ' 点' : (c.Months || c.months) + ' 月' }}</span>
          <span><i class="pill" :class="(c.UsedBy || c.used_by) ? 'err' : 'ok'">{{ (c.UsedBy || c.used_by) ? '已使用' : '未使用' }}</i></span>
          <span class="tiny dim">{{ fmt.date(c.CreatedAt || c.created_at) }}</span>
        </div>
        <p v-if="!codes.length" class="empty-line">还没有卡密</p>
      </div>
    </template>

    <!-- ============ 账号审核 ============ -->
    <template v-else-if="tab === 'accounts'">
      <div class="table glass glass-1">
        <div class="tr th">
          <span>用户</span><span>账号</span><span>Realm</span><span class="num">积分</span><span>状态</span><span>操作</span>
        </div>
        <div v-for="a in accounts" :key="a.id" class="tr">
          <span class="who"><b>{{ a.username }}</b><i class="tiny dim">{{ fmt.date(a.created_at) }}</i></span>
          <span class="who"><b>{{ a.label }}</b><i class="tiny dim mono">{{ (a.uid_ref || '').slice(0, 14) }}</i></span>
          <span class="tiny">{{ a.realm }}</span>
          <span class="num">{{ fmt.n(a.credits) }}</span>
          <span><i class="pill" :class="a.state === 'ok' ? 'ok' : a.state === 'pending' ? 'warn' : 'err'">
            {{ { pending: '待审核', ok: '已接入', cool: '冷却', rejected: '未通过', revoked: '已撤回' }[a.state] || a.state }}</i></span>
          <span class="ops">
            <template v-if="a.state === 'pending'">
              <button class="btn sm pri" @click="reviewAccount(a, 'approve')">通过并接入</button>
              <button class="btn sm ghost danger" @click="reviewAccount(a, 'reject')">驳回</button>
            </template>
            <span v-else class="tiny dim">{{ a.note }}</span>
          </span>
        </div>
        <p v-if="!accounts.length" class="empty-line">还没有提交的账号</p>
      </div>
      <p class="hintbox tiny dim">
        「通过并接入」会写入上游 <code>&lt;WB2_HUB_DIR&gt;/accounts/&lt;uid&gt;.json</code>，
        容器有文件监听会自动加载。如果没生效，重启 wb-proxy-hub 即可。
      </p>
    </template>

    <!-- ============ 账号健康 ============ -->
    <template v-else-if="tab === 'health'">
      <div class="table glass glass-1">
        <div class="tr th hh">
          <span>账号</span><span>归属</span><span class="num">7日在线</span><span class="num">30日在线</span>
          <span class="num">30日调用</span><span class="num">30日 credit</span><span>状态</span>
        </div>
        <div v-for="a in health" :key="a.id" class="tr hh">
          <span class="who"><b>{{ a.label }}</b><i class="tiny dim mono">{{ (a.uid_ref||'').slice(0,14) }}</i></span>
          <span class="tiny">{{ a.username }}<i class="dim"> · {{ a.realm }}</i></span>
          <span class="num" :class="a.uptime_7d >= 90 ? 'g' : a.uptime_7d > 0 ? 'w' : 'r'">{{ a.uptime_7d }}%</span>
          <span class="num" :class="a.uptime_30d >= 90 ? 'g' : a.uptime_30d > 0 ? 'w' : 'r'">{{ a.uptime_30d }}%</span>
          <span class="num">{{ fmt.n(a.calls_30d) }}</span>
          <span class="num ac">{{ a.credit_30d }}</span>
          <span><i class="pill" :class="a.state === 'ok' ? 'ok' : 'warn'">{{ a.state }}</i></span>
        </div>
        <p v-if="!health.length" class="empty-line">还没有审核通过的授权账号</p>
      </div>
      <p class="hintbox tiny dim">
        在线率来自每 10 分钟一次的采样：该账号在过去 1 小时内是否有成功调用。
        连续 3 小时无调用会在账号备注里提示。
      </p>
    </template>

    <!-- ============ 月度结算 ============ -->
    <template v-else-if="tab === 'settle'">
      <article class="panel glass glass-2">
        <div class="phead">
          <div><p class="eyebrow">SETTLEMENT</p><h3>贡献折算结算</h3></div>
          <span class="pill">每月 1 日自动</span>
        </div>
        <div class="gen">
          <input v-model="settleMonth" class="input" placeholder="2026-09" style="max-width:140px" />
          <button class="btn" :disabled="settleBusy" @click="dryRun">试算（不入账）</button>
          <button class="btn pri" :disabled="settleBusy || !settleMonth" @click="doSettle">正式结算</button>
        </div>
        <p class="tiny dim">
          口径：账号在上月实际承载的 credit × 1000 × 用户档位比例，折算成加油包（90 天有效）。
          同一月份只会结算一次。
        </p>

        <div v-if="settleReport" class="report">
          <div class="rhead">
            <b>{{ settleReport.period }}</b>
            <span class="pill ac">{{ settleReport.users }} 人 · {{ settleReport.total_points }} 点</span>
          </div>
          <div v-for="(it, i) in settleReport.items" :key="i" class="ritem" :style="{ '--i': i }">
            <span class="who"><b>{{ it.username }}</b><i class="tiny dim">{{ it.account_uid.slice(0,12) }}</i></span>
            <span class="num">{{ it.credits }} credit</span>
            <span class="num">{{ (it.ratio*100).toFixed(0) }}%</span>
            <span class="num ac">+{{ it.points }}</span>
          </div>
          <p v-if="!settleReport.items.length" class="tiny dim" style="padding:10px 0">
            该月没有产生消耗的账号（或账号未接入）。
          </p>
        </div>
      </article>

      <div class="table glass glass-1">
        <div class="tr th ss">
          <span>月份</span><span>用户</span><span class="num">比例</span><span class="num">点数</span><span>时间</span>
        </div>
        <div v-for="r in settleRecords" :key="r.id" class="tr ss">
          <span class="mono tiny">{{ r.period }}</span>
          <span>{{ r.username }}</span>
          <span class="num">{{ (r.ratio*100).toFixed(0) }}%</span>
          <span class="num ac">+{{ r.points }}</span>
          <span class="tiny dim">{{ (r.at||'').slice(0,10) }}</span>
        </div>
        <p v-if="!settleRecords.length" class="empty-line">还没有结算记录</p>
      </div>
    </template>

    <!-- ============ 支付流水 ============ -->
    <template v-else-if="tab === 'pay'">
      <article class="panel glass glass-1">
        <div class="phead"><div><p class="eyebrow">CHANNELS</p><h3>支付通道</h3></div></div>
        <div class="chans">
          <div v-for="c in payChannels" :key="c.code" class="chan" :class="{ off: !c.enabled }">
            <span class="cdot" :class="c.enabled ? 'on' : 'off'" />
            <div class="cinfo">
              <b>{{ c.name }}</b>
              <span class="tiny dim mono">{{ c.code }} · {{ c.driver }}</span>
            </div>
            <span class="pill" :class="c.enabled ? 'ok' : 'err'">{{ c.enabled ? '已启用' : '未配置' }}</span>
          </div>
        </div>
        <p class="tiny dim">
          通道密钥在服务器 <code>data/env</code> 的 <code>WB2_PAY_*</code>，改完重启服务生效。
          回调地址：<code>/api/pay/notify/&lt;通道&gt;</code>
        </p>
      </article>

      <div class="table glass glass-1">
        <div class="tr th pp">
          <span>时间</span><span>订单号</span><span>通道</span><span class="num">金额</span><span>状态</span>
        </div>
        <div v-for="l in paylogs" :key="l.id" class="tr pp">
          <span class="tiny dim mono">{{ (l.at||'').slice(5,19).replace('T',' ') }}</span>
          <span class="mono tiny trunc">{{ l.order_no }}</span>
          <span class="tiny">{{ l.channel }}</span>
          <span class="num ac">¥{{ l.amount }}</span>
          <span><i class="pill ok">{{ l.action }}</i></span>
        </div>
        <p v-if="!paylogs.length" class="empty-line">还没有支付回调</p>
      </div>
    </template>


    <!-- ============ 抽奖 ============ -->
    <template v-else-if="tab === 'lottery'">
      <div class="pillrow">
        <span class="pill" :class="lpSums.paid === 10000 ? 'ok' : 'err'">付费池权重 {{ lpSums.paid }} / 10000</span>
        <span class="pill" :class="lpSums.free === 10000 ? 'ok' : 'err'">免费池权重 {{ lpSums.free }} / 10000</span>
        <span class="tiny dim">权重合计不等于 10000 时抽奖会被后端拒绝</span>
      </div>

      <article class="panel glass glass-2">
        <div class="phead">
          <div><p class="eyebrow">PRIZES</p><h3>奖池编辑</h3></div>
          <div class="poolsw">
            <button class="btn sm" :data-on="lpPool === 'paid' ? 1 : 0" @click="switchLpPool('paid')">付费池</button>
            <button class="btn sm" :data-on="lpPool === 'free' ? 1 : 0" @click="switchLpPool('free')">免费池</button>
            <button class="btn sm" @click="resetLpStock">重置今日库存</button>
          </div>
        </div>
        <div class="table">
          <div class="tr lp th">
            <span>奖品</span><span>类型</span><span class="num">点数</span><span class="num">权重</span>
            <span class="num">概率</span><span class="num">日库存</span><span class="num">剩余</span><span>操作</span>
          </div>
          <div v-for="p in lpPrizes" :key="p.id" class="tr lp">
            <span><input v-model="p.name" class="input mini" /></span>
            <span class="tiny dim">{{ p.kind }}</span>
            <span class="num"><input v-model.number="p.value" type="number" class="input mini" :disabled="p.kind === 'none'" /></span>
            <span class="num"><input v-model.number="p.weight" type="number" class="input mini" /></span>
            <span class="num tiny dim">{{ p.probability }}</span>
            <span class="num"><input v-model.number="p.daily_stock" type="number" class="input mini" /></span>
            <span class="num tiny">{{ p.remaining_stock < 0 ? '∞' : p.remaining_stock }}</span>
            <span class="ops">
              <button class="btn xs" @click="saveLpPrize(p)">保存</button>
              <button class="btn xs" @click="toggleLpPrize(p)">{{ p.enabled ? '停用' : '启用' }}</button>
            </span>
          </div>
        </div>
        <p class="tiny dim" style="margin:0">日库存 -1 = 不限量；大奖项建议限量，售罄后该奖项权重会并入「0 点」。</p>
      </article>

      <article class="panel glass glass-2" v-if="lpStats">
        <div class="phead"><div><p class="eyebrow">STATS</p><h3>返奖统计（近 14 天）</h3></div></div>
        <div class="stats">
          <div class="s glass glass-1"><span>付费抽次</span><b>{{ fmt.n(lpStats.total.paid_draws) }}</b><i>免费 {{ fmt.n(lpStats.total.free_draws) }}</i></div>
          <div class="s glass glass-1"><span>消耗点数</span><b>{{ fmt.n(lpStats.total.paid_cost_points) }}</b><i>付费池成本</i></div>
          <div class="s glass glass-1"><span>返奖点数</span><b>{{ fmt.n(lpStats.total.paid_back_points) }}</b><i>免费池另出 {{ fmt.n(lpStats.total.free_back_points) }}</i></div>
          <div class="s glass glass-1"><span>返奖率</span><b>{{ lpStats.total.paid_rtp_percent }}%</b><i>设计值 70.09%</i></div>
          <div class="s glass glass-1"><span>毛利点数</span><b>{{ fmt.n(lpStats.total.profit_points) }}</b><i>预算 {{ fmt.n(lpStats.budget_points) }} / 今日已返 {{ fmt.n(lpStats.budget_used_today) }}</i></div>
        </div>
        <div class="table" v-if="lpStats.days && lpStats.days.length">
          <div class="tr lpd th"><span>日期</span><span>池</span><span class="num">抽次</span><span class="num">消耗</span><span class="num">返奖</span><span class="num">返奖率</span></div>
          <div v-for="(d, i) in lpStats.days" :key="i" class="tr lpd">
            <span class="tiny dim">{{ d.day }}</span>
            <span class="tiny">{{ d.pool }}</span>
            <span class="num tiny">{{ fmt.n(d.draws) }}</span>
            <span class="num tiny">{{ fmt.n(d.cost_points) }}</span>
            <span class="num tiny">{{ fmt.n(d.prize_value) }}</span>
            <span class="num tiny">{{ d.cost_points > 0 ? (d.prize_value / d.cost_points * 100).toFixed(1) + '%' : '—' }}</span>
          </div>
        </div>
        <p v-else class="tiny dim" style="margin:0">还没有抽奖数据。</p>
      </article>

      <article class="panel glass glass-2">
        <div class="phead">
          <div><p class="eyebrow">DRAWS</p><h3>抽奖流水</h3></div>
          <div class="searchbar">
            <select v-model="lpFilterPool" class="sel" style="max-width:110px">
              <option value="">全部池</option>
              <option value="paid">付费</option>
              <option value="free">免费</option>
              <option value="clawback">扣回</option>
            </select>
            <input v-model.trim="lpFilterUser" class="input" placeholder="user_id" style="max-width:110px" />
            <button class="btn sm" @click="lpPage = 1; loadLpDraws()">筛选</button>
          </div>
        </div>
        <div class="table">
          <div class="tr lpd th">
            <span>时间</span><span>用户</span><span>池</span><span>奖品</span>
            <span class="num">返点</span><span class="num">扣点</span><span class="num">随机号</span><span>操作</span>
          </div>
          <div v-for="row in lpDraws" :key="row.draw_id" class="tr lpd">
            <span class="tiny dim">{{ fmt.ago(row.created_at) }}</span>
            <span class="tiny trunc">{{ row.username || row.user_id }}</span>
            <span><i class="pill" :class="row.pool === 'paid' ? 'ac' : row.pool === 'free' ? '' : 'err'">{{ row.pool }}</i></span>
            <span class="tiny trunc">{{ row.prize_name }}</span>
            <span class="num tiny">{{ fmt.n(row.prize_value) }}</span>
            <span class="num tiny">{{ fmt.n(row.cost_points) }}</span>
            <span class="num tiny dim">{{ row.random_no }}/{{ row.weight }}</span>
            <span>
              <button v-if="row.prize_value > 0 && row.pool !== 'clawback' && !row.revoked"
                      class="btn xs" @click="doClawback(row)">扣回</button>
              <i v-else-if="row.revoked" class="pill err">已扣回</i>
            </span>
          </div>
          <p v-if="!lpDraws.length" class="empty-line">还没有抽奖记录</p>
        </div>
        <div class="pager">
          <button class="btn sm" :disabled="lpPage <= 1" @click="lpPage--; loadLpDraws()">上一页</button>
          <span class="tiny dim">{{ lpPage }} / {{ lpPages }} · 共 {{ fmt.n(lpTotal) }} 条</span>
          <button class="btn sm" :disabled="lpPage >= lpPages" @click="lpPage++; loadLpDraws()">下一页</button>
        </div>
      </article>

      <article class="panel glass glass-2">
        <div class="phead"><div><p class="eyebrow">SETTINGS</p><h3>抽奖设置</h3></div></div>
        <div class="lpset">
          <label class="fld"><span>总开关（1 开 / 0 关）</span><input v-model="lpSet.lottery_enabled" class="input" /></label>
          <label class="fld"><span>单次消耗付费点数</span><input v-model="lpSet.lottery_cost_points" class="input" /></label>
          <label class="fld"><span>每日免费次数</span><input v-model="lpSet.lottery_daily_free" class="input" /></label>
          <label class="fld"><span>每日返奖预算（0 = 不启用）</span><input v-model="lpSet.lottery_daily_budget_points" class="input" /></label>
          <label class="fld"><span>新账号锁定小时（只能抽免费）</span><input v-model="lpSet.lottery_newbie_paid_hours" class="input" /></label>
          <label class="fld"><span>公示概率（1 / 0）</span><input v-model="lpSet.lottery_show_probability" class="input" /></label>
          <label class="fld wide"><span>公告文案</span><input v-model="lpSet.lottery_announcement" class="input" /></label>
        </div>
        <div style="display:flex;gap:8px;align-items:center">
          <button class="btn pri" @click="saveLpSet">保存设置</button>
          <span class="tiny dim">支付源固定 bonus_points；奖品出口固定 lottery_points（赠点不能抽奖）</span>
        </div>
      </article>
    </template>

    <!-- ============ 档位 ============ -->
    <template v-else-if="tab === 'plans'">
      <div class="plans">
        <article v-for="p in plans" :key="p.Code" class="plan glass glass-1">
          <div class="ph">
            <b>{{ (p.Code || '').toUpperCase() }}</b>
            <span class="tiny dim">{{ p.Kind }}</span>
          </div>
          <label class="fld"><span>名称</span><input v-model="p.Name" class="input" /></label>
          <label class="fld"><span>月价</span><input v-model.number="p.Price" type="number" class="input" /></label>
          <label class="fld"><span>年价</span><input v-model.number="p.YearPrice" type="number" class="input" /></label>
          <label class="fld"><span>日额度</span><input v-model.number="p.Daily" type="number" class="input" /></label>
          <label class="fld"><span>折算比例</span><input v-model.number="p.Ratio" type="number" step="0.01" class="input" /></label>
          <label class="fld"><span>并发</span><input v-model.number="p.Conc" type="number" class="input" /></label>
          <label class="fld"><span>RPM</span><input v-model.number="p.RPM" type="number" class="input" /></label>
          <button class="btn sm" @click="savePlan(p)">保存</button>
        </article>
      </div>
    </template>
  </section>
</template>

<style scoped>
.admin{padding-top:26px;display:flex;flex-direction:column;gap:13px}
.ahead2 h1{margin-top:6px}

.tabs{position:relative;display:flex;gap:0;padding:3px;border-radius:12px;
  background:rgba(255,255,255,.04);border:1px solid var(--line);overflow-x:auto;scrollbar-width:none}
.tabs::-webkit-scrollbar{display:none}
.ink{position:absolute;top:3px;left:3px;height:calc(100% - 6px);
  width:calc((100% - 6px) / 6);border-radius:9px;
  background:linear-gradient(160deg,rgba(255,255,255,.15),rgba(255,255,255,.06));
  border:1px solid rgba(255,255,255,.13);
  transition:transform 420ms var(--e-out)}
.tabs button{position:relative;z-index:1;flex:1;min-width:76px;height:32px;border-radius:9px;
  font-size:12px;font-weight:600;color:var(--tx-4);transition:color 220ms;white-space:nowrap}
.tabs button[data-on="1"]{color:var(--tx-1)}
.badge{font-style:normal;font-size:9px;font-weight:700;margin-left:4px;padding:1px 5px;
  border-radius:99px;background:var(--ac-dim);color:var(--ac-2);border:1px solid var(--ac-line)}

.errline{font-size:12px;color:#f6b1ac;background:rgba(240,103,95,.10);
  border:1px solid rgba(240,103,95,.22);border-radius:9px;padding:8px 11px}
.okline{font-size:12px;color:#a9e8c5;background:rgba(84,201,138,.09);
  border:1px solid rgba(84,201,138,.2);border-radius:9px;padding:8px 11px}

.stats{display:grid;grid-template-columns:repeat(5,1fr);gap:9px}
@media (max-width:900px){ .stats{grid-template-columns:repeat(2,1fr)} }
.s{padding:13px 14px;display:flex;flex-direction:column;gap:2px;
  animation:k-fade-up 500ms var(--e-out) both}
.s span{font-size:10.5px;color:var(--tx-3)}
.s b{font-size:21px;font-weight:720;letter-spacing:-.035em;font-variant-numeric:tabular-nums}
.s i{font-style:normal;font-size:9.5px;color:var(--tx-4)}
.warncard{border-color:var(--ac-line)}
.warncard b{color:var(--ac-2)}

.panel{padding:15px 17px;display:flex;flex-direction:column;gap:11px}
.phead{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.phead h3{margin-top:3px;font-size:14px}
.pool,.usage-sum{display:flex;flex-direction:column}
.prow,.urow{display:flex;align-items:center;justify-content:space-between;gap:12px;
  min-height:30px;border-top:1px solid rgba(255,255,255,.045);font-size:11.5px}
.prow:first-child,.urow:first-child{border-top:0}
.prow span,.urow span{color:var(--tx-3)}
.prow b,.urow b{color:var(--tx-1);font-weight:600;font-variant-numeric:tabular-nums}

.searchbar{display:flex;gap:8px}
.searchbar .input{flex:1;height:36px;font-size:12px}
.searchbar .btn{flex:none;height:36px}

.table{padding:6px 12px 8px;display:flex;flex-direction:column;overflow-x:auto}
.tr{display:grid;grid-template-columns:1.3fr .9fr 70px 70px 80px 150px;align-items:center;gap:10px;
  min-height:44px;border-top:1px solid rgba(255,255,255,.045);font-size:11.5px}
.tr.th{border-top:0;min-height:32px;color:var(--tx-4);font-size:10px}
.tr.th span{font-weight:600}
@media (max-width:820px){
  .tr{grid-template-columns:1.2fr .8fr 60px 70px 110px}
  .tr span:nth-child(4){display:none}
}
.who{display:flex;flex-direction:column;gap:1px;min-width:0}
.who b{font-size:11.5px;font-weight:620;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.num{text-align:right;font-variant-numeric:tabular-nums}
.ops{display:flex;gap:5px;flex-wrap:wrap}
.ops .btn{height:26px;font-size:10.5px;padding:0 8px}
.danger:hover{color:#f6b1ac;background:rgba(240,103,95,.10)}
.sel{height:28px;padding:0 8px;border-radius:7px;border:1px solid var(--line-2);
  background:rgba(0,0,0,.3);color:var(--tx-1);font-size:11px}
.empty-line{font-size:12px;color:var(--tx-4);padding:20px;text-align:center}
.hintbox{line-height:1.6;padding:0 2px}
.hintbox code{font-size:10.5px;color:var(--ac-2);background:var(--ac-dim);padding:1px 5px;border-radius:4px}

.gen{display:flex;gap:8px;flex-wrap:wrap}
.gen .input,.gen .sel{height:34px;font-size:11.5px;width:auto;flex:1;min-width:100px}
.gen .btn{flex:none;height:34px}
.genout{display:flex;flex-direction:column;gap:8px;align-items:flex-start}
.codes{font-size:11px;line-height:1.7;color:var(--ac-2);background:rgba(0,0,0,.3);
  border:1px solid var(--line);border-radius:9px;padding:10px 12px;max-height:200px;overflow:auto;
  width:100%;white-space:pre-wrap}

.hh{grid-template-columns:1.2fr .9fr 74px 80px 80px 90px 70px}
.ss{grid-template-columns:90px 1fr 70px 80px 100px}
.pp{grid-template-columns:120px 1.2fr 90px 80px 70px}
@media (max-width:820px){
  .hh{grid-template-columns:1.1fr .9fr 70px 80px}
  .hh span:nth-child(3),.hh span:nth-child(6){display:none}
  .pp{grid-template-columns:100px 1fr 70px 70px}
}
.num.g{color:#8fe8b6}
.num.w{color:#f0d29a}
.num.r{color:#f6b1ac}
.report{display:flex;flex-direction:column;gap:6px;padding:11px 13px;border-radius:11px;
  background:rgba(255,138,61,.05);border:1px solid var(--ac-line)}
.rhead{display:flex;align-items:center;justify-content:space-between;gap:10px;padding-bottom:6px;
  border-bottom:1px solid rgba(255,138,61,.16)}
.rhead b{font-size:12px}
.ritem{display:grid;grid-template-columns:1.3fr 90px 56px 80px;align-items:center;gap:8px;
  min-height:28px;font-size:11px;animation:k-fade-up 380ms var(--e-out) both;
  animation-delay:calc(var(--i,0)*30ms)}
.chans{display:flex;flex-direction:column;gap:7px}
.chan{display:flex;align-items:center;gap:10px;padding:9px 11px;border-radius:10px;
  background:rgba(255,255,255,.03);border:1px solid var(--line)}
.chan.off{opacity:.55}
.cdot{width:6px;height:6px;border-radius:50%;flex:none}
.cdot.on{background:var(--ok);box-shadow:0 0 8px 1px rgba(84,201,138,.5)}
.cdot.off{background:var(--tx-4)}
.cinfo{display:flex;flex-direction:column;gap:1px;flex:1;min-width:0}
.cinfo b{font-size:12px;font-weight:620}
.plans{display:grid;grid-template-columns:repeat(auto-fill,minmax(190px,1fr));gap:10px}
.plan{padding:12px 13px;display:flex;flex-direction:column;gap:7px}
.ph{display:flex;align-items:baseline;justify-content:space-between}
.ph b{font-size:12px;font-weight:700;color:var(--ac-2)}
.fld{display:flex;flex-direction:column;gap:3px}
.fld span{font-size:10px;color:var(--tx-4)}
.fld .input{height:30px;font-size:11px}


.pillrow{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.poolsw{display:flex;gap:6px;flex-wrap:wrap}
.poolsw .btn[data-on="1"]{background:var(--ac-dim);border-color:var(--ac-line);color:#ffcba6}
.tr.lp{grid-template-columns:1.15fr 74px 64px 64px 60px 70px 52px 104px}
.tr.lpd{grid-template-columns:88px 1fr 60px 1.1fr 70px 66px 74px 80px}
.input.mini{height:28px;font-size:11px;padding:0 6px;width:100%}
.ops{display:flex;gap:4px}
.btn.xs{height:26px;padding:0 8px;font-size:11px;border-radius:7px}
.pager{display:flex;gap:8px;align-items:center;justify-content:flex-end}
.lpset{display:grid;grid-template-columns:repeat(auto-fit,minmax(168px,1fr));gap:10px}
.lpset .wide{grid-column:1 / -1}
@media (max-width:820px){
  .tr.lp{grid-template-columns:1.1fr 58px 54px 54px 60px 84px}
  .tr.lp span:nth-child(3),.tr.lp span:nth-child(7){display:none}
  .tr.lpd{grid-template-columns:70px 1fr 50px 74px 60px}
  .tr.lpd span:nth-child(6),.tr.lpd span:nth-child(7),.tr.lpd span:nth-child(8){display:none}
}

.form-enter-active{animation:k-fade-up 380ms var(--e-out) both}
.form-leave-active{animation:k-fade-in 160ms reverse both}
</style>

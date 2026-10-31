/* ============================================================
   api.js — 前端唯一的网络层
   统一处理：令牌、错误信息、401 自动登出
   ============================================================ */

const TOKEN_KEY = 'wb2_token'
const USER_KEY = 'wb2_user'

export const state = {
  token: localStorage.getItem(TOKEN_KEY) || '',
  user: JSON.parse(localStorage.getItem(USER_KEY) || 'null'),
}

export function setSession(token, user) {
  state.token = token || ''
  state.user = user || null
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
  if (user) localStorage.setItem(USER_KEY, JSON.stringify(user))
  else localStorage.removeItem(USER_KEY)
}

export function logout() {
  setSession('', null)
}

let onUnauthorized = null
export function onAuthFail(fn) { onUnauthorized = fn }

async function req(path, { method = 'GET', body, raw = false } = {}) {
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (state.token) headers.Authorization = 'Bearer ' + state.token

  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (res.status === 401 && onUnauthorized && !path.startsWith('/api/login')) {
    onUnauthorized()
  }

  const text = await res.text()
  let data = null
  try { data = text ? JSON.parse(text) : null } catch (_) { data = { raw: text } }

  if (!res.ok) {
    const msg = data?.error?.message || data?.error || `请求失败（${res.status}）`
    const err = new Error(msg)
    err.status = res.status
    err.data = data
    throw err
  }
  return raw ? { res, data } : data
}

export const api = {
  // ---- 公开 ----
  plans: () => req('/api/plans'),
  status: () => req('/api/status'),
  health: () => req('/api/health'),

  // ---- 认证 ----
  register: (u) => req('/api/register', { method: 'POST', body: u }),
  login: (u) => req('/api/login', { method: 'POST', body: u }),
  sendCode: (email, purpose = 'register') =>
    req('/api/auth/code', { method: 'POST', body: { email, purpose } }),
  emailLogin: (email, code) =>
    req('/api/auth/email-login', { method: 'POST', body: { email, code } }),
  resetPassword: (email, code, password) =>
    req('/api/auth/reset', { method: 'POST', body: { email, code, password } }),
  ghStart: (invite = '') =>
    req('/api/auth/github/start' + (invite ? '?invite=' + encodeURIComponent(invite) : '')),
  ghExchange: (ticket) =>
    req('/api/auth/github/exchange', { method: 'POST', body: { ticket } }),
  me: () => req('/api/me'),
  overview: () => req('/api/me/overview'),
  usage: (days = 7) => req(`/api/me/usage?days=${days}`),
  logs: (page = 1, filter = '') => req(`/api/me/logs?page=${page}${filter ? '&' + filter : ''}`),

  // ---- 密钥 ----
  keys: () => req('/api/me/keys'),
  createKey: (name) => req('/api/me/keys', { method: 'POST', body: { name } }),
  deleteKey: (id) => req('/api/me/keys/delete', { method: 'POST', body: { id } }),
  rotateKey: (id) => req('/api/me/keys/rotate', { method: 'POST', body: { id } }),

  // ---- 兑换 / 订单 / 支付 ----
  redeem: (code) => req('/api/me/redeem', { method: 'POST', body: { code } }),
  orders: () => req('/api/me/orders'),
  createOrder: (plan, months, method, note) =>
    req('/api/me/order', { method: 'POST', body: { plan, months, method, note } }),
  channels: () => req('/api/pay/channels'),
  orderStatus: (orderNo) => req('/api/me/order/status?order_no=' + encodeURIComponent(orderNo)),

  // ---- 抽奖 ----
  lotteryConfig: () => req('/api/lottery/config'),
  lotteryDraw: (pool, idempotencyKey) =>
    req('/api/lottery/draw', { method: 'POST', body: { pool, idempotency_key: idempotencyKey } }),
  lotteryHistory: (page = 1, pageSize = 20) =>
    req(`/api/lottery/history?page=${page}&page_size=${pageSize}`),

  // ---- 授权账号 ----
  accounts: () => req('/api/me/accounts'),
  meInvite: () => req('/api/me/invite'),
  oauthStart: (realm) => req('/api/me/account/oauth/start', { method: 'POST', body: { realm } }),
  oauthPoll: (state) => req('/api/me/account/oauth/poll', { method: 'POST', body: { state } }),
  submitAccount: (realm, label, raw) =>
    req('/api/me/account/submit', { method: 'POST', body: { realm, label, raw } }),
  deleteAccount: (id) => req('/api/me/account/delete', { method: 'POST', body: { id } }),

  // ---- 账号设置 ----
  changePassword: (oldP, newP) => req('/api/me/password', { method: 'POST', body: { old: oldP, new: newP } }),
  studentVerify: (email) => req('/api/me/student', { method: 'POST', body: { email } }),

  // ---- 演练场（流式）----
  async playgroundChat({ model, messages, signal, onDelta, onDone, onError }) {
    const res = await fetch('/api/playground/chat', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: 'Bearer ' + state.token,
      },
      body: JSON.stringify({ model, messages, stream: true }),
      signal,
    })
    if (!res.ok) {
      const t = await res.text()
      let m = `请求失败（${res.status}）`
      try { m = JSON.parse(t)?.error?.message || m } catch (_) {}
      onError && onError(new Error(m))
      return
    }
    const reader = res.body.getReader()
    const dec = new TextDecoder()
    let buf = ''
    let text = ''
    let usage = null
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buf += dec.decode(value, { stream: true })
      const lines = buf.split('\n')
      buf = lines.pop() || ''
      for (const raw of lines) {
        const line = raw.trim()
        if (!line.startsWith('data:')) continue
        const js = line.slice(5).trim()
        if (js === '[DONE]') continue
        try {
          const obj = JSON.parse(js)
          const d = obj.choices?.[0]?.delta || {}
          const piece = d.content || ''
          if (piece) { text += piece; onDelta && onDelta(piece, text) }
          if (obj.usage) usage = obj.usage
        } catch (_) {}
      }
    }
    onDone && onDone(text, usage)
  },

  // ---- 管理 ----
  admin: {
    stats: () => req('/api/admin/stats'),
    users: (q = '', page = 1) => req(`/api/admin/users?q=${encodeURIComponent(q)}&page=${page}`),
    setPlan: (id, plan, months = 1) => req('/api/admin/user/plan', { method: 'POST', body: { id, plan, months } }),
    ban: (id, banned, reason) => req('/api/admin/user/ban', { method: 'POST', body: { id, banned, reason } }),
    points: (id, points, note) => req('/api/admin/user/points', { method: 'POST', body: { id, points, note } }),
    orders: (status = '') => req(`/api/admin/orders?status=${status}`),
    reviewOrder: (id, action, note) => req('/api/admin/order/review', { method: 'POST', body: { id, action, note } }),
    codes: () => req('/api/admin/codes'),
    genCode: (payload) => req('/api/admin/code/gen', { method: 'POST', body: payload }),
    accounts: (state = '') => req(`/api/admin/accounts?state=${state}`),
    reviewAccount: (id, action, note) => req('/api/admin/account/review', { method: 'POST', body: { id, action, note } }),
    plans: () => req('/api/admin/plans'),
    savePlan: (p) => req('/api/admin/plan/save', { method: 'POST', body: p }),
    pool: () => req('/api/admin/pool'),
    settings: () => req('/api/admin/settings'),
    saveSettings: (kv) => req('/api/admin/settings', { method: 'POST', body: kv }),
    settle: (month, dryRun) => req('/api/admin/settle', {
      method: dryRun === undefined ? 'GET' : 'POST',
      body: dryRun === undefined ? undefined : { month, dry_run: dryRun },
    }),
    settleRun: (month) => req('/api/admin/settle', { method: 'POST', body: { month, confirm: true } }),
    paylogs: () => req('/api/admin/paylogs'),
    health: () => req('/api/admin/health'),

    // 抽奖
    lotteryPrizes: (pool = '') => req('/api/admin/lottery/prizes' + (pool ? '?pool=' + pool : '')),
    lotterySavePrize: (p) => req('/api/admin/lottery/prize/save', { method: 'POST', body: p }),
    lotteryTogglePrize: (id, enabled) => req('/api/admin/lottery/prize/toggle', { method: 'POST', body: { id, enabled } }),
    lotteryResetStock: (pool = '') => req('/api/admin/lottery/stock/reset', { method: 'POST', body: { pool } }),
    lotteryDraws: (q = '') => req('/api/admin/lottery/draws' + q),
    lotteryStats: () => req('/api/admin/lottery/stats'),
    lotteryClawback: (drawId, note) => req('/api/admin/lottery/clawback', { method: 'POST', body: { draw_id: drawId, note } }),
    lotterySettings: () => req('/api/admin/lottery/settings'),
    lotterySaveSettings: (kv) => req('/api/admin/lottery/settings', { method: 'POST', body: kv }),
  },
}

/* 常用工具 */
export const fmt = {
  n: (v, d = 0) => (v == null ? '—' : Number(v).toLocaleString('zh-CN', { maximumFractionDigits: d })),
  pts: (v) => {
    if (v == null) return '—'
    const n = Number(v)
    return n >= 10000 ? (n / 10000).toFixed(1) + ' 万' : n.toFixed(n < 10 ? 1 : 0)
  },
  ms: (v) => (v == null || v === 0 ? '—' : v + 'ms'),
  date: (s) => (s ? String(s).slice(0, 10) : '—'),
  ago: (s) => {
    if (!s) return '—'
    const d = (Date.now() - new Date(s).getTime()) / 1000
    if (d < 60) return '刚刚'
    if (d < 3600) return Math.floor(d / 60) + ' 分钟前'
    if (d < 86400) return Math.floor(d / 3600) + ' 小时前'
    return Math.floor(d / 86400) + ' 天前'
  },
}

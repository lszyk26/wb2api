/* ============================================================
   spring.js — 轻量物理动效内核（无依赖，~3KB）
   - 统一 rAF 时钟，多实例共享
   - 支持多属性同时求解、速度投影、橡皮筋阻尼
   - 支持中途打断（保留当前速度 → 真正的"跟手"手感）
   ============================================================ */

const clamp = (v, a, b) => (v < a ? a : v > b ? b : v)

/* ---- 全局单例时钟 ---- */
const tickers = new Set()
let rafId = 0
let last = 0

function loop(t) {
  // 真实 dt，上限 0.25s（避免切后台回来后一次性跳跃）
  const dt = last ? Math.min((t - last) / 1000, 0.25) : 1 / 60
  last = t
  for (const tick of tickers) tick(dt, t)
  if (tickers.size) rafId = requestAnimationFrame(loop)
  else { rafId = 0; last = 0 }
}
function addTicker(fn) {
  tickers.add(fn)
  if (!rafId) { rafId = requestAnimationFrame(loop) }
}
function removeTicker(fn) {
  tickers.delete(fn)
}

export const DEFAULT_SPRING = { stiffness: 380, damping: 32, mass: 1, restDelta: 0.0015, restSpeed: 0.02 }

/**
 * 创建弹簧
 * @param {number|Object} from  起始值，或 {key: value}
 * @param {Object} opts
 *   target 目标值（数字或对象）
 *   stiffness/damping/mass
 *   velocity 初始速度（数字或对象）
 *   onUpdate(v, done)
 *   onComplete()
 */
export function createSpring(from, opts = {}) {
  const isNum = typeof from === 'number'
  const cfg = { ...DEFAULT_SPRING, ...opts }
  const keys = isNum ? ['v'] : Object.keys(from)

  const cur = {}, vel = {}, tgt = {}
  keys.forEach(k => {
    cur[k] = isNum ? from : (from[k] ?? 0)
    vel[k] = ((isNum ? opts.velocity : (opts.velocity && opts.velocity[k])) || 0)
    tgt[k] = isNum ? (opts.target ?? 0) : ((opts.target && opts.target[k]) ?? cur[k])
  })

  let running = false
  let done = false

  const step = (dt) => {
    const { stiffness: k, damping: c, mass: m, restDelta, restSpeed } = cfg
    // 子步积分：低帧率（30fps 或更差）时物理仍然准确，不会"变慢"
    let allRest = true
    let remain = dt
    while (remain > 1e-6) {
      const h = Math.min(remain, 1 / 120)
      remain -= h
      for (const key of keys) {
        // F = -k * x - c * v
        const x = cur[key] - tgt[key]
        const a = (-k * x - c * vel[key]) / m
        vel[key] += a * h
        cur[key] += vel[key] * h
      }
    }
    for (const key of keys) {
      const x = cur[key] - tgt[key]
      if (Math.abs(vel[key]) > restSpeed || Math.abs(x) > restDelta) allRest = false
    }
    if (allRest) {
      for (const key of keys) { cur[key] = tgt[key]; vel[key] = 0 }
      finish()
      return
    }
    if (cfg.onUpdate) cfg.onUpdate(isNum ? cur.v : { ...cur }, false)
  }

  function finish() {
    if (done) return
    done = true; running = false
    removeTicker(step)
    if (cfg.onUpdate) cfg.onUpdate(isNum ? cur.v : { ...cur }, true)
    if (cfg.onComplete) cfg.onComplete()
  }

  const api = {
    start() {
      if (running) return api
      running = true
      addTicker(step)
      return api
    },
    stop() { running = false; removeTicker(step); return api },
    finish,
    /** 中途改目标，保留当前速度（打断动画的最佳手感） */
    retarget(next, extra = {}) {
      keys.forEach(k => {
        if (isNum) tgt.v = next
        else if (next && k in next) tgt[k] = next[k]
      })
      if (extra.stiffness) cfg.stiffness = extra.stiffness
      if (extra.damping) cfg.damping = extra.damping
      if (!running) { running = true; addTicker(step) }
      return api
    },
    /** 设置速度（手势甩出时接管） */
    setVelocity(v) {
      keys.forEach(k => { vel[k] = isNum ? v : (v[k] || 0) })
      if (!running) { running = true; addTicker(step) }
      return api
    },
    get value() { return isNum ? cur.v : { ...cur } },
    get done() { return done },
  }
  return api
}

/* ---- 便捷：数字弹簧 ---- */
export function spring(from, to, onUpdate, o = {}) {
  return createSpring(from, { target: to, onUpdate, ...o }).start()
}

/* ---- 橡皮筋：越界后阻力递增 ---- */
export function rubberband(distance, dimension, constant = 0.55) {
  const sign = Math.sign(distance)
  const abs = Math.abs(distance)
  return sign * (1 - 1 / ((abs * constant) / dimension + 1)) * dimension
}

/* ---- 速度投影（惯性）：v 是 px/s ---- */
export function project(velocity, deceleration = 0.998) {
  // 等比衰减积分：Σ v * d^n = v / (1 - d)，用 60fps 折算
  return (velocity / 1000) / (1 - deceleration) * 16.6
}

/* ---- 手势：支持鼠标 + 触摸 Pointer Events ---- */
export function drag(el, handlers = {}) {
  const {
    onStart, onMove, onEnd, axis = 'xy',
    threshold = 3, lockDirection = true, preventScroll = true,
  } = handlers
  let active = false, id = null, captured = false
  let sx = 0, sy = 0, lx = 0, ly = 0, ltime = 0
  let vx = 0, vy = 0
  let locked = null

  const now = () => performance.now()

  function down(e) {
    if (e.button !== undefined && e.button !== 0) return
    active = true; id = e.pointerId; captured = false
    sx = lx = e.clientX; sy = ly = e.clientY
    ltime = now(); vx = vy = 0; locked = null
    // 注意：不在这里 setPointerCapture —— 否则 click 会被重定向到容器，
    // 抽屉里的按钮就点不动了。等确认是拖拽再捕获。
    if (onStart) onStart({ x: 0, y: 0, event: e })
  }
  function move(e) {
    if (!active || e.pointerId !== id) return
    const dx = e.clientX - sx, dy = e.clientY - sy
    if (!locked) {
      if (Math.hypot(dx, dy) < threshold) return
      if (lockDirection) {
        locked = Math.abs(dx) > Math.abs(dy) ? 'x' : 'y'
        if (axis === 'x' && locked !== 'x') { active = false; return }
        if (axis === 'y' && locked !== 'y') { active = false; return }
      }
      // 确认是拖拽后才捕获指针（保证拖出边界仍能收到事件）
      if (!captured && el.setPointerCapture) {
        try { el.setPointerCapture(id); captured = true } catch (_) {}
      }
    }
    if (preventScroll && e.cancelable) e.preventDefault()
    const t = now()
    const dt = Math.max(t - ltime, 1)
    vx = ((e.clientX - lx) / dt) * 1000
    vy = ((e.clientY - ly) / dt) * 1000
    lx = e.clientX; ly = e.clientY; ltime = t
    if (onMove) onMove({ x: dx, y: dy, vx, vy, event: e })
  }
  function up(e) {
    if (!active || (e.pointerId !== undefined && e.pointerId !== id)) return
    active = false
    if (captured && el.releasePointerCapture) {
      try { el.releasePointerCapture(id) } catch (_) {}
      captured = false
    }
    if (onEnd) onEnd({ x: lx - sx, y: ly - sy, vx, vy, event: e })
  }

  el.addEventListener('pointerdown', down, { passive: true })
  el.addEventListener('pointermove', move, { passive: !preventScroll })
  el.addEventListener('pointerup', up, { passive: true })
  el.addEventListener('pointercancel', up, { passive: true })

  return () => {
    el.removeEventListener('pointerdown', down)
    el.removeEventListener('pointermove', move)
    el.removeEventListener('pointerup', up)
    el.removeEventListener('pointercancel', up)
  }
}

/* ---- 便捷：把数值映射到 transform ---- */
export const tx = (x, y = 0, z = 0) => `translate3d(${x}px,${y}px,${z}px)`
export const lerp = (a, b, t) => a + (b - a) * t
export { clamp }

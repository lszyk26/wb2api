/* 流畅度档位：localStorage 记忆 + <html data-perf> 应用 */
const KEY = 'wb2_perf'
export const LEVELS = [
  { id: 'full', name: '完整', hint: '模糊 + 全部动效' },
  { id: 'balanced', name: '均衡', hint: '去背景模糊，动效保留' },
  { id: 'lite', name: '精简', hint: '去模糊和背景光效' },
  { id: 'smooth', name: '流畅', hint: '全部动效关闭，最省' },
]
const IDS = LEVELS.map(l => l.id)

export function getPerf() {
  try {
    const v = localStorage.getItem(KEY)
    if (v && IDS.includes(v)) return v
  } catch (e) { /* 隐私模式 */ }
  // 没有手动设置过：跟随系统「减少动效」偏好
  try {
    if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'lite'
  } catch (e) { /* ignore */ }
  return 'full'
}

export function applyPerf(level) {
  const v = IDS.includes(level) ? level : 'full'
  document.documentElement.setAttribute('data-perf', v)
  return v
}

export function setPerf(level) {
  const v = applyPerf(level)
  try { localStorage.setItem(KEY, v) } catch (e) { /* ignore */ }
  return v
}

export function initPerf() {
  return applyPerf(getPerf())
}

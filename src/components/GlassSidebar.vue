<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { createSpring, drag, rubberband, project, clamp } from '../lib/spring.js'

const props = defineProps({
  open: { type: Boolean, default: false },
  items: { type: Array, default: () => [] },
  active: { type: String, default: '' },
  user: { type: Object, default: null },
})
const emit = defineEmits(['close', 'select'])

const SB_W = () => (window.innerWidth < 1024 ? Math.min(300, window.innerWidth * 0.84) : 268)

const rootEl = ref(null)
const scrimEl = ref(null)
const inkEl = ref(null)
const listEl = ref(null)

let x = 0            // 当前偏移（0 = 完全展开，-W = 完全收起）
let dragX = null
let springCtl = null
let detach = null
let isDesktop = false

/* ---------- 尺寸 / 断点 ---------- */
function measure() {
  isDesktop = window.innerWidth >= 1024
  document.documentElement.style.setProperty('--sb-w', SB_W() + 'px')
}

/* ---------- 应用变换 ---------- */
function paint(v) {
  const W = SB_W()
  const p = clamp((v + W) / W, 0, 1)          // 0 = 收起, 1 = 展开
  if (rootEl.value) {
    rootEl.value.style.transform = `translate3d(${v}px,0,0)`
  }
  if (scrimEl.value) {
    scrimEl.value.style.opacity = String(p)
    scrimEl.value.style.pointerEvents = p > 0.02 ? 'auto' : 'none'
  }
  // 主界面：缩放 + 变暗，营造景深（不做整页 blur，保帧率）
  const shell = document.getElementById('shell')
  if (shell) {
    const sc = 1 - 0.055 * p
    shell.style.transform = isDesktop ? '' : `scale(${sc.toFixed(4)})`
    shell.style.borderRadius = isDesktop ? '' : `${(18 * p).toFixed(1)}px`
  }
  document.documentElement.style.setProperty('--scrim', p.toFixed(3))
}

function springTo(target, velocity = 0) {
  springCtl && springCtl.stop()
  springCtl = createSpring(x, {
    target,
    velocity,
    stiffness: 420,
    damping: 42,
    onUpdate: (v, done) => { x = v; paint(v); if (done) paint(target) },
  }).start()
}

function setOpen(open) {
  springTo(open ? 0 : -SB_W())
}

/* ---------- 手势：只在移动端 ---------- */
function bindDrag() {
  if (!rootEl.value) return
  detach = drag(rootEl.value, {
    axis: 'x',
    onStart() {
      if (isDesktop) return
      springCtl && springCtl.stop()
      dragX = x
    },
    onMove({ x: dx, vx }) {
      if (isDesktop) return
      const W = SB_W()
      let next = dragX + dx
      // 左右越界橡皮筋
      if (next > 0) next = rubberband(next, 60)
      if (next < -W) next = -W - rubberband(-(next + W), 60)
      x = next
      paint(x)
    },
    onEnd({ vx }) {
      if (isDesktop) return
      const W = SB_W()
      const projected = x + project(vx, 0.995)
      const shouldOpen = vx > 550 ? true : vx < -550 ? false : projected > -W * 0.55
      springTo(shouldOpen ? 0 : -W, vx * 0.6)
      if (!shouldOpen) emit('close')
    },
  })
}

/* ---------- 高亮墨水条 ---------- */
const ink = ref({ top: 0, h: 0, show: false })
function paintInk(animate = true) {
  const list = listEl.value
  if (!list) return
  const el = list.querySelector('[data-on="1"]')
  if (!el) { ink.value.show = false; return }
  const top = el.offsetTop
  const h = el.offsetHeight
  if (!animate) {
    ink.value = { top, h, show: true }
    return
  }
  const from = { top: ink.value.show ? ink.value.top : top, h: ink.value.show ? ink.value.h : h }
  ink.value.show = true
  const ctl = createSpring(from, {
    target: { top, h },
    stiffness: 520, damping: 40,
    onUpdate: v => { ink.value = { top: v.top, h: v.h, show: true } },
  })
  ctl.start()
}

/* ---------- 生命周期 ---------- */
/* 桌面端：侧边栏常驻展开（不参与抽屉逻辑） */
function restX() {
  return isDesktop ? 0 : (props.open ? 0 : -SB_W())
}

onMounted(() => {
  measure()
  x = restX()
  paint(x)
  nextTick(() => paintInk(false))
  bindDrag()
  window.addEventListener('resize', () => {
    measure()
    if (isDesktop) {
      x = 0
    } else if (x > -1) {
      x = props.open ? 0 : -SB_W()
    }
    paint(x)
  }, { passive: true })
})
onBeforeUnmount(() => { detach && detach(); springCtl && springCtl.stop() })

watch(() => props.open, v => {
  if (isDesktop) { x = 0; paint(0); return }
  setOpen(v)
})
watch(() => props.active, () => nextTick(() => paintInk(true)))
watch(() => props.items, () => nextTick(() => paintInk(true)))

function pick(id) {
  emit('select', id)
  if (!isDesktop) emit('close')
}

/* ---------- 视觉分组 ---------- */
const groups = computed(() => {
  const g = new Map()
  for (const it of props.items) {
    const k = it.group || ''
    if (!g.has(k)) g.set(k, [])
    g.get(k).push(it)
  }
  return [...g.entries()].map(([name, list]) => ({ name, list }))
})
</script>

<template>
  <!-- 移动端遮罩 -->
  <div ref="scrimEl" class="sb-scrim" @click="emit('close')" />

  <aside ref="rootEl" class="sb" aria-label="侧边栏导航">
    <div class="sb-inner">
      <!-- 品牌 -->
      <div class="sb-brand">
        <span class="mark" aria-hidden="true">
          <svg viewBox="0 0 32 32" width="26" height="26" fill="none">
            <circle cx="16" cy="16" r="10.5" stroke="currentColor" stroke-width="2.6"
                    stroke-linecap="round" stroke-dasharray="46 20" transform="rotate(-45 16 16)" />
            <circle cx="16" cy="16" r="3.1" fill="currentColor" />
          </svg>
        </span>
        <span class="bname">
          <strong>WorkBuddy</strong>
          <em>2.0</em>
        </span>
        <button class="sb-x" aria-label="关闭侧边栏" @click="emit('close')">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>
          </svg>
        </button>
      </div>

      <!-- 导航 -->
      <nav class="sb-nav no-sb" ref="listEl">
        <div class="ink" v-show="ink.show"
             :style="{ transform: `translate3d(0,${ink.top}px,0)`, height: ink.h + 'px' }" />
        <template v-for="g in groups" :key="g.name">
          <p v-if="g.name" class="sb-group">{{ g.name }}</p>
          <button v-for="it in g.list" :key="it.id"
                  class="sb-it"
                  :data-on="active === it.id ? 1 : 0"
                  :aria-current="active === it.id ? 'page' : undefined"
                  @click="pick(it.id)">
            <span class="ico" v-html="it.icon" />
            <span class="lbl">{{ it.label }}</span>
            <span v-if="it.badge" class="badge">{{ it.badge }}</span>
            <span v-if="it.dot" class="dot" />
          </button>
        </template>
      </nav>

      <!-- 底部账号 -->
      <div class="sb-foot">
        <div class="acct glass-1">
          <span class="av">{{ (user?.name || 'W').slice(0, 1).toUpperCase() }}</span>
          <span class="meta">
            <b class="trunc">{{ user?.name || '未登录' }}</b>
            <i class="trunc">{{ user?.plan || 'Free' }}</i>
          </span>
          <span class="led" :class="user ? 'on' : ''" />
        </div>
        <p class="sb-ver">v2.0.0 · 独立实例</p>
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* ---------- 遮罩 ---------- */
.sb-scrim{
  position:fixed;inset:0;z-index:79;
  background:radial-gradient(120% 90% at 20% 10%,rgba(0,0,0,.62),rgba(0,0,0,.78));
  -webkit-backdrop-filter:blur(2px);backdrop-filter:blur(2px);
  opacity:0;pointer-events:none;
  will-change:opacity;
}
@media (min-width:1024px){ .sb-scrim{display:none} }

/* ---------- 面板 ---------- */
.sb{
  position:fixed;left:0;top:0;bottom:0;z-index:80;
  width:var(--sb-w,268px);
  padding:calc(var(--safe-t) + 10px) 10px calc(var(--safe-b) + 10px);
  will-change:transform;
  touch-action:pan-y;
  border-radius:0 26px 26px 0;
  background:
    linear-gradient(200deg,rgba(255,255,255,.14) 0%,rgba(255,255,255,.045) 26%,rgba(255,255,255,.03) 62%,rgba(255,255,255,.08) 100%),
    rgba(15,15,19,.58);
  -webkit-backdrop-filter:blur(44px) saturate(190%) brightness(1.04);
  backdrop-filter:blur(44px) saturate(190%) brightness(1.04);
  box-shadow:
    inset 0 1px 0 rgba(255,255,255,.24),
    inset -1px 0 0 rgba(255,255,255,.07),
    inset 0 -1px 0 rgba(255,255,255,.05),
    1px 0 44px -8px rgba(0,0,0,.72);
}
/* 边缘 1px 高光渐变 */
.sb::before{
  content:"";position:absolute;inset:0;border-radius:inherit;padding:1px;pointer-events:none;
  background:linear-gradient(165deg,rgba(255,255,255,.55),rgba(255,255,255,.10) 26%,rgba(255,255,255,.02) 55%,rgba(255,255,255,.22));
  -webkit-mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);
  -webkit-mask-composite:xor;mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);mask-composite:exclude;
}
/* 顶部镜面 + 噪点 */
.sb::after{
  content:"";position:absolute;inset:0;border-radius:inherit;pointer-events:none;opacity:.6;
  background:radial-gradient(110% 60% at 8% -6%,rgba(255,255,255,.16),transparent 60%),var(--noise);
  background-blend-mode:screen,overlay;
}

@media (min-width:1024px){
  .sb{
    top:calc(var(--nav-h) + 12px);
    bottom:12px;left:12px;
    height:auto;border-radius:var(--r-xl);
    padding:14px 10px;
  }
  .sb-x{display:none}
}

/* ---------- 内部 ---------- */
.sb-inner{position:relative;z-index:1;height:100%;display:flex;flex-direction:column;gap:14px;min-height:0}

.sb-brand{display:flex;align-items:center;gap:10px;padding:6px 8px 10px}
.mark{color:var(--ac);display:grid;place-items:center;filter:drop-shadow(0 0 14px rgba(255,138,61,.28))}
.bname{display:flex;align-items:baseline;gap:6px;font-size:15px;letter-spacing:-.02em}
.bname strong{font-weight:700}
.bname em{
  font-style:normal;font-size:10.5px;font-weight:750;letter-spacing:.04em;
  color:var(--ac-2);padding:2px 6px;border-radius:6px;
  background:var(--ac-dim);border:1px solid var(--ac-line);
}
.sb-x{
  margin-left:auto;width:30px;height:30px;display:grid;place-items:center;
  border-radius:50%;color:var(--tx-3);
  background:rgba(255,255,255,.05);border:1px solid var(--line);
  transition:transform 240ms var(--e-out),background-color 180ms,color 180ms;
}
.sb-x:active{transform:scale(.9);background:rgba(255,255,255,.11);color:var(--tx-1)}

/* 导航 */
.sb-nav{position:relative;flex:1;min-height:0;overflow-y:auto;overflow-x:hidden;padding:2px 0 8px}
.ink{
  position:absolute;left:6px;right:6px;top:0;
  border-radius:11px;
  background:linear-gradient(160deg,rgba(255,255,255,.13),rgba(255,255,255,.055));
  border:1px solid rgba(255,255,255,.14);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.18),0 6px 18px -12px rgba(0,0,0,.8);
  will-change:transform,height;
  pointer-events:none;
}
.sb-group{
  font-size:10px;font-weight:700;letter-spacing:.15em;text-transform:uppercase;
  color:var(--tx-4);padding:14px 12px 7px;
}
.sb-it{
  position:relative;z-index:1;
  display:flex;align-items:center;gap:10px;
  width:100%;height:38px;padding:0 12px;margin:1px 0;
  border-radius:11px;color:var(--tx-2);
  font-size:13.5px;font-weight:560;letter-spacing:-.005em;
  transition:color 180ms var(--e-out-soft),transform 260ms var(--e-out);
  text-align:left;
}
.sb-it:hover{color:var(--tx-1)}
.sb-it:active{transform:scale(.975)}
.sb-it[data-on="1"]{color:var(--tx-1);font-weight:640}
.sb-it .ico{display:grid;place-items:center;width:18px;height:18px;flex:none;opacity:.86}
.sb-it .ico :deep(svg){width:16px;height:16px;stroke-width:1.7;stroke:currentColor;fill:none;stroke-linecap:round;stroke-linejoin:round}
.sb-it[data-on="1"] .ico{opacity:1;color:var(--ac-2)}
.lbl{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.badge{
  font-size:10px;font-weight:700;height:17px;padding:0 6px;border-radius:99px;
  display:inline-flex;align-items:center;
  background:var(--ac-dim);color:var(--ac-2);border:1px solid var(--ac-line);
}
.dot{width:5px;height:5px;border-radius:50%;background:var(--ac);flex:none;
  box-shadow:0 0 8px 1px rgba(255,138,61,.6)}

/* 底部账号 */
.sb-foot{padding-top:2px}
.acct{
  display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:14px;
  transition:transform 260ms var(--e-out),background-color 200ms;
}
.acct:active{transform:scale(.985)}
.av{
  width:30px;height:30px;flex:none;border-radius:50%;display:grid;place-items:center;
  font-size:12.5px;font-weight:700;color:#ffe8d8;
  background:linear-gradient(160deg,rgba(255,138,61,.34),rgba(255,138,61,.14));
  border:1px solid var(--ac-line);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.25);
}
.meta{display:flex;flex-direction:column;min-width:0;line-height:1.3}
.meta b{font-size:12.5px;font-weight:640}
.meta i{font-style:normal;font-size:10.5px;color:var(--tx-4)}
.led{
  width:7px;height:7px;border-radius:50%;flex:none;margin-left:auto;
  background:var(--tx-4);
}
.led.on{background:var(--ok);box-shadow:0 0 9px 1px rgba(84,201,138,.55);animation:k-breathe 2.4s var(--e-in-out) infinite}
.sb-ver{font-size:10px;color:var(--tx-4);padding:9px 12px 2px;letter-spacing:.02em}
</style>

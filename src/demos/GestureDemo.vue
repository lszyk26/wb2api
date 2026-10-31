<script setup>
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { drag, createSpring, rubberband, project, clamp } from '../lib/spring.js'

const props = defineProps({ kind: String, no: Number, tick: Number })

const root = ref(null)
let cleanup = []
let spring = null

function stopAll() {
  cleanup.forEach(fn => fn()); cleanup = []
  spring && spring.stop(); spring = null
}

/* ============================================================
   M17 抽屉跟手
   ============================================================ */
const W = 150            // 抽屉宽度
let dx = 0
const pageEl = ref(null), drawerEl = ref(null), scrim = ref(null)

function paintDrawer(v) {
  const p = clamp((v + W) / W, 0, 1)
  if (drawerEl.value) drawerEl.value.style.transform = `translate3d(${v}px,0,0)`
  if (pageEl.value) {
    pageEl.value.style.transform = `scale(${(1 - .06 * p).toFixed(3)}) translate3d(${(p * 10).toFixed(1)}px,0,0)`
    pageEl.value.style.borderRadius = (12 * p).toFixed(1) + 'px'
  }
  if (scrim.value) scrim.value.style.opacity = (p * .55).toFixed(2)
}
function springDrawer(to, v = 0) {
  spring && spring.stop()
  spring = createSpring(dx, {
    target: to, velocity: v, stiffness: 400, damping: 40,
    onUpdate: val => { dx = val; paintDrawer(val) },
  }).start()
}
function initDrawer() {
  paintDrawer(dx)
  cleanup.push(drag(root.value, {
    axis: 'x',
    onStart() { spring && spring.stop() },
    onMove({ x }) {
      let next = dx + x
      if (next > 0) next = rubberband(next, 46)
      if (next < -W) next = -W - rubberband(-(next + W), 46)
      dx = next; paintDrawer(next)
    },
    onEnd({ vx }) {
      const proj = dx + project(vx, .995)
      const open = vx > 480 ? true : vx < -480 ? false : proj > -W * .5
      springDrawer(open ? 0 : -W, vx * .5)
    },
  }))
}

/* ============================================================
   M18 下拉刷新
   ============================================================ */
const pullY = ref(0)
const refreshing = ref(false)
const THRESH = 46
const pullEl = ref(null), spinEl = ref(null)

function paintPull(y) {
  if (pullEl.value) pullEl.value.style.transform = `translate3d(0,${y}px,0)`
  if (spinEl.value) {
    spinEl.value.style.opacity = String(clamp(y / THRESH, 0, 1))
    spinEl.value.style.transform = `translateY(${Math.min(y, THRESH) * .5 - 22}px) rotate(${y * 2.4}deg)`
  }
}
function initPull() {
  paintPull(0)
  let base = 0
  cleanup.push(drag(root.value, {
    axis: 'y',
    onStart() { base = pullY.value; spring && spring.stop() },
    onMove({ y }) {
      if (refreshing.value) return
      let next = base + y
      if (next < 0) next = 0
      if (next > 0) next = rubberband(next, 118) * .92
      pullY.value = next
      paintPull(next)
    },
    onEnd({ vy }) {
      if (refreshing.value) return
      if (pullY.value > THRESH) {
        refreshing.value = true
        pullY.value = THRESH
        paintPull(THRESH)
        if (spinEl.value) spinEl.value.classList.add('spin')
        setTimeout(() => {
          pullY.value = 0
          spring && spring.stop()
          spring = createSpring(THRESH, {
            target: 0, stiffness: 320, damping: 30,
            onUpdate: v => { pullY.value = v; paintPull(v) },
            onComplete: () => {
              refreshing.value = false
              if (spinEl.value) spinEl.value.classList.remove('spin')
            },
          }).start()
        }, 850)
      } else {
        spring && spring.stop()
        spring = createSpring(pullY.value, {
          target: 0, velocity: vy * .4, stiffness: 360, damping: 30,
          onUpdate: v => { pullY.value = v; paintPull(v) },
        }).start()
      }
    },
  }))
}

/* ============================================================
   M19 侧滑操作
   ============================================================ */
const swipeX = ref(0)
const rowEl = ref(null)
const MAXL = -74
function paintSwipe(x) {
  if (rowEl.value) rowEl.value.style.transform = `translate3d(${x}px,0,0)`
}
function initSwipe() {
  paintSwipe(0)
  let base = 0
  cleanup.push(drag(root.value, {
    axis: 'x',
    onStart() { base = swipeX.value; spring && spring.stop() },
    onMove({ x }) {
      let next = base + x
      if (next > 0) next = rubberband(next, 30)
      if (next < MAXL) next = MAXL - rubberband(-(next - MAXL), 30)
      swipeX.value = next; paintSwipe(next)
    },
    onEnd({ vx }) {
      const open = vx < -320 ? true : vx > 320 ? false : swipeX.value < MAXL * .5
      const to = open ? MAXL : 0
      spring = createSpring(swipeX.value, {
        target: to, velocity: vx * .4, stiffness: 420, damping: 38,
        onUpdate: v => { swipeX.value = v; paintSwipe(v) },
      }).start()
    },
  }))
}
function resetSwipe() {
  spring && spring.stop()
  spring = createSpring(swipeX.value, {
    target: 0, stiffness: 420, damping: 38,
    onUpdate: v => { swipeX.value = v; paintSwipe(v) },
  }).start()
}

/* ============================================================
   M20 长按菜单
   ============================================================ */
const menu = ref(false)
const pressed = ref(false)
let lt = null
function downLong() {
  pressed.value = true
  clearTimeout(lt)
  lt = setTimeout(() => {
    menu.value = true
    if (navigator.vibrate) navigator.vibrate(8)
  }, 450)
}
function upLong() {
  pressed.value = false
  clearTimeout(lt)
}
function initLong() { /* 用模板事件 */ }

/* ============================================================
   M21 双指缩放
   ============================================================ */
const pz = ref(1)
const pzEl = ref(null)
let pointers = new Map()
let startDist = 0, startScale = 1
function paintPz(s) { if (pzEl.value) pzEl.value.style.transform = `scale(${s})` }
function pd(e) {
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  if (pointers.size === 2) {
    const [a, b] = [...pointers.values()]
    startDist = Math.hypot(a.x - b.x, a.y - b.y)
    startScale = pz.value
    spring && spring.stop()
  }
}
function pm(e) {
  if (!pointers.has(e.pointerId)) return
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  if (pointers.size === 2 && startDist > 0) {
    const [a, b] = [...pointers.values()]
    const d = Math.hypot(a.x - b.x, a.y - b.y)
    if (e.cancelable) e.preventDefault()
    const next = clamp(startScale * (d / startDist), .7, 2.1)
    pz.value = next; paintPz(next)
  }
}
function pu(e) {
  pointers.delete(e.pointerId)
  if (pointers.size < 2) {
    startDist = 0
    spring = createSpring(pz.value, {
      target: 1, stiffness: 340, damping: 26,
      onUpdate: v => { pz.value = v; paintPz(v) },
    }).start()
  }
}
let tapToggle = false
function tapPinch() {
  tapToggle = !tapToggle
  const to = tapToggle ? 1.55 : 1
  spring && spring.stop()
  spring = createSpring(pz.value, {
    target: to, stiffness: 320, damping: 24,
    onUpdate: v => { pz.value = v; paintPz(v) },
  }).start()
}
function initPinch() {
  paintPz(1)
  const el = root.value
  el.addEventListener('pointerdown', pd, { passive: true })
  el.addEventListener('pointermove', pm, { passive: false })
  el.addEventListener('pointerup', pu, { passive: true })
  el.addEventListener('pointercancel', pu, { passive: true })
  cleanup.push(() => {
    el.removeEventListener('pointerdown', pd)
    el.removeEventListener('pointermove', pm)
    el.removeEventListener('pointerup', pu)
    el.removeEventListener('pointercancel', pu)
  })
}

/* ============================================================
   M22 拖拽排序
   ============================================================ */
const ROW_H = 27, GAP = 5
const ord = ref([{ id: 'a', t: 'aio' }, { id: 'b', t: 'glm-5.2' }, { id: 'c', t: 'kimi-k3' }, { id: 'd', t: 'ds-v4' }])
const orderEl = ref(null)
let dragIdx = -1, dragStartY = 0, dragCurY = 0
const dragY = ref(0)

function initReorder() {
  const el = orderEl.value
  if (!el) return
  const step = ROW_H + GAP
  const items = () => [...el.children]

  cleanup.push(drag(el, {
    axis: 'y',
    onStart(ev) {
      const target = ev.event.target.closest('[data-ri]')
      if (!target) return
      dragIdx = Number(target.dataset.ri)
      dragStartY = 0; dragCurY = 0
      target.classList.add('dragging')
      target.style.zIndex = 3
      spring && spring.stop()
    },
    onMove({ y }) {
      if (dragIdx < 0) return
      dragCurY = y
      const kids = items()
      const dragged = kids[dragIdx]
      dragged.style.transform = `translate3d(0,${y}px,0) scale(1.03)`
      const shift = Math.round(y / step)
      kids.forEach((k, i) => {
        if (i === dragIdx) return
        let ty = 0
        if (shift > 0 && i > dragIdx && i <= dragIdx + shift) ty = -step
        if (shift < 0 && i < dragIdx && i >= dragIdx + shift) ty = step
        k.style.transform = ty ? `translate3d(0,${ty}px,0)` : ''
      })
    },
    onEnd() {
      if (dragIdx < 0) return
      const step2 = step
      const shift = Math.round(dragCurY / step2)
      const kids = items()
      kids.forEach(k => {
        k.style.transform = ''
        k.style.zIndex = ''
        k.classList.remove('dragging')
      })
      const arr = ord.value.slice()
      const [moved] = arr.splice(dragIdx, 1)
      arr.splice(clamp(dragIdx + shift, 0, arr.length), 0, moved)
      ord.value = arr
      dragIdx = -1
      nextTick(() => {
        items().forEach((k, i) => {
          k.animate([{ transform: 'scale(.98)' }, { transform: 'none' }], { duration: 260, delay: i * 14, easing: 'cubic-bezier(.16,1,.3,1)' })
        })
      })
    },
  }))
}

/* ============================================================
   M23 底部吸附面板
   ============================================================ */
const SHEET_H = 116
const stops = [0, 58, 116]   // 距底部的展开高度
let sy = 0
const sheetEl = ref(null)
function paintSheet(v) {
  if (sheetEl.value) sheetEl.value.style.transform = `translate3d(0,${SHEET_H - v}px,0)`
}
function springSheet(to, vel = 0) {
  spring && spring.stop()
  spring = createSpring(sy, {
    target: to, velocity: vel, stiffness: 400, damping: 40,
    onUpdate: v => { sy = v; paintSheet(v) },
  }).start()
}
function initSheet() {
  sy = 58; paintSheet(sy)
  cleanup.push(drag(sheetEl.value, {
    axis: 'y',
    onStart() { spring && spring.stop() },
    onMove({ y }) {
      let next = sy - y
      if (next < 0) next = -rubberband(-next, 30)
      if (next > SHEET_H) next = SHEET_H + rubberband(next - SHEET_H, 30)
      sy = next; paintSheet(next)
    },
    onEnd({ vy }) {
      const v = -vy
      let to
      if (v > 500) to = stops.find(s => s > sy + 20) ?? SHEET_H
      else if (v < -500) to = [...stops].reverse().find(s => s < sy - 20) ?? 0
      else to = stops.reduce((a, b) => Math.abs(b - sy) < Math.abs(a - sy) ? b : a, 0)
      springSheet(to, -vy * .4)
    },
  }))
}

/* ============================================================
   M24 边缘弹性
   ============================================================ */
let bY = 0
const bounceEl = ref(null)
const listScroller = ref(null)
function paintBounce(y) {
  if (bounceEl.value) bounceEl.value.style.transform = `translate3d(0,${y}px,0)`
}
function initBounce() {
  paintBounce(0)
  const sc = listScroller.value
  let base = 0
  cleanup.push(drag(sc, {
    axis: 'y',
    onStart() { base = bY },
    onMove({ y }) {
      if (sc.scrollTop > 0) return
      let next = base + y
      if (next < 0) next = rubberband(next, 60) * .8
      if (next > 0) next = 0
      bY = next; paintBounce(next)
    },
    onEnd({ vy }) {
      if (bY === 0) return
      spring = createSpring(bY, {
        target: 0, velocity: vy * .35, stiffness: 300, damping: 24,
        onUpdate: v => { bY = v; paintBounce(v) },
      }).start()
    },
  }))
}

/* ============================================================
   挂载分派
   ============================================================ */
const initMap = {
  drawer: initDrawer, pull: initPull, swipe: initSwipe, longpress: initLong,
  pinch: initPinch, reorder: initReorder, sheet: initSheet, bounce: initBounce,
}
let inited = null

onMounted(() => {
  inited = props.kind
  nextTick(() => { const f = initMap[inited]; f && f() })
})
onBeforeUnmount(stopAll)
</script>

<template>
  <!-- ============ M17 抽屉 ============ -->
  <div v-if="kind === 'drawer'" ref="root" class="stage g">
    <div ref="pageEl" class="page">
      <div class="pl" style="width:52%" /><div class="pl" style="width:78%" /><div class="pl" style="width:64%" />
    </div>
    <div ref="scrim" class="dim-layer" />
    <div ref="drawerEl" class="drawer">
      <div class="dl" /><div class="dl" style="width:58%" /><div class="dl" style="width:70%" />
    </div>
    <span class="hint">左右拖我</span>
  </div>

  <!-- ============ M18 下拉刷新 ============ -->
  <div v-else-if="kind === 'pull'" ref="root" class="stage g pull-stage">
    <div ref="spinEl" class="spinner" :class="{ spin: refreshing }">
      <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2.4">
        <path d="M20 11a8 8 0 10-2.3 5.7" stroke-linecap="round"/>
      </svg>
    </div>
    <div ref="pullEl" class="pull-list">
      <div v-for="i in 4" :key="i" class="pl2"><span>{{ i === 1 ? '今日请求 1,284' : i === 2 ? '平均延迟 61ms' : i === 3 ? '池内可用 3' : '错误率 0.6%' }}</span></div>
    </div>
    <span class="hint">往下拉 → 松手刷新</span>
  </div>

  <!-- ============ M19 侧滑 ============ -->
  <div v-else-if="kind === 'swipe'" ref="root" class="stage g swipe-stage">
    <div class="swipe-bg"><button class="del" @click.stop="resetSwipe">删除</button></div>
    <div ref="rowEl" class="swipe-row">
      <span class="av">W</span>
      <span class="sw-t">key …a92f</span>
      <span class="sw-s">可用</span>
    </div>
    <span class="hint">向左拖出操作</span>
  </div>

  <!-- ============ M20 长按 ============ -->
  <div v-else-if="kind === 'longpress'" class="stage g" @pointerdown.stop="downLong"
       @pointerup="upLong" @pointerleave="upLong" @pointercancel="upLong">
    <div class="lp-target" :class="{ held: pressed }">
      <span class="av">01</span>
      <div class="lp-txt"><b>workbuddy · intl</b><i>长按这一行</i></div>
    </div>
    <Transition name="menu">
      <div v-if="menu" class="ctx glass glass-3" @click.stop="menu = false">
        <button class="ctx-it">复制 API Key</button>
        <button class="ctx-it">查看用量</button>
        <button class="ctx-it err">移出账号池</button>
      </div>
    </Transition>
  </div>

  <!-- ============ M21 双指缩放 ============ -->
  <div v-else-if="kind === 'pinch'" ref="root" class="stage g pinch-stage">
    <div ref="pzEl" class="pz-box" @click.stop="tapPinch">
      <span class="pz-t">2×</span>
    </div>
    <span class="hint">双指张合 · 点我切档</span>
  </div>

  <!-- ============ M22 拖拽排序 ============ -->
  <div v-else-if="kind === 'reorder'" class="stage g reorder-stage">
    <div ref="orderEl" class="ord">
      <div v-for="(r, i) in ord" :key="r.id" class="ord-it" :data-ri="i">{{ r.t }}</div>
    </div>
    <span class="hint">按住竖直拖 · 其余让位</span>
  </div>

  <!-- ============ M23 底部面板 ============ -->
  <div v-else-if="kind === 'sheet'" class="stage g sheet-stage">
    <div class="sheet-body">
      <div class="pl" style="width:56%" /><div class="pl" style="width:34%" />
    </div>
    <div ref="sheetEl" class="sheet">
      <span class="grab" />
      <b>账号详情</b>
      <div class="sh-row"><span>积分</span><b>1,240</b></div>
      <div class="sh-row"><span>延迟</span><b>61ms</b></div>
    </div>
  </div>

  <!-- ============ M24 边缘弹性 ============ -->
  <div v-else-if="kind === 'bounce'" ref="root" class="stage g bounce-stage">
    <div ref="listScroller" class="bounce-scroll no-sb">
      <div ref="bounceEl" class="bounce-in">
        <div v-for="i in 5" :key="i" class="pl2 small"><span>记录 #{{ i }}</span></div>
      </div>
    </div>
    <span class="hint">滚到顶继续下拉</span>
  </div>

  <div v-else class="center"><span class="hint">—</span></div>
</template>

<style scoped>
.stage{position:relative;width:100%;height:100%;overflow:hidden;display:grid;place-items:center}
.stage.g{touch-action:none}
.hint{position:absolute;bottom:5px;left:0;right:0;text-align:center;font-size:9px;color:var(--tx-4);
  pointer-events:none;z-index:5}
.center{display:grid;place-items:center;height:100%}

/* ---- M17 ---- */
.page{
  position:absolute;inset:8px;border-radius:10px;padding:10px;
  background:linear-gradient(160deg,rgba(255,255,255,.075),rgba(255,255,255,.025));
  border:1px solid rgba(255,255,255,.08);
  display:flex;flex-direction:column;gap:6px;
  transition:border-radius 120ms linear;
  will-change:transform;
}
.pl{height:5px;border-radius:2px;background:rgba(255,255,255,.14)}
.dim-layer{position:absolute;inset:0;background:#000;opacity:0;pointer-events:none}
.drawer{
  position:absolute;left:0;top:0;bottom:0;width:150px;padding:12px 10px;
  background:linear-gradient(200deg,rgba(255,255,255,.14),rgba(255,255,255,.04) 40%,rgba(255,255,255,.08));
  -webkit-backdrop-filter:blur(18px) saturate(180%);backdrop-filter:blur(18px) saturate(180%);
  border-right:1px solid rgba(255,255,255,.14);
  box-shadow:inset 1px 0 0 rgba(255,255,255,.18),12px 0 30px -14px rgba(0,0,0,.8);
  display:flex;flex-direction:column;gap:7px;z-index:3;
  will-change:transform;transform:translate3d(-150px,0,0);
}
.dl{height:7px;border-radius:3px;background:rgba(255,255,255,.16)}

/* ---- M18 ---- */
.pull-stage{align-content:start;padding-top:0;grid-template-rows:1fr}
.spinner{
  position:absolute;top:6px;left:50%;margin-left:-9px;width:18px;height:18px;
  display:grid;place-items:center;color:var(--ac);opacity:0;z-index:4;
  transition:opacity 120ms;
}
.spinner.spin svg{animation:spin 780ms linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
.pull-list{width:100%;padding:6px 11px 0;display:flex;flex-direction:column;gap:5px;will-change:transform}
.pl2{
  height:22px;border-radius:7px;padding:0 9px;display:flex;align-items:center;
  background:rgba(255,255,255,.045);border:1px solid rgba(255,255,255,.06);
  font-size:9.5px;color:var(--tx-3);
}
.pl2.small{height:20px}

/* ---- M19 ---- */
.swipe-stage{place-items:center;padding:0 11px}
.swipe-bg{
  position:absolute;inset:8px 11px;border-radius:9px;background:rgba(240,103,95,.14);
  border:1px solid rgba(240,103,95,.22);
  display:flex;align-items:center;justify-content:flex-end;padding-right:10px;
}
.del{font-size:10px;font-weight:650;color:#f6b1ac}
.swipe-row{
  position:relative;z-index:2;width:100%;height:34px;border-radius:9px;
  display:flex;align-items:center;gap:8px;padding:0 9px;
  background:linear-gradient(160deg,rgba(38,38,44,.98),rgba(28,28,33,.98));
  border:1px solid rgba(255,255,255,.11);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.10);
  will-change:transform;
}
.sw-t{font-size:10px;font-family:var(--mono);color:var(--tx-2);flex:1}
.sw-s{font-size:9px;color:#a9e8c5}
.av{width:19px;height:19px;flex:none;border-radius:6px;display:grid;place-items:center;
  font-size:9px;font-weight:700;color:#ffcba6;background:var(--ac-dim);
  border:1px solid rgba(255,138,61,.24)}

/* ---- M20 ---- */
.lp-target{
  display:flex;align-items:center;gap:9px;width:calc(100% - 24px);
  padding:8px 9px;border-radius:10px;
  background:linear-gradient(160deg,rgba(255,255,255,.07),rgba(255,255,255,.025));
  border:1px solid rgba(255,255,255,.09);
  transition:transform 320ms var(--e-out),border-color 240ms,box-shadow 300ms;
  -webkit-user-select:none;user-select:none;
}
.lp-target.held{transform:scale(.96);border-color:var(--ac-line);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.14),0 0 0 3px rgba(255,138,61,.08)}
.lp-txt{display:flex;flex-direction:column;line-height:1.35;min-width:0}
.lp-txt b{font-size:10px;font-weight:600}
.lp-txt i{font-style:normal;font-size:8.5px;color:var(--tx-4)}
.ctx{
  position:absolute;z-index:9;width:150px;padding:5px;border-radius:12px;
  display:flex;flex-direction:column;gap:1px;
}
.ctx-it{
  height:29px;padding:0 9px;border-radius:7px;text-align:left;
  font-size:10.5px;color:var(--tx-2);
  transition:background-color 140ms,color 140ms;
}
.ctx-it:hover{background:rgba(255,255,255,.08);color:var(--tx-1)}
.ctx-it.err{color:#f6b1ac}
.ctx-it.err:hover{background:rgba(240,103,95,.13)}
.menu-enter-active{animation:k-scale-spring 400ms cubic-bezier(.34,1.45,.64,1) both}
.menu-leave-active{animation:k-scale-in 160ms var(--e-in-out) reverse both}

/* ---- M21 ---- */
.pinch-stage{place-items:center}
.pz-box{
  width:92px;height:64px;border-radius:12px;display:grid;place-items:center;
  background:linear-gradient(160deg,rgba(255,138,61,.18),rgba(255,138,61,.06));
  border:1px solid rgba(255,138,61,.28);
  will-change:transform;
}
.pz-t{font-size:15px;font-weight:720;color:#ffcba6;letter-spacing:-.02em}

/* ---- M22 ---- */
.reorder-stage{padding:9px 11px}
.ord{width:100%;display:flex;flex-direction:column;gap:5px;position:relative}
.ord-it{
  position:relative;height:27px;border-radius:8px;display:flex;align-items:center;padding:0 9px;
  font-size:10px;font-family:var(--mono);color:var(--tx-2);
  background:rgba(255,255,255,.05);border:1px solid rgba(255,255,255,.07);
  transition:transform 300ms var(--e-out),background-color 200ms;
  will-change:transform;touch-action:none;
}
.ord-it.dragging{
  background:rgba(255,138,61,.13);border-color:var(--ac-line);color:#ffd6b8;
  box-shadow:0 10px 22px -10px rgba(0,0,0,.9);transition:none;
}

/* ---- M23 ---- */
.sheet-stage{align-content:end}
.sheet-body{padding:10px 11px;display:flex;flex-direction:column;gap:6px;width:100%}
.sheet{
  position:absolute;left:0;right:0;bottom:0;height:116px;
  border-radius:16px 16px 0 0;padding:8px 12px;
  background:linear-gradient(180deg,rgba(255,255,255,.11),rgba(255,255,255,.05));
  -webkit-backdrop-filter:blur(24px) saturate(180%);backdrop-filter:blur(24px) saturate(180%);
  border-top:1px solid rgba(255,255,255,.16);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.22),0 -18px 40px -20px rgba(0,0,0,.9);
  will-change:transform;z-index:4;
  display:flex;flex-direction:column;gap:5px;
}
.grab{width:30px;height:3.5px;border-radius:99px;background:rgba(255,255,255,.26);
  margin:0 auto 3px;flex:none}
.sheet > b{font-size:11px;font-weight:640}
.sh-row{display:flex;justify-content:space-between;font-size:10px;color:var(--tx-3)}
.sh-row b{color:var(--tx-1);font-variant-numeric:tabular-nums}

/* ---- M24 ---- */
.bounce-stage{padding:8px 11px;align-content:start}
.bounce-scroll{
  width:100%;height:72px;overflow-y:auto;overscroll-behavior:contain;
  -webkit-overflow-scrolling:touch;
}
.bounce-in{display:flex;flex-direction:column;gap:5px;will-change:transform}
</style>

<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'
import { createSpring } from '../lib/spring.js'

const props = defineProps({ kind: String, no: Number, tick: Number })
const k = computed(() => props.no + '-' + props.tick)

/* ---------------- M51 墨迹指示器 ---------------- */
const tabs = ['概览', '账号', '用量']
const ti = ref(0)
const inkEl = ref(null)
const tabWrap = ref(null)
let inkTop = 0

function paintInk(idx, animate = true) {
  const wrap = tabWrap.value
  if (!wrap) return
  const btns = [...wrap.querySelectorAll('.tab')]
  const b = btns[idx]
  if (!b) return
  const left = b.offsetLeft
  const w = b.offsetWidth
  if (!animate) {
    if (inkEl.value) {
      inkEl.value.style.width = w + 'px'
      inkEl.value.style.transform = `translate3d(${left}px,0,0)`
    }
    return
  }
  const curW = inkEl.value?.offsetWidth || w
  const curL = inkEl.value ? new DOMMatrixReadOnly(getComputedStyle(inkEl.value).transform).m41 : left
  inkEl.value.style.width = w + 'px'
  createSpring({ l: curL, w: curW }, {
    target: { l: left, w },
    stiffness: 460, damping: 38,
    onUpdate: v => {
      if (!inkEl.value) return
      inkEl.value.style.transform = `translate3d(${v.l}px,0,0)`
      inkEl.value.style.width = v.w + 'px'
    },
  }).start()
}

function pickTab(i) {
  ti.value = i
  paintInk(i)
}

onMounted(() => nextTick(() => paintInk(0, false)))

/* ---------------- M52 面包屑 ---------------- */
const crumbs = [['控制台'], ['控制台', '账号池'], ['控制台', '账号池', 'workbuddy · intl']]
const ci = ref(0)
function deepen() { ci.value = (ci.value + 1) % crumbs.length }

/* ---------------- M53 汉堡 ---------------- */
const burger = ref(false)

/* ---------------- M54 顶栏隐藏 ---------------- */
const scroller = ref(null)
const barHidden = ref(false)
let lastY = 0, acc = 0
function onScroll(e) {
  const y = e.target.scrollTop
  const dy = y - lastY; acc += dy
  if (y < 12) { barHidden.value = false; acc = 0 }
  else if (acc > 22) { barHidden.value = true; acc = 0 }
  else if (acc < -14) { barHidden.value = false; acc = 0 }
  lastY = y
}

/* ---------------- M55 锚点 ---------------- */
const anchors = ['请求参数', '响应结构', '错误码']
const ai = ref(0)

/* ---------------- M56 进度 ---------------- */
const prog = ref(0)
const progScroller = ref(null)
function onProgScroll(e) {
  const el = e.target
  const max = el.scrollHeight - el.clientHeight
  prog.value = max > 0 ? el.scrollTop / max : 0
}
</script>

<template>
  <!-- ============ M51 墨迹 ============ -->
  <div v-if="kind === 'ink'" class="c" :key="k">
    <div class="tabs" ref="tabWrap">
      <span class="ink" ref="inkEl" />
      <button v-for="(t, i) in tabs" :key="t" class="tab"
              :data-on="ti === i ? 1 : 0" @click.stop="pickTab(i)">{{ t }}</button>
    </div>
    <span class="hint">选中背景像墨水滑过去</span>
  </div>

  <!-- ============ M52 面包屑 ============ -->
  <div v-else-if="kind === 'crumb'" class="c" :key="k" @click.stop="deepen">
    <div class="crumbs">
      <template v-for="(c, i) in crumbs[ci]" :key="c">
        <span v-if="i" class="sep">/</span>
        <span class="cr" :class="{ cur: i === crumbs[ci].length - 1 }">{{ c }}</span>
      </template>
    </div>
    <span class="hint">点我下钻</span>
  </div>

  <!-- ============ M53 汉堡 ============ -->
  <div v-else-if="kind === 'burger'" class="c" :key="k">
    <button class="bg-btn" :class="{ x: burger }" @click.stop="burger = !burger" aria-label="菜单">
      <span /><span /><span />
    </button>
    <span class="hint">三条线 → 叉</span>
  </div>

  <!-- ============ M54 滚动隐藏顶栏 ============ -->
  <div v-else-if="kind === 'hidebar'" class="c hidebar" :key="k">
    <div class="fake-bar" :class="{ out: barHidden }">
      <span class="fb-dot" /><b>运行概览</b>
    </div>
    <div class="fake-scroll no-sb" ref="scroller" @scroll="onScroll">
      <div v-for="i in 9" :key="i" class="fake-row">
        <span class="fr-l" />{{ ['请求 #' + i, '延迟 ' + (28 + i * 11) + 'ms', '账号 intl'][i % 3] }}
      </div>
    </div>
    <span class="hint">下滑隐藏 · 上滑立现</span>
  </div>

  <!-- ============ M55 锚点 ============ -->
  <div v-else-if="kind === 'anchor'" class="c" :key="k">
    <div class="anchors">
      <button v-for="(a, i) in anchors" :key="a" class="anc"
              :class="{ on: ai === i }" @click.stop="ai = i">{{ a }}</button>
    </div>
    <span class="hint">下划线滑入当前项</span>
  </div>

  <!-- ============ M56 进度 ============ -->
  <div v-else-if="kind === 'progress'" class="c progress-c" :key="k">
    <div class="pg-bar"><i :style="{ transform: `scaleX(${prog})` }" /></div>
    <div class="pg-scroll no-sb" ref="progScroller" @scroll="onProgScroll">
      <div v-for="i in 10" :key="i" class="pg-row">{{ i }}</div>
    </div>
    <span class="hint">顶部 1.5px 线性进度</span>
  </div>

  <div v-else class="center"><span class="hint">—</span></div>
</template>

<style scoped>
.c{position:relative;width:100%;height:100%;display:flex;align-items:center;justify-content:center;
  flex-direction:column;gap:8px;overflow:hidden}
.hint{font-size:9px;color:var(--tx-4)}
.center{display:grid;place-items:center;height:100%}

/* M51 */
.tabs{
  position:relative;display:flex;padding:3px;border-radius:10px;
  background:rgba(255,255,255,.045);border:1px solid var(--line);
}
.ink{
  position:absolute;top:3px;left:0;height:calc(100% - 6px);border-radius:7px;
  background:linear-gradient(160deg,rgba(255,255,255,.15),rgba(255,255,255,.07));
  border:1px solid rgba(255,255,255,.14);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.18),0 4px 12px -8px rgba(0,0,0,.8);
  will-change:transform,width;
}
.tab{
  position:relative;z-index:1;height:26px;padding:0 12px;border-radius:7px;
  font-size:10.5px;font-weight:600;color:var(--tx-3);
  transition:color 220ms var(--e-out-soft);
}
.tab[data-on="1"]{color:var(--tx-1)}
.ink{pointer-events:none}

/* M52 */
.crumbs{display:flex;align-items:center;gap:5px;font-size:10px;color:var(--tx-4)}
.sep{opacity:.45;font-size:9px}
.cr{animation:k-fade-up 320ms var(--e-out) both;white-space:nowrap;
  max-width:76px;overflow:hidden;text-overflow:ellipsis}
.cr.cur{color:var(--tx-1);font-weight:600}

/* M53 */
.bg-btn{
  width:38px;height:38px;border-radius:11px;display:grid;place-content:center;gap:4px;
  background:rgba(255,255,255,.055);border:1px solid var(--line);
  transition:background-color 200ms,transform 240ms var(--e-out);
}
.bg-btn:active{transform:scale(.94)}
.bg-btn span{
  display:block;width:15px;height:1.6px;border-radius:2px;background:var(--tx-1);
  transition:transform 340ms var(--e-out),opacity 180ms,width 340ms;
}
.bg-btn span:nth-child(2){width:11px;margin-left:-4px}
.bg-btn.x span:nth-child(1){transform:translateY(5.6px) rotate(45deg)}
.bg-btn.x span:nth-child(2){opacity:0;transform:translateX(-7px)}
.bg-btn.x span:nth-child(3){transform:translateY(-5.6px) rotate(-45deg)}

/* M54 */
.hidebar{align-items:stretch;justify-content:flex-start;gap:0;padding:0}
.fake-bar{
  height:26px;display:flex;align-items:center;gap:6px;padding:0 9px;
  background:rgba(20,20,25,.85);border-bottom:1px solid var(--line);
  transition:transform 320ms var(--e-out);
  flex:none;z-index:2;
}
.fake-bar.out{transform:translateY(-100%)}
.fb-dot{width:5px;height:5px;border-radius:50%;background:var(--ok)}
.fake-bar b{font-size:9.5px;font-weight:600;color:var(--tx-2)}
.fake-scroll{flex:1;overflow-y:auto;padding:4px 8px;display:flex;flex-direction:column;gap:4px}
.fake-row{
  height:20px;flex:none;border-radius:6px;background:rgba(255,255,255,.04);
  display:flex;align-items:center;gap:6px;padding:0 7px;font-size:9px;color:var(--tx-4);
}
.fr-l{width:5px;height:5px;border-radius:50%;background:rgba(255,255,255,.2);flex:none}

/* M55 */
.anchors{display:flex;gap:14px}
.anc{
  position:relative;font-size:10.5px;color:var(--tx-3);padding-bottom:5px;
  transition:color 220ms;
}
.anc::after{
  content:"";position:absolute;left:0;right:0;bottom:0;height:1.5px;border-radius:2px;
  background:var(--ac);transform:scaleX(0);transform-origin:left;
  transition:transform 360ms var(--e-out);
}
.anc.on{color:var(--tx-1)}
.anc.on::after{transform:scaleX(1)}

/* M56 */
.progress-c{padding-top:0;gap:0;align-items:stretch;justify-content:flex-start}
.pg-bar{height:1.5px;background:rgba(255,255,255,.07);position:relative;flex:none;z-index:2}
.pg-bar i{
  display:block;height:100%;width:100%;transform-origin:left center;
  background:linear-gradient(90deg,var(--ac),var(--ac-2));
  transition:transform 80ms linear;
}
.pg-scroll{flex:1;overflow-y:auto;padding:5px 9px;display:flex;flex-direction:column;gap:4px}
.pg-row{height:19px;flex:none;border-radius:6px;background:rgba(255,255,255,.04);
  display:grid;place-items:center;font-size:9px;color:var(--tx-4)}
</style>

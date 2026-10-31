<script setup>
import { ref, computed, onBeforeUnmount } from 'vue'

const props = defineProps({
  kind: String,
  body: String,
  cls: String,
  no: Number,
  tick: Number,
})

/* ---------------- 通用：重播 key ---------------- */
const k = computed(() => props.no + '-' + props.tick)

/* ---------------- M08 骨架 ---------------- */
const filled = ref(false)
let t1 = null
function runSkeleton() {
  filled.value = false
  clearTimeout(t1)
  t1 = setTimeout(() => { filled.value = true }, 760)
}
runSkeleton()

/* ---------------- M25 按压 ---------------- */
const pressed = ref(false)

/* ---------------- M26 点赞 ---------------- */
const liked = ref(false)
const burst = ref(0)
const parts = Array.from({ length: 9 }, (_, i) => {
  const a = (i / 9) * Math.PI * 2 - Math.PI / 2
  const d = 22 + (i % 3) * 6
  return { bx: Math.cos(a) * d + 'px', by: Math.sin(a) * d + 'px', dl: (i % 3) * 26 + 'ms' }
})
function like() {
  liked.value = !liked.value
  if (liked.value) burst.value++
}

/* ---------------- M27 复制 ---------------- */
const copied = ref(false)
let t2 = null
function copy() {
  copied.value = true
  clearTimeout(t2)
  t2 = setTimeout(() => { copied.value = false }, 1600)
}

/* ---------------- M28 开关 ---------------- */
const sw = ref(true)

/* ---------------- M29 复选 ---------------- */
const cb = ref(false)

/* ---------------- M31 抖动 ---------------- */
const shakeKey = ref(0)
function doShake() { shakeKey.value++ }

/* ---------------- M32 Toast ---------------- */
const toast = ref(false)
let t3 = null
function showToast() {
  toast.value = true
  clearTimeout(t3)
  t3 = setTimeout(() => { toast.value = false }, 2100)
}

/* ---------------- M33 / M40 数字 ---------------- */
const num = ref(1284)
function bump() { num.value += Math.floor(Math.random() * 340) + 8 }

/* ---------------- M34 液体进度 ---------------- */
const pct = ref(34)
function fill() { pct.value = pct.value > 80 ? 22 : Math.min(96, pct.value + 18) }

onBeforeUnmount(() => { clearTimeout(t1); clearTimeout(t2); clearTimeout(t3) })
</script>

<template>
  <!-- ============ A 转场：通用元素 ============ -->
  <div v-if="['a-up','a-spring','a-depth','a-blur','a-circle','a-flipx'].includes(kind === 'css' ? cls : '')"
       :key="k" class="demo-el" :class="cls">
    <div v-if="body === 'card'" class="mini glass glass-1">
      <span class="dotrow"><i/><i/><i/></span>
      <b>卡片内容</b>
      <span class="bar w1" /><span class="bar w2" />
    </div>
    <div v-else class="bigtext">内容揭示</div>
  </div>

  <div v-else-if="kind === 'css'" :key="k" class="demo-el" :class="cls">
    <div class="mini glass glass-1"><b>卡片内容</b><span class="bar w1" /></div>
  </div>

  <!-- ============ M08 骨架 ============ -->
  <div v-else-if="kind === 'skeleton'" class="skel-wrap" @click.stop="runSkeleton">
    <Transition name="sk">
      <div v-if="!filled" key="s" class="skel-card">
        <span class="skel s1" /><span class="skel s2" /><span class="skel s3" />
      </div>
      <div v-else key="f" class="mini glass glass-1 real">
        <span class="dotrow"><i/><i/><i/></span>
        <b>真实内容</b>
        <span class="bar w1" /><span class="bar w2" />
      </div>
    </Transition>
  </div>

  <!-- ============ D25 按压 ============ -->
  <div v-else-if="kind === 'press'" class="center">
    <button class="btn pri demo-btn" :class="{ held: pressed }"
            @pointerdown="pressed = true" @pointerup="pressed = false"
            @pointerleave="pressed = false">
      按住我
    </button>
    <span class="hint">松手回弹 240ms</span>
  </div>

  <!-- ============ D26 点赞粒子 ============ -->
  <div v-else-if="kind === 'like'" class="center">
    <button class="like-btn" :class="{ on: liked }" @click.stop="like" aria-label="点赞">
      <svg viewBox="0 0 24 24" width="26" height="26"
           :fill="liked ? 'var(--ac)' : 'none'" :stroke="liked ? 'var(--ac)' : 'currentColor'"
           stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 20.2s-7.6-4.6-7.6-9.6A4.4 4.4 0 0112 7.6a4.4 4.4 0 017.6 3c0 5-7.6 9.6-7.6 9.6z"/>
      </svg>
      <span class="pop" :key="burst" v-if="liked">
        <i v-for="(p, i) in parts" :key="i" class="pt"
           :style="{ '--bx': p.bx, '--by': p.by, animationDelay: p.dl }" />
      </span>
    </button>
    <span class="hint">{{ liked ? '已点赞' : '点一下' }}</span>
  </div>

  <!-- ============ D27 复制对勾 ============ -->
  <div v-else-if="kind === 'check'" class="center">
    <button class="copy-btn" :class="{ done: copied }" @click.stop="copy">
      <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor"
           stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
        <Transition name="ic" mode="out-in">
          <g v-if="!copied" key="a">
            <rect x="8.4" y="8.4" width="11" height="11" rx="2.4"/>
            <path d="M15.6 5.4H6.6a1.2 1.2 0 00-1.2 1.2v9"/>
          </g>
          <path v-else key="b" d="M4.8 12.4l4.6 4.4L19.2 7" class="d-check run" style="stroke-dasharray:26"/>
        </Transition>
      </svg>
      <span>{{ copied ? '已复制' : '复制密钥' }}</span>
    </button>
  </div>

  <!-- ============ D28 开关 ============ -->
  <div v-else-if="kind === 'switch'" class="center">
    <button class="sw" :class="{ on: sw }" @click.stop="sw = !sw" role="switch" :aria-checked="sw">
      <span class="track"><span class="knob" /></span>
    </button>
    <span class="hint">{{ sw ? '已开启' : '已关闭' }}</span>
  </div>

  <!-- ============ D29 复选 ============ -->
  <div v-else-if="kind === 'checkbox'" class="center">
    <button class="cb" :class="{ on: cb }" @click.stop="cb = !cb" role="checkbox" :aria-checked="cb">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor"
           stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
        <path d="M5 12.6l4.4 4.2L19 6.8" :style="{ strokeDasharray: 26, strokeDashoffset: cb ? 0 : 26, transition: 'stroke-dashoffset 340ms cubic-bezier(.16,1,.3,1)' }"/>
      </svg>
      <span>记住这台设备</span>
    </button>
  </div>

  <!-- ============ D30 聚焦 / D31 抖动 ============ -->
  <div v-else-if="kind === 'focus'" class="center col">
    <input class="input demo-in" placeholder="点我聚焦" @focus.stop />
    <span class="hint">聚焦光晕 620ms 扩散</span>
  </div>
  <div v-else-if="kind === 'shake'" class="center col">
    <input class="input demo-in err" :key="shakeKey" :class="{ 'd-shake': shakeKey > 0 }"
           value="sk-wrong-key" readonly />
    <button class="btn sm" @click.stop="doShake">触发错误</button>
  </div>

  <!-- ============ D32 Toast ============ -->
  <div v-else-if="kind === 'toast'" class="center">
    <button class="btn sm" @click.stop="showToast">显示提示</button>
    <Transition name="tst">
      <div v-if="toast" class="toast demo-toast glass glass-3">
        <span class="tk">✓</span> 密钥已复制到剪贴板
      </div>
    </Transition>
  </div>

  <!-- ============ D33 / E40 数字 ============ -->
  <div v-else-if="kind === 'odo'" class="center col">
    <div class="odo-num">
      <span class="odo" :key="num"><span>{{ num.toLocaleString() }}</span></span>
    </div>
    <button class="btn sm" @click.stop="bump">增加数字</button>
  </div>
  <div v-else-if="kind === 'pulse'" class="center col">
    <div class="pulse-num" :key="num"><b>{{ num.toLocaleString() }}</b><i>req</i></div>
    <button class="btn sm" @click.stop="bump">变化一次</button>
  </div>

  <!-- ============ D34 液体进度 ============ -->
  <div v-else-if="kind === 'liquid'" class="center col wide">
    <div class="liq">
      <span class="liq-fill" :style="{ width: pct + '%' }">
        <i class="liq-head" />
      </span>
    </div>
    <div class="between" style="width:100%">
      <span class="tiny dim mono">{{ pct }}%</span>
      <button class="btn sm" @click.stop="fill">推进</button>
    </div>
  </div>

  <!-- ============ 兜底 ============ -->
  <div v-else class="center"><span class="hint">—</span></div>
</template>

<style scoped>
.center{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:9px;width:100%;height:100%}
.center.col{gap:8px}
.wide{width:100%;padding:0 14px}
.hint{font-size:10px;color:var(--tx-4);letter-spacing:.02em}
.demo-el{display:grid;place-items:center;width:100%;height:100%}

/* 迷你卡片 */
.mini{
  width:118px;padding:9px 10px;border-radius:12px;display:flex;flex-direction:column;gap:6px;
  background:linear-gradient(160deg,rgba(255,255,255,.09),rgba(255,255,255,.035));
  border:1px solid rgba(255,255,255,.11);
}
.mini b{font-size:10.5px;font-weight:620;letter-spacing:-.01em}
.dotrow{display:flex;gap:3px}
.dotrow i{width:4px;height:4px;border-radius:50%;background:rgba(255,255,255,.28)}
.bar{height:3px;border-radius:2px;background:rgba(255,255,255,.13);display:block}
.bar.w1{width:74%}.bar.w2{width:48%}
.bigtext{font-size:15px;font-weight:680;letter-spacing:-.02em}

/* M08 */
.skel-wrap{width:100%;height:100%;display:grid;place-items:center;position:relative}
.skel-card{width:118px;padding:10px;border-radius:12px;display:flex;flex-direction:column;gap:7px;
  border:1px solid rgba(255,255,255,.07);background:rgba(255,255,255,.02)}
.skel.s1{height:9px;width:52%}
.skel.s2{height:6px;width:100%}
.skel.s3{height:6px;width:72%}
.real{border-color:rgba(255,255,255,.12)}
.sk-enter-active{animation:k-scale-in 420ms var(--e-out) both}
.sk-leave-active{animation:k-fade-in 200ms var(--e-in-out) reverse both;position:absolute}

/* 按钮 */
.demo-btn{height:38px;padding:0 20px;font-size:13px}
.demo-btn.held{transform:scale(.94)}

/* 点赞 */
.like-btn{
  position:relative;width:46px;height:46px;border-radius:50%;
  display:grid;place-items:center;color:var(--tx-3);
  background:rgba(255,255,255,.055);border:1px solid var(--line);
  transition:transform 300ms var(--e-out),background-color 240ms,border-color 240ms,color 240ms;
}
.like-btn:hover{background:rgba(255,255,255,.08);color:var(--tx-2)}
.like-btn.on{
  color:var(--ac);border-color:var(--ac-line);background:var(--ac-dim);
  animation:like-beat 520ms cubic-bezier(.34,1.5,.64,1);
}
@keyframes like-beat{0%{transform:scale(1)}30%{transform:scale(1.22)}60%{transform:scale(.96)}100%{transform:scale(1)}}
.like-btn svg{transition:fill 220ms,stroke 220ms}
.pop{position:absolute;inset:0;pointer-events:none}
.pt{
  position:absolute;left:50%;top:50%;width:4px;height:4px;border-radius:50%;
  background:var(--ac);box-shadow:0 0 6px 1px rgba(255,138,61,.5);
  animation:k-burst 620ms cubic-bezier(.16,1,.3,1) both;
}

/* 复制 */
.copy-btn{
  display:flex;align-items:center;gap:8px;height:36px;padding:0 13px;
  border-radius:10px;border:1px solid var(--line-2);
  background:rgba(255,255,255,.055);font-size:12px;font-weight:600;color:var(--tx-2);
  transition:transform 300ms var(--e-out),border-color 240ms,color 240ms,background-color 240ms;
}
.copy-btn:active{transform:scale(.96)}
.copy-btn.done{color:#b3ecc9;border-color:rgba(84,201,138,.34);background:rgba(84,201,138,.10)}
.ic-enter-active,.ic-leave-active{transition:opacity 140ms var(--e-out),transform 200ms var(--e-out)}
.ic-enter-from{opacity:0;transform:scale(.6)}
.ic-leave-to{opacity:0;transform:scale(.6)}

/* 开关 */
.sw{padding:0;background:none;border:0}
.track{
  display:block;width:48px;height:28px;border-radius:99px;position:relative;
  background:rgba(255,255,255,.10);border:1px solid var(--line-2);
  transition:background-color 300ms var(--e-out),border-color 300ms;
}
.sw.on .track{background:linear-gradient(180deg,rgba(255,138,61,.44),rgba(255,138,61,.28));
  border-color:var(--ac-line)}
.knob{
  position:absolute;top:2.5px;left:2.5px;width:21px;height:21px;border-radius:50%;
  background:#fff;box-shadow:0 1px 4px rgba(0,0,0,.42);
  transition:transform 380ms cubic-bezier(.34,1.36,.64,1),width 220ms var(--e-out);
}
.sw.on .knob{transform:translateX(20px)}
.sw:active .knob{width:26px}
.sw.on:active .knob{transform:translateX(15px)}

/* 复选 */
.cb{
  display:flex;align-items:center;gap:9px;padding:7px 11px 7px 8px;
  border-radius:10px;border:1px solid var(--line);background:rgba(255,255,255,.03);
  font-size:11.5px;color:var(--tx-3);
  transition:transform 260ms var(--e-out),border-color 240ms,color 240ms,background-color 240ms;
}
.cb > svg{
  width:19px;height:19px;padding:2.5px;border-radius:6px;color:transparent;
  border:1.5px solid var(--line-3);background:rgba(0,0,0,.2);
  transition:background-color 240ms,border-color 240ms,color 200ms,transform 340ms cubic-bezier(.34,1.4,.64,1);
}
.cb.on{color:var(--tx-1);border-color:var(--line-2);background:rgba(255,255,255,.055)}
.cb.on > svg{
  color:#fff;border-color:var(--ac);background:var(--ac);transform:scale(1.04);
}
.cb:active{transform:scale(.97)}

/* 输入 */
.demo-in{height:36px;font-size:12px;width:170px;padding:0 11px}
.demo-in.err{border-color:rgba(240,103,95,.55);color:#f6b1ac}

/* Toast */
.demo-toast{
  position:absolute;left:50%;bottom:10px;transform:translateX(-50%);
  display:flex;align-items:center;gap:7px;white-space:nowrap;
  padding:7px 12px;border-radius:10px;font-size:11px;color:var(--tx-1);
}
.tk{color:var(--ok);font-weight:700}
.tst-enter-active{animation:d-toast 420ms var(--e-out) both}
.tst-leave-active{animation:d-toast-out 240ms var(--e-in-out) both}
.tst-enter-active.demo-toast{transform:translateX(-50%)}

/* 数字 */
.odo-num{font-size:26px;font-weight:720;letter-spacing:-.03em;font-variant-numeric:tabular-nums;
  color:var(--tx-1)}
.odo-num .odo{height:1.05em}
.pulse-num{display:flex;align-items:baseline;gap:5px}
.pulse-num b{font-size:24px;font-weight:720;letter-spacing:-.03em;font-variant-numeric:tabular-nums;
  animation:num-pulse 560ms var(--e-out)}
.pulse-num i{font-style:normal;font-size:10px;color:var(--tx-4)}
@keyframes num-pulse{
  0%{transform:scale(1);color:var(--tx-1)}
  35%{transform:scale(1.14);color:var(--ac-2)}
  100%{transform:scale(1);color:var(--tx-1)}
}

/* 液体进度 */
.liq{
  width:100%;height:9px;border-radius:99px;overflow:hidden;position:relative;
  background:rgba(255,255,255,.07);border:1px solid rgba(255,255,255,.06);
}
.liq-fill{
  display:block;height:100%;border-radius:99px;position:relative;
  background:linear-gradient(90deg,rgba(255,138,61,.62),rgba(255,138,61,.92));
  transition:width 620ms cubic-bezier(.16,1,.3,1);
  box-shadow:0 0 12px -2px rgba(255,138,61,.5);
}
.liq-head{
  position:absolute;right:0;top:-2px;bottom:-2px;width:10px;border-radius:50%;
  background:radial-gradient(circle at 50% 50%,rgba(255,240,230,.95),rgba(255,138,61,.2) 60%,transparent);
  opacity:.9;
}
</style>

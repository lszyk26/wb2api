<script setup>
import { ref, nextTick, computed } from 'vue'

const props = defineProps({ kind: String, body: String, cls: String, no: Number, tick: Number })
const k = computed(() => props.no + '-' + props.tick)

/* ---------------- M09 逐项错峰 ---------------- */
const staggerKey = ref(0)

/* ---------------- M10 FLIP 重排 ---------------- */
const rows = ref([
  { id: 'a', n: 'aio · 主通道', v: '24ms' },
  { id: 'b', n: 'glm-5.2', v: '61ms' },
  { id: 'c', n: 'kimi-k3', v: '88ms' },
  { id: 'd', n: 'deepseek-v4', v: '112ms' },
])
const listEl = ref(null)

function shuffle() {
  const els = [...(listEl.value?.children || [])]
  const first = new Map(els.map(el => [el.dataset.id, el.getBoundingClientRect().top]))

  // Fisher-Yates
  const arr = rows.value.slice()
  for (let i = arr.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[arr[i], arr[j]] = [arr[j], arr[i]]
  }
  rows.value = arr

  nextTick(() => {
    if (!listEl.value) return
    const els2 = [...listEl.value.children]
    for (const el of els2) {
      const f = first.get(el.dataset.id)
      if (f == null) continue
      const l = el.getBoundingClientRect().top
      const dy = f - l
      if (Math.abs(dy) > 0.5) {
        el.animate(
          [{ transform: `translateY(${dy}px)` }, { transform: 'translateY(0)' }],
          { duration: 460, easing: 'cubic-bezier(.16,1,.3,1)' }
        )
      }
    }
  })
}

/* ---------------- M11 插入 / M12 删除 ---------------- */
const ins = ref([{ id: 1, n: '账号 · 中国区', s: '可用' }, { id: 2, n: '账号 · 国际区', s: '可用' }])
let seq = 3
const flash = ref(0)
function insertRow() {
  ins.value = [{ id: seq, n: '新账号 · 待授权', s: '新增' }, ...ins.value].slice(0, 4)
  seq++
  flash.value++
}
const delEl = ref(null)
function removeRow(id) {
  // 先做塌陷，再移除
  const el = delEl.value?.querySelector(`[data-id="${id}"]`)
  if (el) {
    el.animate(
      [{ opacity: 1, transform: 'scaleY(1)', filter: 'blur(0)' },
       { opacity: 0, transform: 'scaleY(.55)', filter: 'blur(4px)' }],
      { duration: 280, easing: 'cubic-bezier(.65,.05,.36,1)', fill: 'forwards' }
    )
  }
  setTimeout(() => {
    ins.value = ins.value.filter(r => r.id !== id)
    nextTick(() => {
      const els = [...(delEl.value?.children || [])]
      els.forEach((el, i) => {
        el.animate(
          [{ transform: 'translateY(-8px)', opacity: .6 }, { transform: 'none', opacity: 1 }],
          { duration: 340, delay: i * 24, easing: 'cubic-bezier(.16,1,.3,1)' }
        )
      })
    })
  }, 260)
}

/* ---------------- M13 磁吸 ---------------- */
function magnetMove(e) {
  const el = e.currentTarget
  const r = el.getBoundingClientRect()
  const mx = (e.clientX - (r.left + r.width / 2)) / r.width
  const my = (e.clientY - (r.top + r.height / 2)) / r.height
  el.style.setProperty('--mx', (mx * 9).toFixed(2) + 'px')
  el.style.setProperty('--my', (my * 9).toFixed(2) + 'px')
}
function magnetOut(e) {
  e.currentTarget.style.setProperty('--mx', '0px')
  e.currentTarget.style.setProperty('--my', '0px')
}
/* 触屏没有 hover：点一下也给一次位移，保证 demo 一定能看到 */
function magnetTap(e) {
  const el = e.currentTarget
  const a = Math.random() * Math.PI * 2
  el.style.setProperty('--mx', (Math.cos(a) * 7).toFixed(1) + 'px')
  el.style.setProperty('--my', (Math.sin(a) * 7).toFixed(1) + 'px')
  setTimeout(() => magnetOut(e), 620)
}

/* ---------------- M15 手风琴 ---------------- */
const acc = ref(0)
const peekOn = ref(false)
function toggle(i) { acc.value = acc.value === i ? -1 : i }

/* ---------------- M16 瀑布流 ---------------- */
const mItems = ref([])
const mLoading = ref(false)
let mSeq = 0
const heights = [34, 52, 40, 46, 30, 56, 38, 44]
function loadMore() {
  mLoading.value = true
  setTimeout(() => {
    const add = []
    for (let i = 0; i < 4; i++) {
      add.push({ id: ++mSeq, h: heights[(mSeq + i) % heights.length] })
    }
    mItems.value = [...mItems.value, ...add].slice(-8)
    mLoading.value = false
  }, 420)
}
loadMore()
</script>

<template>
  <!-- ============ M09 逐项错峰 ============ -->
  <div v-if="kind === 'stagger'" class="lst" :key="staggerKey">
    <div v-for="i in 4" :key="i" class="li" :style="{ '--i': i - 1 }">
      <span class="av">{{ i }}</span>
      <span class="txt">列表项 {{ i }}</span>
      <span class="tag">46ms</span>
    </div>
  </div>

  <!-- ============ M10 FLIP ============ -->
  <div v-else-if="kind === 'flip'" class="lst-wrap" @click.stop="shuffle">
    <div class="lst flip" ref="listEl">
      <div v-for="r in rows" :key="r.id" class="li" :data-id="r.id">
        <span class="av m">{{ r.n.slice(0, 1).toUpperCase() }}</span>
        <span class="txt mono">{{ r.n }}</span>
        <span class="tag">{{ r.v }}</span>
      </div>
    </div>
    <span class="hint">点一下洗牌 · FLIP 补间</span>
  </div>

  <!-- ============ M11 插入 ============ -->
  <div v-else-if="kind === 'insert'" class="lst-wrap">
    <div class="lst" :key="flash">
      <div v-for="(r, i) in ins" :key="r.id" class="li" :style="{ '--i': i }">
        <span class="av m">{{ r.n.slice(0, 1) }}</span>
        <span class="txt">{{ r.n }}</span>
        <span class="tag">{{ r.s }}</span>
      </div>
    </div>
    <button class="mini-btn" @click.stop="insertRow">＋ 插入一行</button>
  </div>

  <!-- ============ M12 删除 ============ -->
  <div v-else-if="kind === 'collapse'" class="lst-wrap">
    <div class="lst" ref="delEl">
      <div v-for="r in ins" :key="r.id" class="li" :data-id="r.id">
        <span class="av m">{{ r.n.slice(0, 1) }}</span>
        <span class="txt trunc">{{ r.n }}</span>
        <button class="x" @click.stop="removeRow(r.id)" aria-label="删除">✕</button>
      </div>
    </div>
    <span class="hint">点右侧 ✕ 塌陷移除</span>
  </div>

  <!-- ============ M13 磁吸 ============ -->
  <div v-else-if="kind === 'magnet'" class="tiles">
    <div v-for="i in 3" :key="i" class="tile magnet"
         @pointermove="magnetMove" @pointerleave="magnetOut" @click.stop="magnetTap">
      <span class="ti">{{ i }}</span>
    </div>
  </div>

  <!-- ============ M14 悬停预览 ============ -->
  <div v-else-if="kind === 'peek'" class="peek-wrap">
    <div class="peek" :class="{ on: peekOn }" tabindex="0" @click.stop="peekOn = !peekOn">
      <div class="li peek-row">
        <span class="av m">W</span>
        <span class="txt">workbuddy · intl</span>
        <span class="tag on">可用</span>
      </div>
      <div class="peek-card glass glass-3">
        <div class="pk-row"><span>积分</span><b>1,240</b></div>
        <div class="pk-row"><span>延迟</span><b>61ms</b></div>
        <div class="pk-row"><span>成功 / 错误</span><b>318 / 2</b></div>
      </div>
    </div>
    <span class="hint">悬停或聚焦查看详情</span>
  </div>

  <!-- ============ M15 手风琴 ============ -->
  <div v-else-if="kind === 'accordion'" class="acc">
    <div v-for="(t, i) in ['请求参数', '响应示例', '错误码']" :key="i" class="acc-it"
         :class="{ open: acc === i }">
      <button class="acc-h" @click.stop="toggle(i)">
        <span>{{ t }}</span>
        <i class="chev">⌄</i>
      </button>
      <div class="acc-b">
        <div class="acc-in">
          <p>{{ i === 0 ? 'model / messages / stream / temperature' : i === 1 ? 'data: {"id":"chatcmpl-…"}' : '401 无效密钥 · 429 限流 · 503 池内无可用账号' }}</p>
        </div>
      </div>
    </div>
  </div>

  <!-- ============ M16 瀑布流 ============ -->
  <div v-else-if="kind === 'masonry'" class="mason">
    <div class="mason-in">
      <div v-for="(it, i) in mItems" :key="it.id" class="mason-it"
           :style="{ height: it.h + 'px', '--i': i }">
        <span v-if="mLoading && i >= mItems.length - 4" class="skel" style="width:100%;height:100%" />
      </div>
    </div>
    <button class="mini-btn" @click.stop="loadMore">加载更多</button>
  </div>

  <div v-else class="center"><span class="hint">—</span></div>
</template>

<style scoped>
.hint{font-size:10px;color:var(--tx-4)}
.center{display:grid;place-items:center;height:100%}

.lst{display:flex;flex-direction:column;gap:5px;width:100%;padding:0 11px}
.lst-wrap{width:100%;height:100%;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:7px}
.li{
  display:flex;align-items:center;gap:8px;height:27px;padding:0 8px;
  border-radius:8px;background:rgba(255,255,255,.045);border:1px solid rgba(255,255,255,.06);
  animation:k-fade-up 460ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 46ms);
}
.lst.flip .li{animation:none}
.av{
  width:17px;height:17px;flex:none;border-radius:5px;display:grid;place-items:center;
  font-size:9px;font-weight:700;color:var(--tx-2);background:rgba(255,255,255,.08);
}
.av.m{color:#ffcba6;background:var(--ac-dim);border:1px solid rgba(255,138,61,.22)}
.txt{flex:1;min-width:0;font-size:10.5px;color:var(--tx-2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.tag{font-size:9px;color:var(--tx-4);font-variant-numeric:tabular-nums}
.tag.on{color:#a9e8c5}
.x{width:18px;height:18px;border-radius:5px;display:grid;place-items:center;
  font-size:9px;color:var(--tx-4);transition:background-color 160ms,color 160ms}
.x:hover{background:rgba(240,103,95,.16);color:#f6b1ac}

.mini-btn{
  height:24px;padding:0 9px;border-radius:7px;font-size:9.5px;font-weight:600;
  border:1px solid var(--line-2);background:rgba(255,255,255,.05);color:var(--tx-3);
  transition:transform 220ms var(--e-out),background-color 160ms,color 160ms;
}
.mini-btn:active{transform:scale(.95)}
.mini-btn:hover{color:var(--tx-1);background:rgba(255,255,255,.09)}

/* 磁吸 */
.tiles{display:flex;gap:9px;align-items:center;justify-content:center;height:100%}
.tile{
  width:52px;height:52px;border-radius:14px;display:grid;place-items:center;
  background:linear-gradient(160deg,rgba(255,255,255,.09),rgba(255,255,255,.03));
  border:1px solid rgba(255,255,255,.10);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.12);
}
.ti{font-size:13px;font-weight:700;color:var(--tx-3)}

/* 预览 */
.peek-wrap{width:100%;height:100%;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:7px}
.peek{position:relative;width:calc(100% - 22px)}
.peek-row{height:32px;animation:none}
.peek.on .peek-card{opacity:1;transform:none;pointer-events:auto}
.peek-card{width:186px;padding:10px 11px;display:flex;flex-direction:column;gap:6px;border-radius:12px}
.pk-row{display:flex;justify-content:space-between;font-size:10px;color:var(--tx-3)}
.pk-row b{color:var(--tx-1);font-weight:620;font-variant-numeric:tabular-nums}

/* 手风琴 */
.acc{width:calc(100% - 22px);display:flex;flex-direction:column;gap:4px}
.acc-it{border-radius:9px;background:rgba(255,255,255,.04);border:1px solid rgba(255,255,255,.06);overflow:hidden}
.acc-h{
  display:flex;align-items:center;justify-content:space-between;width:100%;
  height:25px;padding:0 9px;font-size:10.5px;color:var(--tx-2);
}
.chev{font-style:normal;font-size:11px;color:var(--tx-4);transition:transform 320ms var(--e-out);
  display:inline-block;line-height:1}
.acc-it.open .chev{transform:rotate(180deg)}
.acc-b{display:grid;grid-template-rows:0fr;transition:grid-template-rows 380ms var(--e-out),opacity 260ms;
  opacity:0}
.acc-it.open .acc-b{grid-template-rows:1fr;opacity:1}
.acc-in{overflow:hidden}
.acc-in p{font-size:9.5px;line-height:1.55;color:var(--tx-4);padding:0 9px 8px;
  font-family:var(--mono);word-break:break-all}

/* 瀑布流 */
.mason{width:100%;height:100%;display:flex;flex-direction:column;gap:6px;padding:9px 11px}
.mason-in{flex:1;display:grid;grid-template-columns:repeat(4,1fr);gap:5px;align-content:start;overflow:hidden}
.mason-it{
  border-radius:7px;background:linear-gradient(170deg,rgba(255,255,255,.075),rgba(255,255,255,.025));
  border:1px solid rgba(255,255,255,.06);
  animation:k-fade-up 480ms var(--e-out) both;
  animation-delay:calc(var(--i,0) * 40ms);
  overflow:hidden;
}
</style>

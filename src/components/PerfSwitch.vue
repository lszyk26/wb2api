<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { LEVELS, getPerf, setPerf } from '../lib/perf.js'

const open = ref(false)
const cur = ref('full')
const box = ref(null)

onMounted(() => {
  cur.value = getPerf()
  document.addEventListener('click', onDoc, true)
})
onBeforeUnmount(() => document.removeEventListener('click', onDoc, true))

function onDoc(e) {
  if (!open.value) return
  if (box.value && !box.value.contains(e.target)) open.value = false
}
function pick(id) {
  cur.value = setPerf(id)
  open.value = false
}
const label = (id) => (LEVELS.find(l => l.id === id) || LEVELS[0]).name
</script>

<template>
  <div ref="box" class="perf-sw">
    <div v-if="open" class="perf-menu">
      <p class="perf-tt">流畅度</p>
      <button v-for="l in LEVELS" :key="l.id" class="perf-item"
              :class="{ on: cur === l.id }" @click="pick(l.id)">
        <b>{{ l.name }}</b>
        <span>{{ l.hint }}</span>
      </button>
      <p class="perf-tip">手机卡就选「精简」或「流畅」，会记住这台设备的选择</p>
    </div>
    <button class="perf-btn" :title="'流畅度：' + label(cur)" @click="open = !open">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor"
           stroke-width="2" stroke-linecap="round">
        <path d="M4 14a8 8 0 0 1 16 0" />
        <path d="M12 14l4-3" />
      </svg>
      <span>{{ label(cur) }}</span>
    </button>
  </div>
</template>

<style scoped>
.perf-sw{ position:fixed; right:12px; bottom:14px; z-index:9998; }
@media (min-width:900px){ .perf-sw{ right:20px; bottom:20px } }
.perf-btn{
  display:inline-flex; align-items:center; gap:6px;
  padding:7px 11px; border-radius:999px; cursor:pointer;
  font:500 12px/1 var(--sf, system-ui, sans-serif); color:var(--tx-2, #c9c9cf);
  background:rgba(20,20,22,.82); border:1px solid rgba(255,255,255,.12);
  backdrop-filter:blur(10px); -webkit-backdrop-filter:blur(10px);
  box-shadow:0 6px 18px rgba(0,0,0,.32);
}
.perf-btn:hover{ color:#fff; border-color:rgba(255,138,61,.5) }
.perf-menu{
  position:absolute; right:0; bottom:44px; width:206px; padding:10px;
  border-radius:16px; background:rgba(18,18,20,.96);
  border:1px solid rgba(255,255,255,.12); box-shadow:0 18px 40px rgba(0,0,0,.5);
}
.perf-tt{ margin:2px 4px 8px; font:600 11px/1 var(--sf, system-ui); letter-spacing:.08em; color:#8a8a93 }
.perf-item{
  display:block; width:100%; text-align:left; padding:8px 10px; margin-bottom:4px;
  border-radius:10px; border:1px solid transparent; background:transparent; cursor:pointer;
  color:var(--tx-1,#eee); font:400 13px/1.35 var(--sf, system-ui);
}
.perf-item b{ display:block; font-weight:600 }
.perf-item span{ display:block; font-size:11px; color:#8a8a93; margin-top:2px }
.perf-item:hover{ background:rgba(255,255,255,.06) }
.perf-item.on{ border-color:rgba(255,138,61,.55); background:rgba(255,138,61,.12) }
.perf-tip{ margin:6px 4px 0; font-size:10.5px; line-height:1.45; color:#71717a }
</style>

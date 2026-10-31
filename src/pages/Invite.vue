<script setup>
import { ref, onMounted } from 'vue'
import { api, fmt } from '../lib/api.js'

const d = ref(null)
const err = ref('')
const copied = ref('')

async function load() {
  try { d.value = await api.meInvite() } catch (e) { err.value = e.message }
}
onMounted(load)

async function copy(text, tag) {
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    }
    copied.value = tag
    setTimeout(() => { copied.value = '' }, 1800)
  } catch (e) {
    err.value = '复制失败，请长按手动复制'
  }
}
</script>

<template>
  <section class="inv container">
    <header class="ahead">
      <div>
        <p class="eyebrow">INVITE</p>
        <h1>邀请返利</h1>
        <p class="sub" style="margin-top:8px">
          把邀请码发出去：朋友注册时填上，你 +500 点；他们开通赞助，你按档位再拿 8%–15% 返利。
        </p>
      </div>
    </header>

    <p v-if="err" class="err">{{ err }}</p>

    <template v-if="d">
      <article class="glass glass-2 codebox">
        <div class="cb-l">
          <p class="eyebrow">我的邀请码</p>
          <div class="code">{{ d.code }}</div>
          <div class="cbtns">
            <button class="btn pri" @click="copy(d.code, 'code')">
              {{ copied === 'code' ? '已复制 ✓' : '复制邀请码' }}
            </button>
            <button class="btn" @click="copy(d.url, 'url')">
              {{ copied === 'url' ? '已复制 ✓' : '复制邀请链接' }}
            </button>
          </div>
          <p class="tiny dim link">{{ d.url }}</p>
        </div>
        <div class="cb-r">
          <div><b>{{ d.count }}</b><span>已邀请</span></div>
          <i />
          <div><b>{{ fmt.pts(d.earned) }}</b><span>累计返利</span></div>
          <i />
          <div><b>{{ (d.rate * 100).toFixed(0) }}%</b><span>当前返利</span></div>
        </div>
      </article>

      <article class="glass glass-2 rules">
        <h3>返利规则</h3>
        <ul>
          <li>朋友注册填你的邀请码：<b>你 +500 点</b>（临时邮箱不发放）</li>
          <li>朋友首次开通赞助：按你当前档位返点 —— C1/LV1 <b>8%</b>、C2/LV2 <b>12%</b>、C3/LV3/C4 <b>15%</b></li>
          <li>返点以「加油包」形式到账，30 天有效，不占每日额度</li>
          <li v-if="d.rate_next > d.rate">你现在 {{ (d.rate * 100).toFixed(0) }}%，升一档能到 {{ (d.rate_next * 100).toFixed(0) }}%</li>
        </ul>
      </article>

      <article class="glass glass-2 list">
        <h3>我邀请的人 <i class="tiny dim">{{ d.count }}</i></h3>
        <p v-if="!d.invitees.length" class="tiny dim empty">还没有人用你的邀请码注册，去群里发一下吧。</p>
        <ul v-else>
          <li v-for="p in d.invitees" :key="p.username">
            <b>{{ p.username }}</b>
            <span class="tiny dim">{{ p.created_at ? String(p.created_at).slice(0, 10) : '' }}</span>
            <span class="pill" :class="p.paid ? 'ok' : ''">{{ p.paid ? '已赞助' : '未付费' }}</span>
          </li>
        </ul>
      </article>

      <article v-if="d.rewards && d.rewards.length" class="glass glass-2 list">
        <h3>返利流水</h3>
        <ul>
          <li v-for="r in d.rewards" :key="r.id">
            <b>{{ r.invitee }}</b>
            <span class="tiny dim">{{ r.plan }} · ¥{{ r.amount }}</span>
            <span class="amt">+{{ r.points }} 点</span>
          </li>
        </ul>
      </article>
    </template>
  </section>
</template>

<style scoped>
.codebox{display:flex;gap:24px;flex-wrap:wrap;align-items:center;padding:20px;margin-bottom:14px}
.cb-l{flex:1 1 300px;min-width:0}
.code{font:700 26px/1.2 ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.06em;color:#ffd9b8;margin:6px 0 12px;word-break:break-all}
.cbtns{display:flex;gap:8px;flex-wrap:wrap}
.link{margin:10px 0 0;word-break:break-all}
.cb-r{display:flex;gap:18px;align-items:center;flex-wrap:wrap}
.cb-r div{text-align:center}
.cb-r b{display:block;font:700 20px/1.2 var(--sf);color:var(--tx-1)}
.cb-r span{font-size:11px;color:var(--tx-3)}
.cb-r i{width:1px;height:28px;background:var(--line)}
.rules,.list{padding:18px 20px;margin-bottom:14px}
.rules h3,.list h3{margin:0 0 10px;font-size:14px}
.rules ul{margin:0;padding-left:18px;color:var(--tx-2);font-size:12.5px;line-height:1.9}
.rules b{color:var(--ac-1)}
.list ul{list-style:none;margin:0;padding:0}
.list li{display:flex;align-items:center;gap:10px;padding:9px 0;border-bottom:1px solid var(--line);font-size:13px}
.list li:last-child{border-bottom:0}
.list li b{min-width:96px}
.amt{margin-left:auto;color:#7fe3a1;font-weight:600}
.empty{margin:0}
.err{color:#ff9a9a;font-size:12.5px}
</style>

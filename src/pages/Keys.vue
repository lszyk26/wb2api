<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, fmt } from '../lib/api.js'

const emit = defineEmits(['need-login'])

const keys = ref([])
const maxKeys = ref(1)
const loading = ref(true)
const err = ref('')
const creating = ref(false)
const newName = ref('')

/* 刚创建的明文（只显示一次，也允许再次查看因为我们加密存了） */
const revealed = ref({})

const name = ref('')

async function load() {
  loading.value = true
  try {
    const d = await api.keys()
    keys.value = d.keys || []
    maxKeys.value = d.max || 1
  } catch (e) {
    if (e.status === 401) emit('need-login')
    else err.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)

const active = computed(() => keys.value.filter(k => !k.revoked))
const canCreate = computed(() => active.value.length < maxKeys.value)

async function create() {
  if (creating.value) return
  creating.value = true
  err.value = ''
  try {
    const d = await api.createKey(newName.value || '默认密钥')
    revealed.value[d.key.id] = d.secret
    newName.value = ''
    await load()
  } catch (e) {
    err.value = e.message
  } finally {
    creating.value = false
  }
}

const copied = ref(0)
async function copySecret(id, secret) {
  try {
    await navigator.clipboard.writeText(secret)
  } catch (_) {
    const ta = document.createElement('textarea')
    ta.value = secret
    document.body.appendChild(ta); ta.select(); document.execCommand('copy'); ta.remove()
  }
  copied.value = id
  setTimeout(() => { if (copied.value === id) copied.value = 0 }, 1600)
}

async function revoke(k) {
  if (!confirm(`吊销「${k.name}」？已在使用它的脚本会立刻失效。`)) return
  try {
    await api.deleteKey(k.id)
    delete revealed.value[k.id]
    await load()
  } catch (e) { err.value = e.message }
}

async function rotate(k) {
  if (!confirm(`轮换「${k.name}」？旧密钥立刻失效，新密钥只显示这一次。`)) return
  try {
    const d = await api.rotateKey(k.id)
    revealed.value[k.id] = d.secret
    await load()
  } catch (e) { err.value = e.message }
}

const showAll = ref(false)
const baseUrl = computed(() => (location.origin || 'http://localhost:8078') + '/v1')
const sample = computed(() => {
  const sk = revealed.value[active.value[0]?.id] || (active.value[0]?.prefix ? active.value[0].prefix + '…' : 'sk-wb-…')
  return `curl ${baseUrl.value}/chat/completions \\
  -H "Authorization: Bearer ${sk}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"glm-5.3-flash","stream":true,
       "messages":[{"role":"user","content":"你好"}]}'`
})
</script>

<template>
  <section class="keys container">
    <header class="khead">
      <div>
        <p class="eyebrow">API KEYS</p>
        <h1>密钥</h1>
        <p class="sub" style="margin-top:8px">
          密钥形如 <code class="inl">sk-wb-…</code>，只在创建时完整显示。吊销立即生效。
        </p>
      </div>
      <div class="kbadge glass glass-1">
        <b>{{ active.length }}<i>/{{ maxKeys }}</i></b>
        <span>已用 / 上限</span>
      </div>
    </header>

    <!-- 创建 -->
    <article class="create glass glass-2">
      <div class="c-left">
        <label class="tiny dim">密钥名称（自己看得懂就行）</label>
        <input v-model.trim="newName" class="input" placeholder="例如：我的笔记本 / 班级项目"
               @keydown.enter="create" />
      </div>
      <button class="btn pri lg" :disabled="!canCreate || creating" @click="create">
        <span v-if="creating" class="spin" />
        <template v-else>＋ 新建密钥</template>
      </button>
    </article>
    <p v-if="!canCreate" class="warnline">
      当前档位上限 {{ maxKeys }} 把。需要更多请升级档位，或先吊销不用的。
    </p>
    <p v-if="err" class="errline">{{ err }}</p>

    <!-- 列表 -->
    <div class="list">
      <TransitionGroup name="krow" tag="div" class="rows">
        <article v-for="k in keys" :key="k.id" class="krow glass glass-1" :class="{ dead: k.revoked }">
          <div class="k-top">
            <span class="dot" :class="k.revoked ? 'off' : 'on'" />
            <b class="kname">{{ k.name }}</b>
            <span v-if="k.revoked" class="pill err">已吊销</span>
            <span v-else class="pill ok">有效</span>
          </div>

          <div class="k-secret">
            <code class="mono">{{ revealed[k.id] || (k.prefix + '••••••••••••••••') }}</code>
            <button v-if="revealed[k.id]" class="icobtn" :class="{ done: copied === k.id }"
                    @click="copySecret(k.id, revealed[k.id])" :title="copied === k.id ? '已复制' : '复制'">
              <svg v-if="copied !== k.id" viewBox="0 0 24 24" width="13" height="13" fill="none"
                   stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
                <rect x="8.4" y="8.4" width="11" height="11" rx="2.4"/>
                <path d="M15.6 5.4H6.6a1.2 1.2 0 00-1.2 1.2v9"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor"
                   stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
                <path d="M5 12.6l4.3 4.2L19 6.9"/>
              </svg>
            </button>
          </div>

          <div class="k-meta">
            <span><i>调用</i><b>{{ fmt.n(k.calls) }}</b></span>
            <span><i>创建</i><b>{{ fmt.date(k.created_at) }}</b></span>
            <span><i>最近</i><b>{{ k.last_used ? fmt.ago(k.last_used) : '未使用' }}</b></span>
          </div>

          <div v-if="!k.revoked" class="k-acts">
            <button class="btn sm ghost" @click="rotate(k)">轮换</button>
            <button class="btn sm ghost danger" @click="revoke(k)">吊销</button>
          </div>
        </article>
      </TransitionGroup>

      <p v-if="!loading && !keys.length" class="empty glass glass-1">
        还没有密钥。新建一把，然后在客户端里把它当 OpenAI 的 api_key 用。
      </p>
    </div>

    <!-- 用法 -->
    <article class="usage glass glass-1">
      <div class="between">
        <div>
          <p class="eyebrow">QUICK START</p>
          <h3>直接替换 base_url 就能用</h3>
        </div>
        <span class="pill ac">OpenAI 兼容</span>
      </div>
      <pre class="code">{{ sample }}</pre>
      <div class="tips">
        <div class="tip"><i>Base URL</i><code>{{ baseUrl }}</code></div>
        <div class="tip"><i>模型名</i><code>glm-5.3-flash / kimi-k3 / gpt-5.5 …</code></div>
        <div class="tip"><i>流式</i><code>"stream": true</code></div>
      </div>
    </article>
  </section>
</template>

<style scoped>
.keys{padding-top:26px;display:flex;flex-direction:column;gap:13px}
.khead{display:flex;align-items:flex-end;justify-content:space-between;gap:18px;flex-wrap:wrap;margin-bottom:4px}
.khead h1{margin-top:6px}
.inl{font-family:var(--mono);font-size:12px;color:var(--ac-2);background:var(--ac-dim);
  padding:1px 5px;border-radius:4px}
.kbadge{padding:10px 16px;display:flex;flex-direction:column;align-items:flex-end;gap:1px;border-radius:14px}
.kbadge b{font-size:20px;font-weight:720;letter-spacing:-.03em;font-variant-numeric:tabular-nums}
.kbadge b i{font-style:normal;font-size:11px;color:var(--tx-4);font-weight:500}
.kbadge span{font-size:10px;color:var(--tx-4)}

.create{display:flex;gap:12px;align-items:flex-end;padding:14px 16px}
.c-left{flex:1;min-width:0;display:flex;flex-direction:column;gap:6px}
.create .btn{flex:none}
@media (max-width:600px){
  .create{flex-direction:column;align-items:stretch}
  .create .btn{width:100%}
}
.warnline{font-size:11.5px;color:#f0d29a;padding-left:2px}
.errline{font-size:12px;color:#f6b1ac;background:rgba(240,103,95,.10);
  border:1px solid rgba(240,103,95,.22);border-radius:9px;padding:8px 11px}

/* 列表 */
.list{display:flex;flex-direction:column;gap:9px}
.rows{display:flex;flex-direction:column;gap:9px}
.krow{padding:13px 15px;display:flex;flex-direction:column;gap:9px;
  transition:border-color 260ms,opacity 260ms}
.krow.dead{opacity:.55}
.k-top{display:flex;align-items:center;gap:8px}
.dot{width:6px;height:6px;border-radius:50%;flex:none}
.dot.on{background:var(--ok);box-shadow:0 0 8px 1px rgba(84,201,138,.5);
  animation:k-breathe 2.4s var(--e-in-out) infinite}
.dot.off{background:var(--tx-4)}
.kname{font-size:13px;font-weight:640;letter-spacing:-.01em;flex:1;min-width:0;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

.k-secret{display:flex;align-items:center;gap:8px;
  background:rgba(0,0,0,.28);border:1px solid var(--line);border-radius:9px;
  padding:7px 9px 7px 11px}
.k-secret code{flex:1;min-width:0;font-size:11px;color:var(--tx-2);
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap;letter-spacing:.01em}
.icobtn{width:24px;height:24px;flex:none;border-radius:7px;display:grid;place-items:center;
  color:var(--tx-4);transition:background-color 180ms,color 180ms,transform 240ms var(--e-out)}
.icobtn:hover{background:rgba(255,255,255,.08);color:var(--tx-1)}
.icobtn:active{transform:scale(.9)}
.icobtn.done{color:var(--ok);background:rgba(84,201,138,.12)}

.k-meta{display:flex;gap:20px;flex-wrap:wrap}
.k-meta span{display:flex;align-items:baseline;gap:5px;font-size:11px}
.k-meta i{font-style:normal;color:var(--tx-4);font-size:10px}
.k-meta b{font-weight:600;color:var(--tx-2);font-variant-numeric:tabular-nums}

.k-acts{display:flex;gap:6px;padding-top:2px}
.k-acts .btn{height:28px;font-size:11px}
.danger:hover{color:#f6b1ac;background:rgba(240,103,95,.10)}

.empty{padding:26px;text-align:center;font-size:12.5px;color:var(--tx-4);line-height:1.7}

.krow-enter-active{animation:k-fade-up 420ms var(--e-out) both}
.krow-leave-active{animation:k-fade-in 200ms reverse both}
.krow-move{transition:transform 340ms var(--e-out)}

/* 用法 */
.usage{padding:16px 18px;display:flex;flex-direction:column;gap:11px;margin-top:4px}
.usage h3{margin-top:3px}
.code{font-size:10.5px;line-height:1.75;color:var(--tx-2);white-space:pre-wrap;word-break:break-all;
  background:rgba(0,0,0,.3);border:1px solid var(--line);border-radius:10px;padding:12px 13px;
  max-height:230px;overflow:auto}
.tips{display:flex;flex-direction:column;gap:6px}
.tip{display:flex;align-items:center;gap:10px;font-size:11px;padding-top:6px;
  border-top:1px solid var(--line)}
.tip i{font-style:normal;color:var(--tx-4);width:64px;flex:none}
.tip code{font-size:10.5px;color:var(--ac-2);background:var(--ac-dim);padding:2px 6px;
  border-radius:5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

.spin{width:13px;height:13px;border-radius:50%;border:1.8px solid rgba(255,255,255,.3);
  border-top-color:#fff;animation:spin .7s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
</style>

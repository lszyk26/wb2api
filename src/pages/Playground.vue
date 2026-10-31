<script setup>
import { ref, nextTick, computed, onBeforeUnmount, onMounted } from 'vue'
import { api, state } from '../lib/api.js'

const models = ref([
  { id: 'glm-5.3-flash', name: 'glm-5.3-flash', desc: '快 · 便宜' },
  { id: 'kimi-k3', name: 'kimi-k3', desc: '长上下文' },
  { id: 'gpt-5.5', name: 'gpt-5.5', desc: '通用' },
])
const model = ref('glm-5.3-flash')
const modelList = ref([])

onMounted(async () => {
  try {
    const s = await api.status()
    if (s.models && s.models.length) {
      modelList.value = s.models
      // 优先挑几个常用模型
      const prefer = ['glm-5.3-flash', 'kimi-k3', 'gpt-5.5', 'deepseek-v4.1-flash', 'glm-5.3', 'gpt-5.6-sol']
      const picked = prefer.filter(m => s.models.includes(m)).slice(0, 3)
      const rest = s.models.filter(m => !picked.includes(m)).slice(0, 3 - picked.length)
      const final = [...picked, ...rest]
      if (final.length) {
        models.value = final.map(id => ({ id, name: id, desc: '' }))
        if (!final.includes(model.value)) model.value = final[0]
      }
    }
  } catch (_) {}
})
const input = ref('')
const sending = ref(false)
const msgs = ref([
  { role: 'assistant', text: '网关已就绪。发一条消息，请求会走真实账号池。', meta: 'aio · 0.4s' },
])
const scroller = ref(null)
let streamTimer = null



function scrollBottom() {
  nextTick(() => {
    const el = scroller.value
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  })
}

let aborter = null

function send() {
  const text = input.value.trim()
  if (!text || sending.value) return
  if (!state.token) { emit('need-login'); return }
  msgs.value.push({ role: 'user', text })
  input.value = ''
  sending.value = true
  scrollBottom()

  const history = msgs.value
    .filter(m => m.role === 'user' || m.role === 'assistant')
    .slice(-12)
    .map(m => ({ role: m.role, content: m.text }))

  const reply = { role: 'assistant', text: '', meta: model.value + ' · 连接中', streaming: true }
  msgs.value.push(reply)
  scrollBottom()

  const t0 = performance.now()
  aborter = new AbortController()
  let ttft = 0

  api.playgroundChat({
    model: model.value,
    messages: history,
    signal: aborter.signal,
    onDelta: (piece) => {
      if (!ttft) ttft = Math.round(performance.now() - t0)
      reply.text += piece
      reply.meta = model.value + ' · 生成中'
      scrollBottom()
    },
    onDone: (full, usage) => {
      reply.streaming = false
      const ms = Math.round(performance.now() - t0)
      const tok = usage?.total_tokens ? ` · ${usage.total_tokens} tokens` : ''
      reply.meta = `${model.value} · ${(ms / 1000).toFixed(1)}s · 首字 ${ttft || ms}ms${tok}`
      sending.value = false
      scrollBottom()
    },
    onError: (e) => {
      reply.streaming = false
      reply.text = reply.text || ''
      reply.meta = '请求失败'
      reply.error = e.message
      sending.value = false
      scrollBottom()
    },
  }).catch(e => {
    reply.streaming = false
    reply.error = e.message
    reply.meta = '请求失败'
    sending.value = false
  })
}

function stopGen() {
  if (aborter) { aborter.abort(); aborter = null }
  sending.value = false
}

function onKey(e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
}

const canSend = computed(() => input.value.trim() && !sending.value)
const emit = defineEmits(['need-login'])
onBeforeUnmount(() => clearInterval(streamTimer))

const origin = (window.location && window.location.origin) || 'http://localhost:8078'
const codeExample = computed(() =>
`curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer sk-wb-••••" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${model.value}","stream":true,
       "messages":[{"role":"user","content":"hi"}]}'`
)
</script>

<template>
  <section class="pg container">
    <header class="phead2">
      <div>
        <p class="eyebrow">PLAYGROUND</p>
        <h1>让模型回答一个问题</h1>
        <p class="sub" style="margin-top:8px">真实请求经过同一账号池，对话只留在当前页面。</p>
      </div>
      <div class="mods">
        <button v-for="m in models" :key="m.id" class="mod" :data-on="model === m.id ? 1 : 0"
                @click="model = m.id">
          <b>{{ m.name }}</b>
          <span>{{ m.desc }}</span>
        </button>
      </div>
    </header>

    <div class="pg-grid">
      <!-- 对话 -->
      <article class="chat glass glass-2">
        <div class="chat-top">
          <span class="pill ok"><i class="live" />池内可用</span>
          <span class="tiny dim mono">{{ model }}</span>
          <button class="btn sm ghost" style="margin-left:auto"
                  @click="msgs = [{ role: 'assistant', text: '对话已清空。', meta: model + ' · 0.1s' }]">
            清空
          </button>
        </div>

        <div class="msgs no-sb" ref="scroller">
          <div v-for="(m, i) in msgs" :key="i" class="msg" :class="m.role" :style="{ '--i': i }">
            <span v-if="m.role === 'assistant'" class="mav">W</span>
            <div class="bubble" :class="{ streaming: m.streaming }">
              <p>{{ m.text }}<i v-if="m.streaming" class="caret" /></p>
              <span v-if="m.error" class="merr tiny">{{ m.error }}</span>
              <span class="mmeta tiny">{{ m.meta }}</span>
            </div>
          </div>
        </div>

        <div class="composer">
          <textarea v-model="input" rows="1" placeholder="输入消息，Enter 发送 · Shift+Enter 换行"
                    @keydown="onKey" />
          <button v-if="sending" class="send stop" @click="stopGen" aria-label="停止">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><rect x="7" y="7" width="10" height="10" rx="2"/></svg>
          </button>
          <button v-else class="send" :class="{ on: canSend }" :disabled="!canSend" @click="send"
                  aria-label="发送">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 19V5M5.5 11.5L12 5l6.5 6.5"/>
            </svg>
          </button>
        </div>
      </article>

      <!-- 接入 -->
      <aside class="side">
        <article class="glass glass-1 code-card">
          <div class="between">
            <p class="eyebrow">QUICK START</p>
            <span class="pill">OpenAI 兼容</span>
          </div>
          <pre class="code">{{ codeExample }}</pre>
          <button class="btn sm" style="align-self:flex-start">复制示例</button>
        </article>

        <article class="glass glass-1 tips">
          <p class="eyebrow">BASE URL</p>
          <div class="kv"><span>Endpoint</span><code>{{ origin }}/v1</code></div>
          <div class="kv"><span>鉴权</span><code>Bearer sk-wb-…</code></div>
          <div class="kv"><span>流式</span><code>"stream": true</code></div>
        </article>
      </aside>
    </div>
  </section>
</template>

<style scoped>
.pg{padding-top:26px}
.phead2{display:flex;align-items:flex-end;justify-content:space-between;gap:20px;flex-wrap:wrap;margin-bottom:16px}
.phead2 h1{margin-top:6px}
.mods{display:flex;gap:6px;flex-wrap:wrap}
.mod{
  display:flex;flex-direction:column;gap:1px;padding:8px 12px;border-radius:11px;
  border:1px solid var(--line);background:rgba(255,255,255,.03);text-align:left;
  transition:transform 260ms var(--e-out),background-color 200ms,border-color 200ms;
}
.mod:active{transform:scale(.97)}
.mod b{font-size:12px;font-weight:640;color:var(--tx-2)}
.mod span{font-size:9.5px;color:var(--tx-4)}
.mod[data-on="1"]{border-color:var(--ac-line);
  background:linear-gradient(180deg,rgba(255,138,61,.16),rgba(255,138,61,.07))}
.mod[data-on="1"] b{color:#ffd6b8}

.pg-grid{display:grid;grid-template-columns:1.5fr .8fr;gap:12px;align-items:start}
@media (max-width:980px){ .pg-grid{grid-template-columns:1fr} }

/* 对话 */
.chat{display:flex;flex-direction:column;height:min(560px,calc(100dvh - 190px));min-height:380px;
  padding:12px 12px 12px;overflow:hidden}
.chat-top{display:flex;align-items:center;gap:9px;padding:2px 4px 10px;border-bottom:1px solid var(--line)}
.live{width:6px;height:6px;border-radius:50%;background:var(--ok);
  box-shadow:0 0 8px 1px rgba(84,201,138,.6);animation:k-breathe 2.4s var(--e-in-out) infinite}

.msgs{flex:1;overflow-y:auto;padding:12px 4px;display:flex;flex-direction:column;gap:10px}
.msg{display:flex;gap:8px;align-items:flex-end;animation:k-fade-up 420ms var(--e-out) both}
.msg.user{flex-direction:row-reverse}
.mav{
  width:24px;height:24px;flex:none;border-radius:50%;display:grid;place-items:center;
  font-size:10px;font-weight:700;color:#ffcba6;background:var(--ac-dim);
  border:1px solid var(--ac-line);
}
.bubble{
  max-width:76%;padding:9px 12px;border-radius:14px;
  background:rgba(255,255,255,.05);border:1px solid rgba(255,255,255,.075);
  display:flex;flex-direction:column;gap:4px;
}
.msg.user .bubble{
  background:linear-gradient(160deg,rgba(255,138,61,.20),rgba(255,138,61,.10));
  border-color:rgba(255,138,61,.24);border-bottom-right-radius:5px;
}
.msg.assistant .bubble{border-bottom-left-radius:5px}
.bubble p{font-size:13px;line-height:1.65;color:var(--tx-1);white-space:pre-wrap;word-break:break-word}
.mmeta{color:var(--tx-4);font-family:var(--mono);font-size:9.5px}
.merr{color:#f6b1ac;font-size:11px;line-height:1.5}
.send.stop{background:rgba(240,103,95,.18);border-color:rgba(240,103,95,.34);color:#f6b1ac}
.caret{display:inline-block;width:5px;height:12px;margin-left:2px;border-radius:1.5px;
  background:var(--ac);vertical-align:-1px;animation:k-blink 1.05s steps(1,end) infinite}

/* 输入 */
.composer{display:flex;align-items:flex-end;gap:8px;padding:9px 9px 9px 13px;
  border-radius:16px;border:1px solid var(--line-2);background:rgba(0,0,0,.28);
  transition:border-color 220ms,box-shadow 300ms}
.composer:focus-within{border-color:var(--ac-line);box-shadow:0 0 0 3px rgba(255,138,61,.08)}
.composer textarea{
  flex:1;min-height:24px;max-height:110px;border:0;background:none;outline:none;resize:none;
  font-size:13px;line-height:1.55;color:var(--tx-1);padding:2px 0;
}
.composer textarea::placeholder{color:var(--tx-4)}
.send{
  width:33px;height:33px;flex:none;border-radius:11px;display:grid;place-items:center;
  background:rgba(255,255,255,.06);border:1px solid var(--line);color:var(--tx-4);
  transition:transform 260ms var(--e-out),background-color 240ms,color 240ms,border-color 240ms;
}
.send.on{background:linear-gradient(180deg,rgba(255,138,61,.34),rgba(255,138,61,.2));
  border-color:var(--ac-line);color:#fff;
  box-shadow:inset 0 1px 0 rgba(255,220,190,.2)}
.send:active{transform:scale(.92)}

/* 侧栏 */
.side{display:flex;flex-direction:column;gap:12px}
.code-card{padding:14px 15px;display:flex;flex-direction:column;gap:11px}
.code{font-size:10.5px;line-height:1.72;color:var(--tx-2);white-space:pre-wrap;word-break:break-all;
  background:rgba(0,0,0,.32);border:1px solid var(--line);border-radius:10px;padding:11px 12px;
  max-height:210px;overflow:auto}
.tips{padding:14px 15px;display:flex;flex-direction:column;gap:9px}
.kv{display:flex;align-items:center;justify-content:space-between;gap:10px;
  padding-top:8px;border-top:1px solid var(--line);font-size:11px;color:var(--tx-3)}
.kv code{font-size:10px;color:var(--ac-2);background:var(--ac-dim);
  padding:2px 6px;border-radius:5px;max-width:62%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
</style>

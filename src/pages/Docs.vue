<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../lib/api.js'

const origin = (window.location && window.location.origin) || 'http://localhost:8078'
const models = ref([])
const copied = ref('')

onMounted(async () => {
  try {
    const s = await api.status()
    models.value = s.models || []
  } catch (_) {}
})

const modelList = computed(() => models.value.slice(0, 24))

const tab = ref('chat')
const tabs = [
  { id: 'chat', label: '对话补全' },
  { id: 'models', label: '模型列表' },
  { id: 'errors', label: '错误码' },
  { id: 'quota', label: '额度规则' },
]

const snippets = {
  curl: computed(() => `curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer sk-wb-你的密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "glm-5.3-flash",
    "stream": true,
    "messages": [{"role": "user", "content": "你好"}]
  }'`),
  python: computed(() => `from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="sk-wb-你的密钥",
)

stream = client.chat.completions.create(
    model="glm-5.3-flash",
    messages=[{"role": "user", "content": "你好"}],
    stream=True,
)
for chunk in stream:
    print(chunk.choices[0].delta.content or "", end="")`),
  node: computed(() => `import OpenAI from "openai"

const client = new OpenAI({
  baseURL: "${origin}/v1",
  apiKey: "sk-wb-你的密钥",
})

const stream = await client.chat.completions.create({
  model: "glm-5.3-flash",
  messages: [{ role: "user", content: "你好" }],
  stream: true,
})
for await (const c of stream) {
  process.stdout.write(c.choices[0]?.delta?.content ?? "")
}`),
}

const lang = ref('curl')
const code = computed(() => snippets[lang.value].value)

async function copy(t) {
  try { await navigator.clipboard.writeText(t) } catch (_) {}
  copied.value = t
  setTimeout(() => { if (copied.value === t) copied.value = '' }, 1500)
}

const errors = [
  ['401', 'API Key 缺失 / 无效 / 已吊销', '检查 Authorization 头，密钥形如 sk-wb-…'],
  ['402', '额度用完 / 超出软上限', '第二天 0 点恢复，或兑换卡密、赞助档位'],
  ['404', '模型不存在或当前不可用', '先调 /v1/models 看可用列表'],
  ['429', '并发或每分钟请求超限', '降低并发，或升级档位'],
  ['502', '上游网关不可达', '稍后重试；持续出现请反馈'],
]
</script>

<template>
  <section class="docs container">
    <header class="dhead">
      <p class="eyebrow">API DOCS</p>
      <h1>接口文档</h1>
      <p class="sub" style="margin-top:8px">
        完全兼容 OpenAI 协议：把 <code class="inl">base_url</code> 换成
        <code class="inl">{{ origin }}/v1</code>，其余代码不用改。
      </p>
    </header>

    <div class="tabs">
      <span class="ink" :style="{ transform: `translateX(${tabs.findIndex(t => t.id === tab) * 100}%)` }" />
      <button v-for="t in tabs" :key="t.id" :data-on="tab === t.id ? 1 : 0" @click="tab = t.id">
        {{ t.label }}
      </button>
    </div>

    <!-- 对话补全 -->
    <template v-if="tab === 'chat'">
      <article class="panel glass glass-1">
        <div class="between">
          <div>
            <p class="eyebrow">POST</p>
            <h3 class="mono ep">/v1/chat/completions</h3>
          </div>
          <span class="pill ok">OpenAI 兼容</span>
        </div>

        <div class="langs">
          <button v-for="l in ['curl','python','node']" :key="l" class="lbtn"
                  :data-on="lang === l ? 1 : 0" @click="lang = l">{{ l }}</button>
        </div>

        <div class="codewrap">
          <button class="copybtn" :class="{ done: copied === code }" @click="copy(code)">
            {{ copied === code ? '已复制' : '复制' }}
          </button>
          <pre class="code">{{ code }}</pre>
        </div>
      </article>

      <article class="panel glass glass-1">
        <div><p class="eyebrow">BODY</p><h3>请求参数</h3></div>
        <div class="params">
          <div class="prow"><code>model</code><span>必填。见「模型列表」。</span></div>
          <div class="prow"><code>messages</code><span>必填。标准 OpenAI 消息数组。</span></div>
          <div class="prow"><code>stream</code><span>可选。true 时按 SSE 逐块返回，支持 <code>stream_options.include_usage</code>。</span></div>
          <div class="prow"><code>max_tokens</code><span>可选。最大输出长度。</span></div>
          <div class="prow"><code>temperature</code><span>可选。采样温度。</span></div>
          <div class="prow"><code>tools</code><span>可选。函数调用（模型支持时生效）。</span></div>
        </div>
      </article>
    </template>

    <!-- 模型 -->
    <template v-else-if="tab === 'models'">
      <article class="panel glass glass-1">
        <div class="between">
          <div><p class="eyebrow">GET</p><h3 class="mono ep">/v1/models</h3></div>
          <span class="pill">{{ models.length }} 个可用</span>
        </div>
        <div class="mgrid">
          <div v-for="m in modelList" :key="m" class="mchip" @click="copy(m)">
            <code>{{ m }}</code>
            <i v-if="copied === m">已复制</i>
          </div>
        </div>
        <p v-if="!models.length" class="tiny dim">模型列表暂时取不到（上游可能未就绪）。</p>
        <p class="tiny dim">点任意模型名可复制。</p>
      </article>
    </template>

    <!-- 错误码 -->
    <template v-else-if="tab === 'errors'">
      <article class="panel glass glass-1">
        <div><p class="eyebrow">ERRORS</p><h3>常见错误</h3></div>
        <div class="errtable">
          <div v-for="e in errors" :key="e[0]" class="erow">
            <span class="ecode">{{ e[0] }}</span>
            <b>{{ e[1] }}</b>
            <i>{{ e[2] }}</i>
          </div>
        </div>
      </article>
    </template>

    <!-- 额度 -->
    <template v-else-if="tab === 'quota'">
      <article class="panel glass glass-1">
        <div><p class="eyebrow">QUOTA</p><h3>怎么计费</h3></div>
        <div class="qlist">
          <div class="qrow"><b>1 元 = 1000 点</b><span>一次轻量对话（几百 token）约 0.4–4 点。</span></div>
          <div class="qrow"><b>按 token 计价</b><span>(输入×2 + 输出×8) / 1000 × 模型系数。输出更贵，因为生成更耗算力。</span></div>
          <div class="qrow"><b>模型系数</b><span>flash 类 1.0 · 主力 1.8 · 顶配（gpt-6 系）4.0。</span></div>
          <div class="qrow"><b>按日重置</b><span>每天 0 点恢复日额度，不做「清零惩罚」。</span></div>
          <div class="qrow"><b>软上限</b><span>日额度用完后限速到 1/3，超 150% 且无加油包才停到次日。</span></div>
          <div class="qrow"><b>加油包优先</b><span>超额部分先扣加油包，不动日额度。</span></div>
        </div>
      </article>
    </template>
  </section>
</template>

<style scoped>
.docs{padding-top:26px;display:flex;flex-direction:column;gap:13px}
.dhead h1{margin-top:6px}
.inl{font-family:var(--mono);font-size:12px;color:var(--ac-2);background:var(--ac-dim);
  padding:1px 6px;border-radius:5px}

.tabs{position:relative;display:flex;padding:3px;border-radius:12px;
  background:rgba(255,255,255,.04);border:1px solid var(--line);overflow-x:auto;scrollbar-width:none}
.tabs::-webkit-scrollbar{display:none}
.ink{position:absolute;top:3px;left:3px;height:calc(100% - 6px);
  width:calc((100% - 6px) / 4);border-radius:9px;
  background:linear-gradient(160deg,rgba(255,255,255,.15),rgba(255,255,255,.06));
  border:1px solid rgba(255,255,255,.13);transition:transform 420ms var(--e-out)}
.tabs button{position:relative;z-index:1;flex:1;min-width:80px;height:32px;border-radius:9px;
  font-size:12px;font-weight:600;color:var(--tx-4);transition:color 220ms;white-space:nowrap}
.tabs button[data-on="1"]{color:var(--tx-1)}

.panel{padding:16px 18px;display:flex;flex-direction:column;gap:12px}
.ep{font-size:13px;font-family:var(--mono);color:var(--ac-2);margin-top:4px}

.langs{display:flex;gap:6px}
.lbtn{height:26px;padding:0 11px;border-radius:7px;font-size:11px;font-weight:600;
  color:var(--tx-4);border:1px solid transparent;transition:all 200ms}
.lbtn[data-on="1"]{color:var(--tx-1);background:rgba(255,255,255,.07);border-color:var(--line)}

.codewrap{position:relative}
.copybtn{position:absolute;top:9px;right:9px;height:24px;padding:0 9px;border-radius:7px;
  font-size:10.5px;color:var(--tx-3);background:rgba(255,255,255,.07);
  border:1px solid var(--line);transition:all 180ms;z-index:2}
.copybtn:hover{color:var(--tx-1);background:rgba(255,255,255,.11)}
.copybtn.done{color:#a9e8c5;border-color:rgba(84,201,138,.3);background:rgba(84,201,138,.1)}
.code{font-size:11px;line-height:1.75;color:var(--tx-2);white-space:pre;
  background:rgba(0,0,0,.32);border:1px solid var(--line);border-radius:11px;
  padding:14px 15px;overflow:auto;max-height:380px}

.params{display:flex;flex-direction:column}
.prow{display:grid;grid-template-columns:132px 1fr;gap:12px;align-items:start;
  padding:9px 0;border-top:1px solid rgba(255,255,255,.045);font-size:11.5px}
.prow:first-child{border-top:0}
.prow code{color:var(--ac-2);font-size:11px}
.prow span{color:var(--tx-3);line-height:1.6}
.prow span code{font-size:10.5px}
@media (max-width:560px){ .prow{grid-template-columns:1fr;gap:3px} }

.mgrid{display:flex;flex-wrap:wrap;gap:6px}
.mchip{padding:5px 9px;border-radius:8px;border:1px solid var(--line);
  background:rgba(255,255,255,.03);cursor:pointer;display:flex;align-items:center;gap:6px;
  transition:all 200ms}
.mchip:hover{background:rgba(255,255,255,.07);border-color:var(--line-2)}
.mchip code{font-size:10.5px;color:var(--tx-2)}
.mchip i{font-style:normal;font-size:9px;color:#8fe8b6}

.errtable{display:flex;flex-direction:column}
.erow{display:grid;grid-template-columns:56px 1fr;gap:10px;align-items:baseline;
  padding:10px 0;border-top:1px solid rgba(255,255,255,.045);font-size:11.5px}
.erow:first-child{border-top:0}
.ecode{font-family:var(--mono);font-size:11px;color:#f6b1ac;font-weight:600}
.erow b{font-weight:620;color:var(--tx-1);display:block}
.erow i{font-style:normal;font-size:10.5px;color:var(--tx-4);display:block;margin-top:2px}

.qlist{display:flex;flex-direction:column}
.qrow{display:flex;flex-direction:column;gap:3px;padding:10px 0;
  border-top:1px solid rgba(255,255,255,.045)}
.qrow:first-child{border-top:0}
.qrow b{font-size:12px;font-weight:640}
.qrow span{font-size:11px;color:var(--tx-3);line-height:1.65}
</style>

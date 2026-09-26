// 本文件负责IVR 节点属性编辑器。
import { html, nothing } from "lit";
import { NODE_TYPES } from "../ivr-model.js";

const nodeTitle = (id, node) => `${NODE_TYPES[node?.type] || "未知"} · ${node?.prompt || id}`;

export function renderTargetSelect(host, value, onChange, exclude="") {
    return html`<select .value=${value||""} @change=${e=>onChange(e.target.value)}><option value="">请选择节点</option>${Object.entries(host.draft.nodes||{}).filter(([id])=>id!==exclude).map(([id,n])=>html`<option value=${id}>${nodeTitle(id,n)}</option>`)}</select>`;
  }

export function renderNodeEditor(host) {
    const id=host.selectedNode, n=host.draft.nodes?.[id];
    if (!n) return html`<p class="muted">在画布上选择一个节点，或从左侧添加节点。</p>`;
    return html`<h3>${NODE_TYPES[n.type]} <span class="muted">${id}</span></h3>
      <div class="row"><button @click=${()=>host.mutate(d=>{d.start=id})} ?disabled=${host.draft.start===id}>设为开始</button><button class="danger" @click=${()=>host.deleteNode()}>删除节点</button></div>
      ${["play","menu"].includes(n.type) ? html`
        <label>提示文字（用于管理与事件展示）<input .value=${n.prompt||""} @input=${e=>host.updateNode("prompt",e.target.value)} placeholder="例如：按 1 语音，按 2 视频" /></label>
        <label>来电实际播放的 WAV 素材<select .value=${n.file||""} @change=${e=>host.updateNode("file",e.target.value)}><option value="">请选择素材</option>${host.assets.map(a=>html`<option value=${`${a.id}.wav`}>${a.name}</option>`)}</select></label>
        <div class="row"><button @click=${()=>host.preview(n.file)} ?disabled=${!n.file}>试听</button><span class="muted">PCM 16 位、单声道、8/16 kHz</span></div>
        ${host.previewUrl ? html`<audio controls src=${host.previewUrl}></audio>` : nothing}
        <label>${n.type==="menu"?"等待按键秒数":"播后跳转秒数"}<input type="number" min="1" max="120" .value=${String(n.timeout_sec||"")} @input=${e=>host.updateNode("timeout_sec",Number(e.target.value))} /></label>
      ` : nothing}
      ${n.type==="play" ? html`<label>下一节点${renderTargetSelect(host, n.next,v=>host.updateNode("next",v),id)}</label>` : nothing}
      ${n.type==="menu" ? html`
        <label>按键分支</label>
        ${Object.entries(n.choices||{}).map(([digit,target])=>html`<div class="rule"><select class="mini" .value=${digit} @change=${e=>host.changeChoice(digit,e.target.value)}>${"1234567890*#".split("").map(d=>html`<option value=${d}>${d}</option>`)}</select>${renderTargetSelect(host, target,v=>host.mutate(d=>{d.nodes[id].choices[digit]=v}),id)}<button class="danger" aria-label=${`删除按键 ${digit}`} @click=${()=>host.mutate(d=>{delete d.nodes[id].choices[digit]})}>×</button></div>`)}
        <button @click=${()=>host.addChoice()}>＋ 添加按键</button>
        <label>超时去向${renderTargetSelect(host, n.default,v=>host.updateNode("default",v),id)}</label>
        <label>无效按键次数<input type="number" min="0" max="5" .value=${String(n.max_retries??2)} @input=${e=>host.updateNode("max_retries",Number(e.target.value))} /></label>
        <label>多次无效后去向${renderTargetSelect(host, n.invalid,v=>host.updateNode("invalid",v),id)}</label>
      ` : nothing}
      ${n.type==="time_check" ? html`<p class="muted">使用呼入队列配置的营业时间。</p><label>营业时${renderTargetSelect(host, n.open,v=>host.updateNode("open",v),id)}</label><label>非营业时${renderTargetSelect(host, n.closed,v=>host.updateNode("closed",v),id)}</label>` : nothing}
      ${n.type==="route_queue" ? html`<label>目标队列<select .value=${n.queue_id||""} @change=${e=>host.updateNode("queue_id",e.target.value)}><option value="">请选择队列</option>${host.queues.map(q=>html`<option value=${q.id}>${q.name}</option>`)}</select></label><label>通话类型<select .value=${n.session_type||"audio"} @change=${e=>host.updateNode("session_type",e.target.value)}><option value="audio">语音</option><option value="video">视频</option></select></label>` : nothing}
      ${n.type==="hangup" ? html`<p class="muted">执行到此节点后结束通话。</p>` : nothing}`;
  }

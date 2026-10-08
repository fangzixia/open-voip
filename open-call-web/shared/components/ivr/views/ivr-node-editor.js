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
        <div class="row"><button @click=${()=>host.preview(n.file, true)} ?disabled=${!n.file||host.previewBusy}>试听</button><span class="muted">PCM 16 位、单声道、8/16 kHz</span></div>
        ${host.previewUrl ? html`<audio class="ivr-preview" controls src=${host.previewUrl}></audio>` : nothing}
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
      ${n.type==="business_action" ? html`
        <p class="muted">软交换等待业务系统提交结果；客户查询和业务判断在业务系统执行。</p>
        <label>业务动作名称<input .value=${n.action||""} @input=${e=>host.updateNode("action",e.target.value)} placeholder="customer.eligible" /></label>
        <label>等待结果秒数<input type="number" min="1" max="120" .value=${String(n.timeout_sec||10)} @input=${e=>host.updateNode("timeout_sec",Number(e.target.value))} /></label>
        ${Object.entries(n.choices||{}).map(([key,target])=>html`<div class="rule"><input aria-label="业务结果" .value=${key} @change=${e=>host.changeChoice(key,e.target.value)} />${renderTargetSelect(host,target,v=>host.mutate(d=>{d.nodes[id].choices[key]=v}),id)}<button @click=${()=>host.mutate(d=>{delete d.nodes[id].choices[key]})}>删除</button></div>`)}
        <button @click=${()=>host.mutate(d=>{const choices=d.nodes[id].choices;let i=1;while(("result_"+i) in choices)i++;choices["result_"+i]="";})}>添加业务结果</button>
        <label>超时去向${renderTargetSelect(host,n.default,v=>host.updateNode("default",v),id)}</label>
      ` : nothing}
      ${n.type==="time_condition" ? html`<label>营业时间 JSON<textarea rows="6" .value=${n.schedule||""} @input=${e=>host.updateNode("schedule",e.target.value)} placeholder='{"timezone":"Asia/Shanghai","mon":"09:00-18:00"}'></textarea></label><p class="muted">与队列 business_hours_json 相同结构；留空或 always 表示始终营业。</p><label>营业时${renderTargetSelect(host, n.open,v=>host.updateNode("open",v),id)}</label><label>非营业时${renderTargetSelect(host, n.closed,v=>host.updateNode("closed",v),id)}</label>` : nothing}
      ${n.type==="queue_condition" ? html`<label>队列<select .value=${n.queue_id||""} @change=${e=>host.updateNode("queue_id",e.target.value)}><option value="">请选择</option>${(host.queues||[]).filter(q=>!q.video_enabled).map(q=>html`<option value=${q.id}>${q.name}</option>`)}</select></label><label>等待人数 &gt; <input type="number" min="0" .value=${String(n.waiting_gt??0)} @input=${e=>host.updateNode("waiting_gt",Number(e.target.value))} /> 视为忙碌</label><label>空闲时${renderTargetSelect(host, n.open,v=>host.updateNode("open",v),id)}</label><label>忙碌时${renderTargetSelect(host, n.busy||n.closed,v=>host.updateNode("busy",v),id)}</label>` : nothing}
      ${n.type==="voicemail" ? html`<label>提示文字<input .value=${n.prompt||""} @input=${e=>host.updateNode("prompt",e.target.value)} /></label><label>可选提示 WAV<select .value=${n.file||""} @change=${e=>host.updateNode("file",e.target.value)}><option value="">无</option>${host.assets.map(a=>html`<option value=${`${a.id}.wav`}>${a.name}</option>`)}</select></label><p class="muted">执行后进入留言录音并结束流程。</p>` : nothing}
      ${n.type==="csat" ? html`<label>提示文字<input .value=${n.prompt||""} @input=${e=>host.updateNode("prompt",e.target.value)} /></label><label>可选引导 WAV<select .value=${n.file||""} @change=${e=>host.updateNode("file",e.target.value)}><option value="">无</option>${host.assets.map(a=>html`<option value=${`${a.id}.wav`}>${a.name}</option>`)}</select></label><label>等待按键秒数<input type="number" min="1" max="120" .value=${String(n.timeout_sec||8)} @input=${e=>host.updateNode("timeout_sec",Number(e.target.value))} /></label><label>打分后（1–5）${renderTargetSelect(host, n.next,v=>host.updateNode("next",v),id)}</label><label>超时去向${renderTargetSelect(host, n.default,v=>host.updateNode("default",v),id)}</label>` : nothing}
      ${n.type==="collect_input" ? html`
        <label>结果名称<input .value=${n.result_key||""} @input=${e=>host.updateNode("result_key",e.target.value)} /></label>
        <label>允许的按键<input .value=${n.accepted_digits||""} @input=${e=>host.updateNode("accepted_digits",e.target.value)} placeholder="例如：0123456789*#" /></label>
        <label>提示文字<input .value=${n.prompt||""} @input=${e=>host.updateNode("prompt",e.target.value)} /></label>
        <label>可选引导 WAV<select .value=${n.file||""} @change=${e=>host.updateNode("file",e.target.value)}><option value="">无</option>${host.assets.map(a=>html`<option value=${`${a.id}.wav`}>${a.name}</option>`)}</select></label>
        <label>等待按键秒数<input type="number" min="1" max="120" .value=${String(n.timeout_sec||8)} @input=${e=>host.updateNode("timeout_sec",Number(e.target.value))} /></label>
        <label>采集后${renderTargetSelect(host,n.next,v=>host.updateNode("next",v),id)}</label>
        <label>超时去向${renderTargetSelect(host,n.default,v=>host.updateNode("default",v),id)}</label>
      ` : nothing}
      ${n.type==="route_queue" ? (() => {
        const session = n.session_type || "audio";
        const wantVideo = session === "video";
        const opts = (host.queues || []).filter((q) => !!q.video_enabled === wantVideo);
        return html`<label>通话类型<select .value=${session} @change=${e=>{host.updateNode("session_type",e.target.value);host.updateNode("queue_id","");}}><option value="audio">语音</option><option value="video">视频</option></select></label><label>目标队列<select .value=${n.queue_id||""} @change=${e=>host.updateNode("queue_id",e.target.value)}><option value="">请选择${wantVideo?"视频":"语音"}队列</option>${opts.map(q=>html`<option value=${q.id}>${q.name}</option>`)}</select></label><p class="muted">${wantVideo ? "视频队列仅用于访客端场景。" : "电话呼入请选择语音队列。"}</p>`;
      })() : nothing}
      ${n.type==="hangup" ? html`<p class="muted">执行到此节点后结束通话。</p>` : nothing}`;
  }

import { LitElement, html, svg, css, nothing } from "lit";
import { createIvrFlow, getIvrFlow, updateIvrFlow, deleteIvrFlow, listIvrVersions, rollbackIvrFlow, publishIvr, listIvrAssets, uploadIvrAsset, fetchIvrAsset, patchQueue } from "../shared/api.js";
import { NODE_TYPES, outgoing, validateIVR, simulateIVR, layoutIVR } from "./ivr-model.js";
import { formatDateTime } from "../shared/datetime.js";
import { tokenStyles } from "../shared/styles/tokens.js";

const blank = () => ({ start: "", nodes: {} });
const copy = value => structuredClone(value);
const nodeTitle = (id, node) => `${NODE_TYPES[node?.type] || "未知"} · ${node?.prompt || id}`;

export class IVRFlowEditor extends LitElement {
  static properties = {
    flows: { type: Array }, queues: { type: Array }, assets: { type: Array }, versions: { type: Array },
    selectedId: { type: String }, selectedNode: { type: String }, flowName: { type: String }, draft: { type: Object },
    dirty: { type: Boolean }, creating: { type: Boolean }, busy: { type: Boolean }, notice: { type: String }, problem: { type: String },
    testDigits: { type: String }, testOpen: { type: Boolean }, simulation: { type: Object }, previewUrl: { type: String }, bindingQueueId: { type: String },
    linkDraft: { type: Object },
  };
  static styles = [tokenStyles, css`
    :host { display:block; color:var(--ov-text); font:14px/1.5 Inter,-apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei",sans-serif; }
    * { box-sizing:border-box; }
    button,input,select { font:inherit; }
    button { cursor:pointer; border-radius:var(--ov-radius-sm); border:1px solid var(--ov-border-strong); background:#fff; color:var(--ov-text); min-height:36px; padding:4px 12px; }
    button:hover { color:var(--ov-primary); border-color:var(--ov-primary); }
    button:focus-visible,input:focus-visible,select:focus-visible { outline:none; box-shadow:var(--ov-focus); }
    button:disabled { opacity:.5; cursor:not-allowed; }
    .primary { color:white; background:var(--ov-primary); border-color:var(--ov-primary); }
    .primary:hover { color:white; background:var(--ov-primary-hover); }
    .danger { color:var(--ov-danger); }
    .ghost { border-color:transparent; background:transparent; color:var(--ov-primary); }
    .wrap { background:white; border:1px solid var(--ov-border); border-radius:var(--ov-radius-lg); box-shadow:var(--ov-shadow-sm); padding:16px; min-height:600px; }
    .header,.toolbar,.row { display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
    .header { justify-content:space-between; margin-bottom:12px; }
    .header h2 { margin:0; font-size:18px; }
    .header .flow-name { width:220px; margin-top:5px; }
    .muted { color:#737373; }
    .status { margin:0 0 10px; }
    .error { color:#cf1322; background:#fff1f0; border:1px solid #ffa39e; padding:8px; margin:8px 0; }
    .success { color:#135200; background:#f6ffed; padding:8px; margin:8px 0; }
    .workspace { display:grid; grid-template-columns:minmax(175px,210px) minmax(360px,1fr) minmax(250px,300px); gap:14px; align-items:start; }
    .pane { border:1px solid #e5e5e5; padding:12px; min-width:0; }
    .pane h3 { margin:0 0 10px; font-size:14px; }
    .list { display:grid; gap:6px; }
    .list button { text-align:left; }
    .list button.active { color:var(--ov-primary); border-color:#c9c7f4; background:var(--ov-primary-soft); }
    .flow-list { margin-bottom:18px; max-height:220px; overflow-y:auto; }
    .rule { display:flex; align-items:center; gap:5px; margin-top:6px; }
    .rule select { flex:1; min-width:0; }
    .canvas { overflow:auto; background:var(--ov-surface-soft); min-height:450px; }
    svg { display:block; min-width:100%; }
    .edge { stroke:#8c8c8c; stroke-width:1.5; fill:none; }
    .edge-label { fill:#595959; font-size:11px; paint-order:stroke; stroke:#fafafa; stroke-width:5px; }
    .node rect { fill:white; stroke:#b8b5eb; stroke-width:1.5; rx:8; }
    .node.selected rect { fill:#f1f0ff; stroke:#5b5bd6; stroke-width:2.5; }
    .node text { fill:#262626; font-size:13px; pointer-events:none; }
    .node .sub { fill:#737373; font-size:11px; }
    .node { cursor:pointer; }
    .node:focus { outline:none; }
    .node:focus rect { stroke:#5b5bd6; stroke-width:3; }
    .port { fill:#5b5bd6; stroke:#fff; stroke-width:2; cursor:crosshair; }
    .node.dragging rect { stroke:#4a4ac4; stroke-dasharray:5 3; }
    .link-preview { stroke:#5b5bd6; stroke-width:2; stroke-dasharray:5 4; fill:none; pointer-events:none; }
    .link-editor { margin-top:10px; padding:10px; background:var(--ov-primary-soft); border:1px solid #c9c7f4; }
    .palette-item { cursor:grab; }
    .canvas.drag-over { outline:2px dashed var(--ov-primary); outline-offset:-3px; }
    label { display:block; font-weight:500; margin:9px 0 4px; }
    input,select { width:100%; min-height:36px; padding:4px 9px; border:1px solid var(--ov-border-strong); border-radius:var(--ov-radius-sm); background:white; color:var(--ov-text); }
    input[type=file] { height:auto; }
    .mini { width:80px; flex:none; }
    .two { display:grid; grid-template-columns:1fr 1fr; gap:8px; }
    .divider { border-top:1px solid #eee; margin:14px 0; }
    .details { margin-top:14px; display:grid; gap:10px; grid-template-columns:repeat(auto-fit,minmax(240px,1fr)); }
    .details .pane { height:fit-content; }
    .simulation { padding:8px 0; line-height:1.9; }
    .badge { color:var(--ov-primary); background:var(--ov-primary-soft); border-radius:999px; padding:2px 8px; margin-right:5px; display:inline-block; }
    audio { width:100%; max-width:300px; margin-top:8px; }
    .version { display:flex; gap:8px; align-items:center; justify-content:space-between; padding:6px 0; border-bottom:1px solid #eee; }
    @media(max-width:1100px) { .workspace { grid-template-columns:180px 1fr; } .inspector { grid-column:1 / -1; } }
    @media(max-width:700px) { .workspace { grid-template-columns:1fr; } .inspector { grid-column:auto; } .wrap { padding:10px; } }
  `];

  constructor() {
    super(); this.flows=[]; this.queues=[]; this.assets=[]; this.versions=[]; this.selectedId=""; this.selectedNode="";
    this.flowName=""; this.draft=blank(); this.dirty=false; this.creating=false; this.busy=false; this.notice=""; this.problem="";
    this.testDigits="1"; this.testOpen=true; this.simulation=null; this.previewUrl=""; this.bindingQueueId="";
    this.linkDraft=null; this.pointerDrag=null; this.dragPoint=null;
  }
  connectedCallback() { super.connectedCallback(); void this.loadAssets(); }
  disconnectedCallback() { super.disconnectedCallback(); if (this.previewUrl) URL.revokeObjectURL(this.previewUrl); this.stopPointer(); }
  willUpdate(changed) {
    if (changed.has("flows") && !this.selectedId && !this.creating && this.flows?.length) this.chooseFlow(this.flows[0].id);
  }
  async loadAssets() { try { this.assets=(await listIvrAssets()).items||[]; } catch(e) { this.problem=e.message; } }
  async loadVersions() { if (!this.selectedId) { this.versions=[]; return; } try { this.versions=(await listIvrVersions(this.selectedId)).items||[]; } catch(e) { this.problem=e.message; } }
  chooseFlow(id) {
    if (this.dirty) { this.problem="请先保存草稿，再切换流程"; return; }
    const flow=this.flows.find(f=>f.id===id); if (!flow) return;
    this.selectedId=id; this.flowName=flow.name; this.draft=copy(flow.draft||blank()); this.selectedNode=this.draft.start||Object.keys(this.draft.nodes||{})[0]||"";
    this.creating=false; this.problem=""; this.notice=""; this.simulation=null; this.bindingQueueId=this.queues.find(q=>q.ivr_flow_id===id)?.id||"";
    void this.loadVersions();
  }
  newFlow() {
    if (this.dirty) { this.problem="请先保存草稿，再新建流程"; return; }
    this.creating=true; this.selectedId=""; this.flowName="新流程"; this.draft=blank(); this.selectedNode=""; this.versions=[]; this.problem=""; this.notice=""; this.dirty=true;
  }
  /** 克隆草稿后应用修改，触发 Lit 更新并清除旧的提示与模拟结果。 */
  mutate(fn) { const draft=copy(this.draft); draft.nodes ||= {}; fn(draft); this.draft=draft; this.dirty=true; this.problem=""; this.notice=""; this.simulation=null; }
  updateNode(key,value) { if (!this.selectedNode) return; this.mutate(d=>{ d.nodes[this.selectedNode][key]=value; }); }
  addNode(type, position = null) {
    const id=`node_${crypto.randomUUID().slice(0,8)}`;
    const defaults={ play:{type,prompt:"",file:"",timeout_sec:2,next:""}, menu:{type,prompt:"",file:"",timeout_sec:8,max_retries:2,choices:{},default:"",invalid:""}, time_check:{type,open:"",closed:""}, route_queue:{type,queue_id:this.queues[0]?.id||"",session_type:"audio"}, hangup:{type} };
    this.mutate(d=>{ d.nodes[id]=defaults[type]; if (!d.start) d.start=id; if (position) { d.layout ||= {}; d.layout[id]=position; } }); this.selectedNode=id;
  }
  /** 删除节点时一并清理起点、布局及其他节点指向它的分支。 */
  deleteNode() {
    const id=this.selectedNode; if (!id) return;
    this.mutate(d=>{
      delete d.nodes[id]; if (d.start===id) d.start=Object.keys(d.nodes)[0]||"";
      if (d.layout) delete d.layout[id];
      for (const n of Object.values(d.nodes)) {
        for (const key of ["next","default","invalid","open","closed"]) if (n[key]===id) n[key]="";
        for (const [digit,target] of Object.entries(n.choices||{})) if (target===id) delete n.choices[digit];
      }
    }); this.selectedNode=this.draft.start;
  }
  addChoice() {
    const node=this.draft.nodes[this.selectedNode]; const digit="1234567890*#".split("").find(x=>!(x in (node.choices||{})));
    if (!digit) { this.problem="按键已全部使用"; return; }
    this.mutate(d=>{ d.nodes[this.selectedNode].choices[digit]=""; });
  }
  canvasPoint(event) {
    const svg=this.renderRoot.querySelector(".canvas svg");
    if (!svg) return {x:105,y:75};
    const rect=svg.getBoundingClientRect();
    const x=(event.clientX-rect.left)*Number(svg.getAttribute("width"))/rect.width;
    const y=(event.clientY-rect.top)*Number(svg.getAttribute("height"))/rect.height;
    return {x:Math.min(4000,Math.max(90,Math.round(x))),y:Math.min(4000,Math.max(45,Math.round(y)))};
  }
  onPaletteDrag(event,type) {
    event.dataTransfer.effectAllowed="copy";
    event.dataTransfer.setData("application/x-ivr-node",type);
  }
  onCanvasDrop(event) {
    event.preventDefault(); event.currentTarget.classList.remove("drag-over");
    const type=event.dataTransfer.getData("application/x-ivr-node");
    if (NODE_TYPES[type]) this.addNode(type,this.canvasPoint(event));
  }
  startPointer(event,kind,id) {
    if (event.button!==0) return;
    event.preventDefault(); event.stopPropagation();
    this.stopPointer();
    this.pointerDrag={kind,id,startX:event.clientX,startY:event.clientY}; this.dragPoint=this.canvasPoint(event);
    this.pointerMoveHandler=e=>{ if (!this.pointerDrag) return; this.dragPoint=this.canvasPoint(e); this.requestUpdate(); };
    this.pointerUpHandler=e=>this.finishPointer(e);
    window.addEventListener("pointermove",this.pointerMoveHandler);
    window.addEventListener("pointerup",this.pointerUpHandler,{once:true});
  }
  stopPointer() {
    if (this.pointerMoveHandler) window.removeEventListener("pointermove",this.pointerMoveHandler);
    if (this.pointerUpHandler) window.removeEventListener("pointerup",this.pointerUpHandler);
    this.pointerMoveHandler=null; this.pointerUpHandler=null;
  }
  /** 根据拖拽类型保存节点位置，或准备一条待确认的连线。 */
  finishPointer(event) {
    const drag=this.pointerDrag, point=this.canvasPoint(event);
    this.stopPointer(); this.pointerDrag=null; this.dragPoint=null;
    if (!drag) return;
    if (drag.kind==="move") {
      if (Math.hypot(event.clientX-drag.startX,event.clientY-drag.startY)>4) this.mutate(d=>{d.layout ||= {}; d.layout[drag.id]=point;});
      this.selectedNode=drag.id;
    } else {
      const positions=layoutIVR(this.draft).positions;
      const target=Object.entries(positions).find(([id,p])=>id!==drag.id && Math.abs(p.x-point.x)<80 && Math.abs(p.y-point.y)<35)?.[0];
      if (target) {
        const source=this.draft.nodes[drag.id];
        const branch=source.type==="play"?"next":source.type==="time_check"?"open":Object.keys(source.choices||{})[0]?`choice:${Object.keys(source.choices)[0]}`:"default";
        this.linkDraft={from:drag.id,to:target,branch}; this.selectedNode=drag.id;
      }
    }
    this.requestUpdate();
  }
  /** 将已选择的分支目标写回草稿。 */
  applyLink() {
    const link=this.linkDraft; if (!link) return;
    this.mutate(d=>{
      const node=d.nodes[link.from];
      if (link.branch.startsWith("choice:")) { node.choices ||= {}; node.choices[link.branch.slice(7)]=link.to; }
      else node[link.branch]=link.to;
    });
    this.linkDraft=null;
  }
  linkEditor() {
    const link=this.linkDraft; if (!link) return nothing;
    const source=this.draft.nodes[link.from];
    const branches=source.type==="play"?[["next","下一步"]]:source.type==="time_check"?[["open","营业"],["closed","非营业"]]:[
      ..."1234567890*#".split("").map(d=>[`choice:${d}`,`按 ${d}`]),["default","超时"],["invalid","无效按键"]
    ];
    return html`<div class="link-editor"><strong>连接 ${NODE_TYPES[source.type]} → ${NODE_TYPES[this.draft.nodes[link.to].type]}</strong>
      <label>触发条件<select .value=${link.branch} @change=${e=>this.linkDraft={...link,branch:e.target.value}}>${branches.map(([value,label])=>html`<option value=${value}>${label}</option>`)}</select></label>
      <div class="row"><button class="primary" @click=${()=>this.applyLink()}>连接</button><button @click=${()=>this.linkDraft=null}>取消</button></div></div>`;
  }
  changeChoice(oldDigit,newDigit) {
    if (newDigit!==oldDigit && this.draft.nodes[this.selectedNode].choices[newDigit]!==undefined) { this.problem="此按键已配置"; return; }
    this.mutate(d=>{ const choices=d.nodes[this.selectedNode].choices; const target=choices[oldDigit]; delete choices[oldDigit]; choices[newDigit]=target; });
  }
  async save() {
    if (!this.flowName.trim()) { this.problem="请输入流程名称"; return null; }
    this.busy=true; this.problem="";
    try {
      const flow=this.selectedId ? await updateIvrFlow(this.selectedId,this.flowName.trim(),this.draft) : await createIvrFlow(this.flowName.trim(),this.draft);
      this.selectedId=flow.id; this.creating=false; this.dirty=false; this.notice="草稿已保存";
      this.flows=[...this.flows.filter(f=>f.id!==flow.id),flow];
      this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true}));
      return flow;
    } catch(e) { this.problem=e.message; return null; }
    finally { this.busy=false; }
  }
  async publish() {
    const issues=validateIVR(this.draft,this.queues,this.assets);
    if (issues.length) { this.problem=issues.join("；"); return; }
    if (this.dirty && !await this.save()) return;
    this.busy=true;
    try { const version=await publishIvr(this.selectedId); this.flows=this.flows.map(f=>f.id===this.selectedId?{...f,published_version:version.version}:f); this.notice=`版本 v${version.version} 已发布，新呼入将使用该版本`; this.problem=""; await this.loadVersions(); this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async rollback(version) {
    if (this.dirty) { this.problem="请先保存草稿"; return; }
    if (!window.confirm(`将版本 v${version} 复制并发布为新版本？`)) return;
    this.busy=true;
    try { const result=await rollbackIvrFlow(this.selectedId,version); const flow=await getIvrFlow(this.selectedId); this.flows=this.flows.map(f=>f.id===flow.id?flow:f); this.draft=copy(flow.draft); this.selectedNode=this.draft.start; this.dirty=false; this.notice=`已回滚并发布 v${result.version}`; this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); await this.loadVersions(); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async removeFlow() {
    if (!this.selectedId || !window.confirm(`删除流程“${this.flowName}”？已绑定队列的流程不能删除。`)) return;
    this.busy=true;
    try { await deleteIvrFlow(this.selectedId); this.flows=this.flows.filter(f=>f.id!==this.selectedId); this.selectedId=""; this.draft=blank(); this.selectedNode=""; this.dirty=false; this.notice="流程已删除"; this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async bindQueue() {
    if (!this.selectedId || !this.bindingQueueId) { this.problem="请选择已发布流程和队列"; return; }
    const published=this.flows.find(f=>f.id===this.selectedId)?.published_version;
    if (!published) { this.problem="请先发布流程再绑定队列"; return; }
    this.busy=true;
    try { await patchQueue(this.bindingQueueId,{ivr_flow_id:this.selectedId}); this.notice="队列已绑定此流程"; this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async upload(event) {
    const file=event.target.files?.[0]; if (!file) return;
    this.busy=true;
    try { const asset=await uploadIvrAsset(file); await this.loadAssets(); if (this.selectedNode && ["play","menu"].includes(this.draft.nodes[this.selectedNode]?.type)) this.updateNode("file",`${asset.id}.wav`); this.notice=`已上传 ${asset.name}`; }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; event.target.value=""; }
  }
  async preview(file) {
    if (!file) return;
    try { const blob=await fetchIvrAsset(file.replace(/\.wav$/i,"")); if (this.previewUrl) URL.revokeObjectURL(this.previewUrl); this.previewUrl=URL.createObjectURL(blob); this.problem=""; }
    catch(e) { this.problem=e.message; }
  }
  runSimulation() {
    const issues=validateIVR(this.draft,this.queues,this.assets);
    if (issues.length) { this.problem=issues.join("；"); return; }
    this.simulation=simulateIVR(this.draft,{digits:this.testDigits,open:this.testOpen}); this.problem="";
  }
  targetSelect(value, onChange, exclude="") {
    return html`<select .value=${value||""} @change=${e=>onChange(e.target.value)}><option value="">请选择节点</option>${Object.entries(this.draft.nodes||{}).filter(([id])=>id!==exclude).map(([id,n])=>html`<option value=${id}>${nodeTitle(id,n)}</option>`)}</select>`;
  }
  nodeEditor() {
    const id=this.selectedNode, n=this.draft.nodes?.[id];
    if (!n) return html`<p class="muted">在画布上选择一个节点，或从左侧添加节点。</p>`;
    return html`<h3>${NODE_TYPES[n.type]} <span class="muted">${id}</span></h3>
      <div class="row"><button @click=${()=>this.mutate(d=>{d.start=id})} ?disabled=${this.draft.start===id}>设为开始</button><button class="danger" @click=${()=>this.deleteNode()}>删除节点</button></div>
      ${["play","menu"].includes(n.type) ? html`
        <label>提示文字（用于管理与事件展示）<input .value=${n.prompt||""} @input=${e=>this.updateNode("prompt",e.target.value)} placeholder="例如：按 1 语音，按 2 视频" /></label>
        <label>来电实际播放的 WAV 素材<select .value=${n.file||""} @change=${e=>this.updateNode("file",e.target.value)}><option value="">请选择素材</option>${this.assets.map(a=>html`<option value=${`${a.id}.wav`}>${a.name}</option>`)}</select></label>
        <div class="row"><button @click=${()=>this.preview(n.file)} ?disabled=${!n.file}>试听</button><span class="muted">PCM 16 位、单声道、8/16 kHz</span></div>
        ${this.previewUrl ? html`<audio controls src=${this.previewUrl}></audio>` : nothing}
        <label>${n.type==="menu"?"等待按键秒数":"播后跳转秒数"}<input type="number" min="1" max="120" .value=${String(n.timeout_sec||"")} @input=${e=>this.updateNode("timeout_sec",Number(e.target.value))} /></label>
      ` : nothing}
      ${n.type==="play" ? html`<label>下一节点${this.targetSelect(n.next,v=>this.updateNode("next",v),id)}</label>` : nothing}
      ${n.type==="menu" ? html`
        <label>按键分支</label>
        ${Object.entries(n.choices||{}).map(([digit,target])=>html`<div class="rule"><select class="mini" .value=${digit} @change=${e=>this.changeChoice(digit,e.target.value)}>${"1234567890*#".split("").map(d=>html`<option value=${d}>${d}</option>`)}</select>${this.targetSelect(target,v=>this.mutate(d=>{d.nodes[id].choices[digit]=v}),id)}<button class="danger" aria-label=${`删除按键 ${digit}`} @click=${()=>this.mutate(d=>{delete d.nodes[id].choices[digit]})}>×</button></div>`)}
        <button @click=${()=>this.addChoice()}>＋ 添加按键</button>
        <label>超时去向${this.targetSelect(n.default,v=>this.updateNode("default",v),id)}</label>
        <label>无效按键次数<input type="number" min="0" max="5" .value=${String(n.max_retries??2)} @input=${e=>this.updateNode("max_retries",Number(e.target.value))} /></label>
        <label>多次无效后去向${this.targetSelect(n.invalid,v=>this.updateNode("invalid",v),id)}</label>
      ` : nothing}
      ${n.type==="time_check" ? html`<p class="muted">使用呼入队列配置的营业时间。</p><label>营业时${this.targetSelect(n.open,v=>this.updateNode("open",v),id)}</label><label>非营业时${this.targetSelect(n.closed,v=>this.updateNode("closed",v),id)}</label>` : nothing}
      ${n.type==="route_queue" ? html`<label>目标队列<select .value=${n.queue_id||""} @change=${e=>this.updateNode("queue_id",e.target.value)}><option value="">请选择队列</option>${this.queues.map(q=>html`<option value=${q.id}>${q.name}</option>`)}</select></label><label>通话类型<select .value=${n.session_type||"audio"} @change=${e=>this.updateNode("session_type",e.target.value)}><option value="audio">语音</option><option value="video">视频</option></select></label>` : nothing}
      ${n.type==="hangup" ? html`<p class="muted">执行到此节点后结束通话。</p>` : nothing}`;
  }
  canvas() {
    const graph=layoutIVR(this.draft); const nodes=this.draft.nodes||{};
    if (this.pointerDrag?.kind==="move" && graph.positions[this.pointerDrag.id]) graph.positions[this.pointerDrag.id]=this.dragPoint;
    const source=this.pointerDrag?.kind==="link"?graph.positions[this.pointerDrag.id]:null;
    return svg`<svg viewBox=${`0 0 ${graph.width} ${graph.height}`} width=${graph.width} height=${graph.height} role="img" aria-label="IVR 流程图；拖动节点可调整位置，从节点下方圆点拖动到目标节点可连线">
      <defs><marker id="ivr-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0 0 L8 4 L0 8" fill="#8c8c8c" /></marker></defs>
      ${Object.keys(nodes).length?nothing:svg`<text x="25" y="55" fill="#737373">从左侧拖入第一个节点，建立流程入口。</text>`}
      ${graph.edges.map(e=>{ const a=graph.positions[e.from],b=graph.positions[e.to]; const mx=(a.x+b.x)/2,my=(a.y+b.y)/2; return svg`<path class="edge" marker-end="url(#ivr-arrow)" d=${`M ${a.x} ${a.y+32} C ${a.x} ${my} ${b.x} ${my} ${b.x} ${b.y-39}`} /><text class="edge-label" x=${mx} y=${my-4} text-anchor="middle">${e.label}</text>`; })}
      ${source&&this.dragPoint?svg`<path class="link-preview" d=${`M ${source.x} ${source.y+31} L ${this.dragPoint.x} ${this.dragPoint.y}`} />`:nothing}
      ${Object.entries(graph.positions).map(([id,p])=>svg`<g class=${`node ${id===this.selectedNode?"selected":""} ${this.pointerDrag?.kind==="move"&&this.pointerDrag.id===id?"dragging":""}`} role="button" tabindex="0" aria-label=${nodeTitle(id,nodes[id])} @pointerdown=${e=>this.startPointer(e,"move",id)} @click=${()=>{this.selectedNode=id; this.previewUrl=""}} @keydown=${e=>{if(e.key==="Enter"||e.key===" "){e.preventDefault();this.selectedNode=id}}}><rect x=${p.x-80} y=${p.y-31} width="160" height="62"></rect><text x=${p.x} y=${p.y-5} text-anchor="middle">${NODE_TYPES[nodes[id].type]}</text><text class="sub" x=${p.x} y=${p.y+15} text-anchor="middle">${(nodes[id].prompt||this.queues.find(q=>q.id===nodes[id].queue_id)?.name||id).slice(0,16)}</text>${["play","menu","time_check"].includes(nodes[id].type)?svg`<circle class="port" cx=${p.x} cy=${p.y+31} r="7" aria-label="拖动连线" @pointerdown=${e=>this.startPointer(e,"link",id)}></circle>`:nothing}</g>`)}
    </svg>`;
  }
  render() {
    const published=this.flows.find(f=>f.id===this.selectedId)?.published_version;
    return html`<div class="wrap">
      <div class="header"><div><h2>IVR 流程设计器</h2><input class="flow-name" aria-label="流程名称" .value=${this.flowName} @input=${e=>{this.flowName=e.target.value;this.dirty=true}} placeholder="流程名称" /><br /><span class="muted">${this.selectedId ? `线上 v${published||"未发布"}` : "新流程"} ${this.dirty?" · 草稿有未保存修改":""}</span></div>
      <div class="toolbar"><button @click=${()=>this.mutate(d=>{delete d.layout})}>自动排列</button><button @click=${()=>this.runSimulation()}>模拟路径</button><button @click=${()=>this.save()} ?disabled=${this.busy}>保存草稿</button><button class="primary" @click=${()=>this.publish()} ?disabled=${this.busy}>检查并发布</button></div></div>
      ${this.problem ? html`<div class="error" role="alert">${this.problem}</div>` : nothing}${this.notice ? html`<div class="success" role="status">${this.notice}</div>` : nothing}
      <div class="workspace">
        <div class="pane"><h3>流程</h3><div class="list flow-list">${this.flows.map(f=>html`<button class=${this.selectedId===f.id?"active":""} @click=${()=>this.chooseFlow(f.id)}>${f.name}　${f.published_version?`v${f.published_version}`:"草稿"}</button>`)}</div><button @click=${()=>this.newFlow()}>＋ 新建流程</button>
          <div class="divider"></div><h3>添加节点</h3><p class="muted">拖入画布，或点击添加。</p><div class="list">${Object.entries(NODE_TYPES).map(([type,label])=>html`<button class="palette-item" draggable="true" @dragstart=${e=>this.onPaletteDrag(e,type)} @click=${()=>this.addNode(type)}>＋ ${label}</button>`)}</div></div>
        <div class="pane"><div class="header"><h3>流程画布</h3><span class="muted">开始节点：${this.draft.start||"未设置"}</span></div><div class="canvas" @dragover=${e=>{e.preventDefault();e.dataTransfer.dropEffect="copy"}} @dragenter=${e=>e.currentTarget.classList.add("drag-over")} @dragleave=${e=>{if(!e.currentTarget.contains(e.relatedTarget))e.currentTarget.classList.remove("drag-over")}} @drop=${e=>this.onCanvasDrop(e)}>${this.canvas()}</div>${this.linkEditor()}</div>
        <div class="pane inspector"><h3>节点属性</h3>${this.nodeEditor()}</div>
      </div>
      <div class="details">
        <div class="pane"><h3>语音素材</h3><label>上传语音文件<input type="file" accept=".wav,audio/wav" @change=${e=>this.upload(e)} ?disabled=${this.busy} /></label><p class="muted">已上传 ${this.assets.length} 个 WAV 素材。选择播放或菜单节点可关联录音。</p></div>
        <div class="pane"><h3>路径模拟</h3><div class="two"><label>模拟按键<input .value=${this.testDigits} @input=${e=>this.testDigits=e.target.value} placeholder="如 1 或 2；留空模拟超时" /></label><label>工作时间<select .value=${this.testOpen?"open":"closed"} @change=${e=>this.testOpen=e.target.value==="open"}><option value="open">营业</option><option value="closed">非营业</option></select></label></div><button @click=${()=>this.runSimulation()}>运行模拟</button>${this.simulation?html`<div class="simulation">${this.simulation.path.map(x=>html`<span class="badge">${NODE_TYPES[x.type]} ${x.event}</span> → `)}<strong>${this.simulation.result}</strong></div>`:nothing}</div>
        <div class="pane"><h3>绑定与版本</h3><label>绑定到呼入队列<select .value=${this.bindingQueueId} @change=${e=>this.bindingQueueId=e.target.value}><option value="">选择队列</option>${this.queues.map(q=>html`<option value=${q.id}>${q.name}</option>`)}</select></label><div class="row"><button @click=${()=>this.bindQueue()} ?disabled=${this.busy||!this.selectedId}>保存队列绑定</button><button class="danger" @click=${()=>this.removeFlow()} ?disabled=${this.busy||!this.selectedId}>删除流程</button></div><div class="divider"></div>${this.versions.map(v=>html`<div class="version"><span>v${v.version}　${formatDateTime(v.published_at)}</span><button class="ghost" @click=${()=>this.rollback(v.version)}>回滚</button></div>`)}</div>
      </div>
    </div>`;
  }
}
customElements.define("ivr-flow-editor",IVRFlowEditor);

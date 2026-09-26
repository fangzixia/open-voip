import { LitElement, html, nothing } from "lit";
import { NODE_TYPES, validateIVR, simulateIVR, layoutIVR } from "./ivr-model.js";
import { ivrEditorStyles } from "./views/ivr-styles.js";
import { renderIvrLayout } from "./views/ivr-layout.js";

const blank = () => ({ start: "", nodes: {} });
const copy = value => structuredClone(value);

export class IVRFlowEditor extends LitElement {
  static properties = {
    flows: { type: Array }, queues: { type: Array }, assets: { type: Array }, versions: { type: Array },
    selectedId: { type: String }, selectedNode: { type: String }, flowName: { type: String }, draft: { type: Object },
    dirty: { type: Boolean }, creating: { type: Boolean }, busy: { type: Boolean }, notice: { type: String }, problem: { type: String },
    testDigits: { type: String }, testOpen: { type: Boolean }, simulation: { type: Object }, previewUrl: { type: String }, bindingQueueId: { type: String },
    linkDraft: { type: Object },
    service: { attribute: false },
  };
  static styles = ivrEditorStyles;

  constructor() {
    super(); this.flows=[]; this.queues=[]; this.assets=[]; this.versions=[]; this.selectedId=""; this.selectedNode="";
    this.flowName=""; this.draft=blank(); this.dirty=false; this.creating=false; this.busy=false; this.notice=""; this.problem="";
    this.testDigits="1"; this.testOpen=true; this.simulation=null; this.previewUrl=""; this.bindingQueueId="";
    this.linkDraft=null; this.pointerDrag=null; this.dragPoint=null; this.service=null;
  }
  disconnectedCallback() { super.disconnectedCallback(); if (this.previewUrl) URL.revokeObjectURL(this.previewUrl); this.stopPointer(); }
  willUpdate(changed) {
    if ((changed.has("flows") || changed.has("service")) && this.service && !this.selectedId && !this.creating && this.flows?.length) this.chooseFlow(this.flows[0].id);
  }
  updated(changed) { if (changed.has("service") && this.service) void this.loadAssets(); }
  async loadAssets() { try { this.assets=(await this.service.listIvrAssets()).items||[]; } catch(e) { this.problem=e.message; } }
  async loadVersions() { if (!this.selectedId) { this.versions=[]; return; } try { this.versions=(await this.service.listIvrVersions(this.selectedId)).items||[]; } catch(e) { this.problem=e.message; } }
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
      const flow=this.selectedId ? await this.service.updateIvrFlow(this.selectedId,this.flowName.trim(),this.draft) : await this.service.createIvrFlow(this.flowName.trim(),this.draft);
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
    try { const version=await this.service.publishIvr(this.selectedId); this.flows=this.flows.map(f=>f.id===this.selectedId?{...f,published_version:version.version}:f); this.notice=`版本 v${version.version} 已发布，新呼入将使用该版本`; this.problem=""; await this.loadVersions(); this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async rollback(version) {
    if (this.dirty) { this.problem="请先保存草稿"; return; }
    if (!window.confirm(`将版本 v${version} 复制并发布为新版本？`)) return;
    this.busy=true;
    try { const result=await this.service.rollbackIvrFlow(this.selectedId,version); const flow=await this.service.getIvrFlow(this.selectedId); this.flows=this.flows.map(f=>f.id===flow.id?flow:f); this.draft=copy(flow.draft); this.selectedNode=this.draft.start; this.dirty=false; this.notice=`已回滚并发布 v${result.version}`; this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); await this.loadVersions(); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async removeFlow() {
    if (!this.selectedId || !window.confirm(`删除流程“${this.flowName}”？已绑定队列的流程不能删除。`)) return;
    this.busy=true;
    try { await this.service.deleteIvrFlow(this.selectedId); this.flows=this.flows.filter(f=>f.id!==this.selectedId); this.selectedId=""; this.draft=blank(); this.selectedNode=""; this.dirty=false; this.notice="流程已删除"; this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async bindQueue() {
    if (!this.selectedId || !this.bindingQueueId) { this.problem="请选择已发布流程和队列"; return; }
    const published=this.flows.find(f=>f.id===this.selectedId)?.published_version;
    if (!published) { this.problem="请先发布流程再绑定队列"; return; }
    this.busy=true;
    try { await this.service.patchQueue(this.bindingQueueId,{ivr_flow_id:this.selectedId}); this.notice="队列已绑定此流程"; this.dispatchEvent(new CustomEvent("ivr-changed",{bubbles:true,composed:true})); }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; }
  }
  async upload(event) {
    const file=event.target.files?.[0]; if (!file) return;
    this.busy=true;
    try { const asset=await this.service.uploadIvrAsset(file); await this.loadAssets(); if (this.selectedNode && ["play","menu"].includes(this.draft.nodes[this.selectedNode]?.type)) this.updateNode("file",`${asset.id}.wav`); this.notice=`已上传 ${asset.name}`; }
    catch(e) { this.problem=e.message; }
    finally { this.busy=false; event.target.value=""; }
  }
  async preview(file) {
    if (!file) return;
    try { const blob=await this.service.fetchIvrAsset(file.replace(/\.wav$/i,"")); if (this.previewUrl) URL.revokeObjectURL(this.previewUrl); this.previewUrl=URL.createObjectURL(blob); this.problem=""; }
    catch(e) { this.problem=e.message; }
  }
  runSimulation() {
    const issues=validateIVR(this.draft,this.queues,this.assets);
    if (issues.length) { this.problem=issues.join("；"); return; }
    this.simulation=simulateIVR(this.draft,{digits:this.testDigits,open:this.testOpen}); this.problem="";
  }
  render() { return renderIvrLayout(this); }
}
customElements.define("ivr-flow-editor",IVRFlowEditor);

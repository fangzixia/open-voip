// 本文件负责IVR 编辑器布局。
import { html, nothing } from "lit";
import { NODE_TYPES } from "../ivr-model.js";
import { formatDateTime } from "../../../datetime.js";
import { renderNodeEditor } from "./ivr-node-editor.js";
import { renderCanvas } from "./ivr-canvas.js";

export function renderIvrLayout(host) {
    const published=host.flows.find(f=>f.id===host.selectedId)?.published_version;
    return html`<div class="wrap">
      <div class="header"><div><h2>IVR 流程设计器</h2><input class="flow-name" aria-label="流程名称" .value=${host.flowName} @input=${e=>{host.flowName=e.target.value;host.dirty=true}} placeholder="流程名称" /><br /><span class="muted">${host.selectedId ? `线上 v${published||"未发布"}` : "新流程"} ${host.dirty?" · 草稿有未保存修改":""}</span></div>
      <div class="toolbar"><button @click=${()=>host.mutate(d=>{delete d.layout})}>自动排列</button><button @click=${()=>host.runSimulation()}>模拟路径</button><button @click=${()=>host.save()} ?disabled=${host.busy}>保存草稿</button><button class="primary" @click=${()=>host.publish()} ?disabled=${host.busy}>检查并发布</button></div></div>
      ${host.problem ? html`<div class="error" role="alert">${host.problem}</div>` : nothing}${host.notice ? html`<div class="success" role="status">${host.notice}</div>` : nothing}
      <div class="workspace">
        <div class="pane"><h3>流程</h3><div class="list flow-list">${host.flows.map(f=>html`<button class=${host.selectedId===f.id?"active":""} @click=${()=>host.chooseFlow(f.id)}>${f.name}　${f.published_version?`v${f.published_version}`:"草稿"}</button>`)}</div><button @click=${()=>host.newFlow()}>＋ 新建流程</button>
          <div class="divider"></div><h3>添加节点</h3><p class="muted">拖入画布，或点击添加。</p><div class="list">${Object.entries(NODE_TYPES).map(([type,label])=>html`<button class="palette-item" draggable="true" @dragstart=${e=>host.onPaletteDrag(e,type)} @click=${()=>host.addNode(type)}>＋ ${label}</button>`)}</div></div>
        <div class="pane"><div class="header"><h3>流程画布</h3><span class="muted">开始节点：${host.draft.start||"未设置"}</span></div><div class="canvas" @dragover=${e=>{e.preventDefault();e.dataTransfer.dropEffect="copy"}} @dragenter=${e=>e.currentTarget.classList.add("drag-over")} @dragleave=${e=>{if(!e.currentTarget.contains(e.relatedTarget))e.currentTarget.classList.remove("drag-over")}} @drop=${e=>host.onCanvasDrop(e)}>${renderCanvas(host)}</div>${host.linkEditor()}</div>
        <div class="pane inspector"><h3>节点属性</h3>${renderNodeEditor(host)}</div>
      </div>
      <div class="details">
        <div class="pane"><h3>语音素材</h3><label>上传语音文件<input type="file" accept=".wav,audio/wav" @change=${e=>host.upload(e)} ?disabled=${host.busy} /></label><p class="muted">已上传 ${host.assets.length} 个 WAV 素材。选择播放或菜单节点可关联录音。</p></div>
        <div class="pane"><h3>路径模拟</h3><div class="two"><label>模拟按键<input .value=${host.testDigits} @input=${e=>host.testDigits=e.target.value} placeholder="如 1 或 2；留空模拟超时" /></label><label>工作时间<select .value=${host.testOpen?"open":"closed"} @change=${e=>host.testOpen=e.target.value==="open"}><option value="open">营业</option><option value="closed">非营业</option></select></label></div><button @click=${()=>host.runSimulation()}>运行模拟</button>${host.simulation?html`<div class="simulation">${host.simulation.path.map(x=>html`<span class="badge">${NODE_TYPES[x.type]} ${x.event}</span> → `)}<strong>${host.simulation.result}</strong></div>`:nothing}</div>
        <div class="pane"><h3>绑定与版本</h3><label>绑定到呼入队列<select .value=${host.bindingQueueId} @change=${e=>host.bindingQueueId=e.target.value}><option value="">选择队列</option>${host.queues.map(q=>html`<option value=${q.id}>${q.name}</option>`)}</select></label><div class="row"><button @click=${()=>host.bindQueue()} ?disabled=${host.busy||!host.selectedId}>保存队列绑定</button><button class="danger" @click=${()=>host.removeFlow()} ?disabled=${host.busy||!host.selectedId}>删除流程</button></div><div class="divider"></div>${host.versions.map(v=>html`<div class="version"><span>v${v.version}　${formatDateTime(v.published_at)}</span><button class="ghost" @click=${()=>host.rollback(v.version)}>回滚</button></div>`)}</div>
      </div>
    </div>`;
  }

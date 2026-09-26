// 本文件负责IVR 流程图画布。
import { svg, nothing } from "lit";
import { NODE_TYPES, layoutIVR } from "../ivr-model.js";

const nodeTitle = (id, node) => `${NODE_TYPES[node?.type] || "未知"} · ${node?.prompt || id}`;

export function renderCanvas(host) {
    const graph=layoutIVR(host.draft); const nodes=host.draft.nodes||{};
    if (host.pointerDrag?.kind==="move" && graph.positions[host.pointerDrag.id]) graph.positions[host.pointerDrag.id]=host.dragPoint;
    const source=host.pointerDrag?.kind==="link"?graph.positions[host.pointerDrag.id]:null;
    return svg`<svg viewBox=${`0 0 ${graph.width} ${graph.height}`} width=${graph.width} height=${graph.height} role="img" aria-label="IVR 流程图；拖动节点可调整位置，从节点下方圆点拖动到目标节点可连线">
      <defs><marker id="ivr-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0 0 L8 4 L0 8" fill="#8c8c8c" /></marker></defs>
      ${Object.keys(nodes).length?nothing:svg`<text x="25" y="55" fill="#737373">从左侧拖入第一个节点，建立流程入口。</text>`}
      ${graph.edges.map(e=>{ const a=graph.positions[e.from],b=graph.positions[e.to]; const mx=(a.x+b.x)/2,my=(a.y+b.y)/2; return svg`<path class="edge" marker-end="url(#ivr-arrow)" d=${`M ${a.x} ${a.y+32} C ${a.x} ${my} ${b.x} ${my} ${b.x} ${b.y-39}`} /><text class="edge-label" x=${mx} y=${my-4} text-anchor="middle">${e.label}</text>`; })}
      ${source&&host.dragPoint?svg`<path class="link-preview" d=${`M ${source.x} ${source.y+31} L ${host.dragPoint.x} ${host.dragPoint.y}`} />`:nothing}
      ${Object.entries(graph.positions).map(([id,p])=>svg`<g class=${`node ${id===host.selectedNode?"selected":""} ${host.pointerDrag?.kind==="move"&&host.pointerDrag.id===id?"dragging":""}`} role="button" tabindex="0" aria-label=${nodeTitle(id,nodes[id])} @pointerdown=${e=>host.startPointer(e,"move",id)} @click=${()=>{host.selectedNode=id; host.previewUrl=""}} @keydown=${e=>{if(e.key==="Enter"||e.key===" "){e.preventDefault();host.selectedNode=id}}}><rect x=${p.x-80} y=${p.y-31} width="160" height="62"></rect><text x=${p.x} y=${p.y-5} text-anchor="middle">${NODE_TYPES[nodes[id].type]}</text><text class="sub" x=${p.x} y=${p.y+15} text-anchor="middle">${(nodes[id].prompt||host.queues.find(q=>q.id===nodes[id].queue_id)?.name||id).slice(0,16)}</text>${["play","menu","time_check"].includes(nodes[id].type)?svg`<circle class="port" cx=${p.x} cy=${p.y+31} r="7" aria-label="拖动连线" @pointerdown=${e=>host.startPointer(e,"link",id)}></circle>`:nothing}</g>`)}
    </svg>`;
  }

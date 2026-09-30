// 本文件负责IVR 编辑器样式。
import { css } from "lit";
import { tokenStyles } from "../../../styles/tokens.js";

export const ivrEditorStyles = [tokenStyles, css`
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
    input:not([type="file"]),select,textarea { width:100%; min-height:36px; padding:4px 9px; border:1px solid var(--ov-border-strong); border-radius:var(--ov-radius-sm); background:white; color:var(--ov-text); transition:border-color .16s, box-shadow .16s; }
    select {
      appearance:none; -webkit-appearance:none; padding-right:32px; cursor:pointer;
      background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12' fill='none'%3E%3Cpath d='M3 4.5L6 7.5L9 4.5' stroke='%23687086' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
      background-repeat:no-repeat; background-position:right 10px center; background-size:12px;
    }
    input:not([type="file"]):hover:not(:disabled), select:hover:not(:disabled), textarea:hover:not(:disabled) { border-color:#c9c7f4; }
    textarea { min-height:88px; resize:vertical; }
    select:disabled, input:disabled, textarea:disabled { color:var(--ov-text-muted); background:var(--ov-surface-soft); cursor:not-allowed; opacity:.85; }
    .mini { width:80px; min-width:80px; flex:none; padding-left:8px; padding-right:28px; background-position:right 8px center; }
    .file { position:relative; display:inline-flex; align-items:center; gap:10px; max-width:100%; margin:4px 0; cursor:pointer; user-select:none; }
    .file input { position:absolute; inset:0; width:100%; height:100%; min-height:0; margin:0; padding:0; opacity:0; border:0; cursor:pointer; }
    .file-btn { flex:0 0 auto; display:inline-flex; align-items:center; justify-content:center; min-height:36px; height:36px; padding:0 14px; color:var(--ov-text-secondary); font-size:13px; font-weight:500; line-height:34px; background:#fff; border:1px solid var(--ov-border-strong); border-radius:var(--ov-radius-sm); transition:border-color .16s, color .16s, background .16s, box-shadow .16s; }
    .file:hover:not(.is-disabled) .file-btn { color:var(--ov-primary); background:var(--ov-primary-soft); border-color:#c9c7f4; }
    .file input:focus-visible + .file-btn { border-color:var(--ov-primary); box-shadow:var(--ov-focus); }
    .file-name { min-width:0; color:var(--ov-text-muted); font-size:13px; font-weight:400; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .file.has-file .file-name { color:var(--ov-text-secondary); }
    .file.is-disabled { cursor:not-allowed; opacity:.65; }
    .file.is-disabled input { cursor:not-allowed; }
    .two { display:grid; grid-template-columns:1fr 1fr; gap:8px; }
    .divider { border-top:1px solid #eee; margin:14px 0; }
    .details { margin-top:14px; display:grid; gap:10px; grid-template-columns:repeat(auto-fit,minmax(240px,1fr)); }
    .details .pane { height:fit-content; }
    .simulation { padding:8px 0; line-height:1.9; }
    .badge { color:var(--ov-primary); background:var(--ov-primary-soft); border-radius:999px; padding:2px 8px; margin-right:5px; display:inline-block; }
    audio { width:100%; max-width:300px; margin-top:8px; }
    .asset-list { max-height:200px; overflow-y:auto; margin-top:8px; border:1px solid #eee; border-radius:var(--ov-radius-sm); }
    .asset-row { display:flex; align-items:center; gap:8px; padding:6px 8px; border-bottom:1px solid #f0f0f0; }
    .asset-row:last-child { border-bottom:none; }
    .asset-name { flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-weight:400; }
    .version { display:flex; gap:8px; align-items:center; justify-content:space-between; padding:6px 0; border-bottom:1px solid #eee; }
    @media(max-width:1100px) { .workspace { grid-template-columns:180px 1fr; } .inspector { grid-column:1 / -1; } }
    @media(max-width:700px) { .workspace { grid-template-columns:1fr; } .inspector { grid-column:auto; } .wrap { padding:10px; } }
  `];

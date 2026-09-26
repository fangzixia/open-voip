export const NODE_TYPES = {
  play: "播放语音",
  menu: "按键菜单",
  time_check: "工作时间",
  route_queue: "转入队列",
  hangup: "结束通话",
};

/** 将不同类型节点的后续分支统一表示为标签与目标节点对。 */
export function outgoing(node) {
  if (!node) return [];
  switch (node.type) {
    case "play": return node.next ? [["下一步", node.next]] : [];
    case "menu": return [...Object.entries(node.choices || {}).map(([digit, target]) => [`按 ${digit}`, target]), ...(node.default ? [["超时", node.default]] : []), ...(node.invalid ? [["无效按键", node.invalid]] : [])];
    case "time_check": return [["营业", node.open], ["非营业", node.closed]].filter(([, target]) => target);
    default: return [];
  }
}

/** 校验节点配置、引用、循环和从起点不可达的节点。 */
export function validateIVR(doc, queues = [], assets = []) {
  const issues = [];
  const nodes = doc?.nodes || {};
  const ids = Object.keys(nodes);
  if (!doc?.start || !nodes[doc.start]) issues.push("请选择有效的开始节点");
  if (!ids.length) issues.push("至少添加一个节点");
  if (ids.length > 100) issues.push("节点不能超过 100 个");
  for (const [id, node] of Object.entries(nodes)) {
    if (!NODE_TYPES[node.type]) { issues.push(`${id}：节点类型无效`); continue; }
    if (["play", "menu"].includes(node.type)) {
      if (!node.file) issues.push(`${id}：请选择语音素材`);
      else if (!assets.some(asset => `${asset.id}.wav` === node.file)) issues.push(`${id}：语音素材不存在`);
      if (!(node.timeout_sec >= 1 && node.timeout_sec <= 120)) issues.push(`${id}：播放/等待时间须为 1–120 秒`);
    }
    if (node.type === "play" && !node.next) issues.push(`${id}：请选择下一节点`);
    if (node.type === "menu") {
      if (!Object.keys(node.choices || {}).length) issues.push(`${id}：至少配置一个按键`);
      if (!node.default) issues.push(`${id}：请选择超时去向`);
      if (!(node.max_retries >= 0 && node.max_retries <= 5)) issues.push(`${id}：无效按键重试次数须为 0–5`);
      for (const digit of Object.keys(node.choices || {})) if (!/^[0-9*#]$/.test(digit)) issues.push(`${id}：按键 ${digit} 无效`);
    }
    if (node.type === "time_check" && (!node.open || !node.closed)) issues.push(`${id}：请配置营业和非营业去向`);
    if (node.type === "route_queue" && !queues.some(q => q.id === node.queue_id)) issues.push(`${id}：目标队列不存在`);
    for (const [, target] of outgoing(node)) if (!nodes[target]) issues.push(`${id}：目标节点 ${target || "空"} 不存在`);
  }
  if (nodes[doc?.start]) {
    const visiting = new Set(), visited = new Set();
    function walk(id) {
      if (visiting.has(id)) { issues.push("流程中存在循环，可能导致来电无法结束"); return; }
      if (visited.has(id)) return;
      visiting.add(id); visited.add(id);
      for (const [, target] of outgoing(nodes[id])) if (nodes[target]) walk(target);
      visiting.delete(id);
    }
    walk(doc.start);
    for (const id of ids) if (!visited.has(id)) issues.push(`${id}：节点无法从开始节点到达`);
  }
  return [...new Set(issues)];
}

/** 按测试按键与营业状态模拟执行流程，最多推进 100 步。 */
export function simulateIVR(doc, { digits = "", open = true } = {}) {
  const path = [], nodes = doc?.nodes || {};
  let current = doc?.start, input = 0, invalidAttempts = 0;
  for (let step = 0; current && step < 100; step++) {
    const node = nodes[current];
    if (!node) return { path, result: `缺少节点 ${current}` };
    path.push({ id: current, type: node.type, event: "" });
    if (node.type === "route_queue") return { path, result: `转入队列 ${node.queue_id || "未选择"}` };
    if (node.type === "hangup") return { path, result: "结束通话" };
    if (node.type === "play") current = node.next;
    else if (node.type === "time_check") current = open ? node.open : node.closed;
    else if (node.type === "menu") {
      const digit = digits[input++];
      if (!digit) { path[path.length - 1].event = "超时"; current = node.default; }
      else if ((node.choices || {})[digit]) { path[path.length - 1].event = `按 ${digit}`; current = node.choices[digit]; invalidAttempts = 0; }
      else {
        invalidAttempts++;
        const limit = node.max_retries ?? 2;
        path[path.length - 1].event = `按 ${digit} 无效（${invalidAttempts}/${limit}）`;
        current = invalidAttempts >= limit ? node.invalid || node.default : current;
      }
    } else return { path, result: "未知节点类型" };
  }
  return { path, result: current ? "流程超过 100 步，可能存在循环" : "缺少结束节点" };
}

/** 按流程层级布置节点，并优先保留用户手动调整的位置。 */
export function layoutIVR(doc) {
  const nodes = doc?.nodes || {}, ids = Object.keys(nodes);
  const levels = new Map(), queue = doc?.start && nodes[doc.start] ? [doc.start] : [];
  if (queue.length) levels.set(queue[0], 0);
  while (queue.length) {
    const id = queue.shift();
    for (const [, next] of outgoing(nodes[id])) if (nodes[next] && !levels.has(next)) { levels.set(next, levels.get(id) + 1); queue.push(next); }
  }
  for (const id of ids) if (!levels.has(id)) levels.set(id, Math.max(0, ...levels.values()) + 1);
  const groups = new Map();
  for (const id of ids) { const level = levels.get(id); if (!groups.has(level)) groups.set(level, []); groups.get(level).push(id); }
  const max = Math.max(1, ...[...groups.values()].map(g => g.length));
  const width = Math.max(580, max * 210 + 40), positions = {};
  for (const [level, group] of groups) group.forEach((id, i) => { positions[id] = { x: (width - group.length * 210) / 2 + i * 210 + 105, y: 54 + level * 125 }; });
  for (const [id, point] of Object.entries(doc?.layout || {})) {
    if (positions[id] && Number.isFinite(point?.x) && Number.isFinite(point?.y)) {
      positions[id] = { x: Math.max(90, point.x), y: Math.max(45, point.y) };
    }
  }
  const finalWidth = Math.max(width, ...Object.values(positions).map(p => p.x + 100));
  const height = Math.max(450, (Math.max(0, ...groups.keys()) + 1) * 125 + 55, ...Object.values(positions).map(p => p.y + 65));
  return { width: finalWidth, height, positions, edges: ids.flatMap(id => outgoing(nodes[id]).filter(([, target]) => positions[target]).map(([label, target]) => ({ from: id, to: target, label }))) };
}

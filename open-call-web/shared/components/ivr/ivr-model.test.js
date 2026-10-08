// 本文件验证ivr model的关键行为。
import test from "node:test";
import assert from "node:assert/strict";
import { layoutIVR, simulateIVR, validateIVR } from "./ivr-model.js";

const queue = { id: "q1", name: "语音服务" };
const asset = { id: "11111111-1111-4111-8111-111111111111", name: "welcome.wav" };

test("generic input and business survey use explicit success and timeout paths", () => {
  const doc = { start: "input", nodes: {
    input: { type: "collect_input", result_key: "reference", accepted_digits: "09*#", timeout_sec: 8, next: "end", default: "end" },
    end: { type: "hangup" },
  } };
  assert.deepEqual(validateIVR(doc), []);
  for (const digits of ["0", "9", "*", "#", "5#", ""]) assert.equal(simulateIVR(doc, { digits }).result, "结束通话");
  assert.equal(simulateIVR(doc, { digits: "#" }).path[0].event, "按键 #");
  delete doc.nodes.input.result_key;
  assert.ok(validateIVR(doc).some(issue => issue.includes("结果标识")));
  doc.nodes.input = { type: "csat", timeout_sec: 8, next: "end", default: "end" };
  assert.deepEqual(validateIVR(doc), []);
  assert.equal(simulateIVR(doc, { digits: "95" }).path[1].event, "评分 5");
  delete doc.nodes.input.default;
  assert.ok(validateIVR(doc).some(issue => issue.includes("超时去向")));
});
const document = () => ({
  start: "menu",
  nodes: {
    menu: { type: "menu", file: `${asset.id}.wav`, timeout_sec: 8, max_retries: 2, choices: { "1": "queue" }, default: "end", invalid: "end" },
    queue: { type: "route_queue", queue_id: queue.id, session_type: "audio" },
    end: { type: "hangup" },
  },
});

test("valid menu branches simulate digit and timeout paths", () => {
  const doc = document();
  assert.deepEqual(validateIVR(doc, [queue], [asset]), []);
  assert.equal(simulateIVR(doc, { digits: "1" }).result, "转入队列 q1");
  assert.equal(simulateIVR(doc, { digits: "" }).result, "结束通话");
  assert.equal(simulateIVR(doc, { digits: "91" }).result, "转入队列 q1");
  assert.equal(simulateIVR(doc, { digits: "99" }).result, "结束通话");
});

test("dragged node layout is retained and graph edges still follow it", () => {
  const doc = document();
  doc.layout = { menu: { x: 940, y: 300 }, queue: { x: 400, y: 520 } };
  const graph = layoutIVR(doc);
  assert.deepEqual(graph.positions.menu, { x: 940, y: 300 });
  assert.deepEqual(graph.positions.queue, { x: 400, y: 520 });
  assert.ok(graph.width >= 1040 && graph.height >= 585);
  assert.ok(graph.edges.some(edge => edge.from === "menu" && edge.to === "queue" && edge.label === "按 1"));
});

test("invalid targets, missing audio and cycles block publishing", () => {
  const doc = document();
  doc.nodes.menu.file = "missing.wav";
  doc.nodes.menu.choices["1"] = "menu";
  const issues = validateIVR(doc, [queue], [asset]);
  assert.ok(issues.some(issue => issue.includes("语音素材不存在")));
  assert.ok(issues.some(issue => issue.includes("循环")));
  assert.ok(issues.some(issue => issue.includes("无法从开始节点到达")));
});

test("business decisions accept only declared outcomes and have an explicit timeout path", () => {
 const doc = {start:"check",nodes:{
 check:{type:"business_action",action:"customer.eligible",timeout_sec:5,choices:{yes:"queue"},default:"end"},
 queue:{type:"route_queue",queue_id:"q1"},end:{type:"hangup"}
 }};
 assert.deepEqual(validateIVR(doc,[queue]),[]);
 assert.equal(simulateIVR(doc,{outcomes:{"customer.eligible":"yes"}}).result,"转入队列 q1");
 assert.equal(simulateIVR(doc).result,"结束通话");
 assert.equal(simulateIVR(doc,{outcomes:{"customer.eligible":"unexpected"}}).result,"业务结果未在流程中声明");
 doc.nodes.check.timeout_sec=0;
 assert.ok(validateIVR(doc,[queue]).some(issue=>issue.includes("业务超时")));
});


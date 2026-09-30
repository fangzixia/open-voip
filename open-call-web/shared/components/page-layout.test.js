import test from "node:test";
import assert from "node:assert/strict";
import { renderCrudSection, renderPage, renderPageCrud, renderPageDashboard, renderPageSplit } from "./page-layout.js";

test("page templates return lit templates", () => {
  assert.equal(typeof renderPage({ description: "说明", children: "x" }), "object");
  assert.equal(typeof renderCrudSection({ title: "区块", hint: "提示", form: "f", list: "l" }), "object");
  assert.equal(typeof renderPageCrud({ description: "d", sections: [{ title: "A", list: "l" }] }), "object");
  assert.equal(typeof renderPageSplit({ description: "d", master: "m", detail: "d2" }), "object");
  assert.equal(typeof renderPageDashboard({
    description: "d",
    kpis: [{ label: "在线", value: 1 }],
    cards: ["c"],
  }), "object");
});

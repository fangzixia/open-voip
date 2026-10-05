import test from "node:test";
import assert from "node:assert/strict";
import { renderCrudSection, renderPage, renderPageCrud, renderPageDashboard, renderPageSplit } from "./page-layout.js";

test("page templates return lit templates", () => {
  assert.equal(typeof renderPage({ children: "x" }), "object");
  assert.equal(typeof renderCrudSection({ title: "区块", hint: "提示", form: "f", list: "l" }), "object");
  assert.equal(typeof renderPageCrud({ sections: [{ title: "A", list: "l" }] }), "object");
  assert.equal(typeof renderPageSplit({ master: "m", detail: "d2" }), "object");
  assert.equal(typeof renderPageDashboard({
    kpis: [{ label: "在线", value: 1 }],
    cards: ["c"],
  }), "object");
});

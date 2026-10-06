// 管理端应用入口：身份、路由、队列/IVR/DID 等配置与发布。
import { NAV, renderApp } from "./views/shell.js";
import { identityActions, newIdentityUserDraft } from "./controllers/identity.js";
import { bindApiFeedback } from "../shared/http-client.js";
import { FeedbackController, runFeedbackAction } from "../shared/feedback.js";
import { formatDateTime } from "../shared/datetime.js";
import { LitElement } from "lit";
import { addQaMark, authMe, authOptions, bindQueueAgents, bindAgentSkills, createBridge, createQueue, createSkill, createWebhook, downloadCdrCsv, downloadRecording, endBridge, exchangeSSOTicket, fetchRecordingBlob, fetchAgentUtil, fetchHistoricalReport, fetchLiveReport, fetchStatus, forceCheckout, getCall, listAgents, listAudit, listCdr, listDids, listGroupMappings, listIvr, listOpenCalls, listPermissions, listQueues, listRoles, listSkills, listUsers, listWebhooks, listWrapUps, login, logout, patchQueue, popSSOTicket, replaceBridge, startSSO, upsertDid } from "../shared/api.js";
import { clearAccessToken, getAccessToken, setAuthTokens } from "../shared/auth-store.js";
import { canOpenAdminPage, firstAdminPage } from "../shared/workspace-permissions.js";
import { isAdminNav } from "../shared/staff-nav.js";
import { appStyles } from "../shared/styles/index.js";
import "../shared/components/ivr/ivr-editor.js";

const CDR_PAGE_SIZE = 50;

/** 按导航页懒加载的数据作业：[hostKey, loader, field|null, permission] */
const NAV_LOAD_JOBS = {
  overview: [
    ["status", fetchStatus, null, "status.read"],
    ["queues", listQueues, "items", "queues.read"],
    ["agents", listAgents, "items", "agents.read"],
    ["live", fetchLiveReport, null, "reports.read"],
    ["hist", fetchHistoricalReport, null, "reports.read"],
  ],
  queues: [
    ["queues", listQueues, "items", "queues.read"],
    ["agents", listAgents, "items", "agents.read"],
    ["skills", listSkills, null, "skills.read"],
    ["users", listUsers, "items", "users.read"],
  ],
  agents: [
    ["agents", listAgents, "items", "agents.read"],
    ["hist", fetchHistoricalReport, null, "reports.read"],
    ["utils", fetchAgentUtil, "items", "reports.read"],
  ],
  dids: [
    ["dids", listDids, null, "dids.read"],
    ["queues", listQueues, "items", "queues.read"],
    ["ivrs", listIvr, null, "ivr.read"],
  ],
  cdr: [
    ["wrapUps", listWrapUps, "items", "cdr.read"],
  ],
  ivr: [
    ["ivrs", listIvr, null, "ivr.read"],
    ["queues", listQueues, "items", "queues.read"],
  ],
  webhooks: [["hooks", listWebhooks, null, "webhooks.read"]],
  audit: [["audit", listAudit, "items", "audit.read"]],
  identity: [
    ["users", listUsers, "items", "users.read"],
    ["roles", listRoles, "items", "roles.read"],
    ["permissionsCatalog", listPermissions, "items", "roles.read"],
    ["groupMappings", listGroupMappings, "items", "identity.read"],
  ],
};

function blankQueueForm(video = false) {
  return {
    name: "",
    video_enabled: !!video,
    max_wait_sec: 300,
    strategy: "longest_idle",
    overflow_policy: "hangup",
    overflow_queue_id: "",
    recording_policy: video ? "video_composite" : "audio",
    announce_recording: true,
    wait_prompt: "您前面还有 {position} 位，请稍候",
    priority_enabled: false,
    listen_announce: false,
  };
}

export class AdminApp extends LitElement {
  static properties = {
    error: { type: String },
    notice: { type: String },
    username: { type: String },
    password: { type: String },
    authed: { type: Boolean },
    status: { type: Object },
    users: { type: Array },
    queues: { type: Array },
    cdr: { type: Array },
    live: { type: Object },
    hist: { type: Object },
    recs: { type: Array },
    audit: { type: Array },
    ivrs: { type: Array },
    hooks: { type: Array },
    hookUrl: { type: String },
    newVoiceQueue: { type: Object },
    newVideoQueue: { type: Object },
    agents: { type: Array },
    utils: { type: Array },
    dids: { type: Array },
    didForm: { type: Object },
    qaCallId: { type: String },
    qaLabel: { type: String },
    forceAgentId: { type: String },
    skills: { type: Array },
    skillName: { type: String },
    wrapUps: { type: Array },
    playUrl: { type: String },
    playType: { type: String },
    bindAgentId: { type: String },
    bindSkillId: { type: String },
    nav: { type: String },
    cdrCaller: { type: String },
    cdrResult: { type: String },
    cdrPage: { type: Number },
    cdrTotal: { type: Number },
    clock: { type: String },
    authOptions: { type: Object }, me: { type: Object }, roles: { type: Array }, permissionsCatalog: { type: Array }, groupMappings: { type: Array }, selectedUser: { type: String }, identities: { type: Array }, newRole: { type: Object }, newMapping: { type: Object }, identityInput: { type: Object }, agentProfileDraft: { type: Object }, newIdentityUser: { type: Object },
    openCalls: { type: Array }, runtimeCall: { type: Object }, bridgeForm: { type: Object },
    embedded: { type: Boolean },
    workspaceActive: { type: Boolean },
    contentOnly: { type: Boolean, attribute: "content-only" },
    staffNav: { type: String, attribute: "staff-nav" },
  };

  static styles = appStyles;

  #clockTimer = null;
  feedback = new FeedbackController(this);

  constructor() {
    super();
    this.error = ""; this.notice="";
    this.username = "";
    this.password = "";
    this.authed = !!getAccessToken();
    this.status = null;
    this.users = [];
    this.queues = [];
    this.cdr = [];
    this.live = null;
    this.hist = null;
    this.recs = [];
    this.audit = [];
    this.ivrs = [];
    this.hooks = [];
    this.hookUrl = "https://crm.internal/webhook";
    this.newVoiceQueue = blankQueueForm(false);
    this.newVideoQueue = blankQueueForm(true);
    this.agents = [];
    this.utils = [];
    this.dids = [];
    this.didForm = { trunk_id: "*", did: "", target_type: "queue", target_id: "" };
    this.qaCallId = "";
    this.qaLabel = "";
    this.forceAgentId = "";
    this.skills = [];
    this.skillName = "通用";
    this.wrapUps = [];
    this.playUrl = "";
    this.playType = "";
    this.bindAgentId = "";
    this.bindSkillId = "";
    this.nav = "overview";
    this.cdrCaller = "";
    this.cdrResult = "";
    this.cdrPage = 1;
    this.cdrTotal = 0;
    this.cdrPageSize = CDR_PAGE_SIZE;
    this.clock = "";
    this.authOptions = null; this.me = null; this.roles = []; this.permissionsCatalog = []; this.groupMappings = []; this.selectedUser = ""; this.identities = []; this.newRole = { id: "", name: "", permissions: [] }; this.newMapping = { group: "", roles: [] }; this.identityInput = { issuer: "", subject: "" }; this.agentProfileDraft = { extension: "", terminal_type: "webrtc", video_capable: false };     this.newIdentityUser = newIdentityUserDraft();
    this.openCalls = [];
    this.runtimeCall = null;
    this.bridgeForm = { bridge_id: "", leg_a: "", leg_b: "" };
    this.embedded = false;
    this.workspaceActive = false;
    this.contentOnly = false;
    this.staffNav = "";
  }

  #run(fn) {
    return runFeedbackAction(this.feedback, fn);
  }

  connectedCallback() {
    super.connectedCallback();
    this._unbindApi = bindApiFeedback(this, () => {
      this.authed = false;
      if (this.embedded) {
        this.dispatchEvent(new CustomEvent("session-ended", { bubbles: true, composed: true }));
      }
    });
    this.#tickClock();
    this.#clockTimer = setInterval(() => this.#tickClock(), 1000);
    if (this.embedded) {
      if (getAccessToken()) void this.#load();
      return;
    }
    authOptions().then((v) => { this.authOptions = v || {}; }).catch((e) => {
      this.authOptions = { unavailable: true };
      this.feedback.liveError(e instanceof Error ? e.message : "无法读取登录方式");
    });
    const ticket = popSSOTicket();
    if (ticket) this.#completeSSO(ticket);
    else if (this.authed) this.#load();
  }

  updated(changed) {
    if (changed.has("workspaceActive") && this.workspaceActive) {
      this.feedback.activate();
    }
    if (this.contentOnly && changed.has("staffNav") && isAdminNav(this.staffNav) && this.staffNav !== this.nav) {
      if (this.feedback) this.feedback.begin();
      this.nav = this.staffNav;
      void this.#loadNav(this.staffNav);
    }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unbindApi?.();
    clearInterval(this.#clockTimer);
  }

  #tickClock() {
    this.clock = formatDateTime();
  }

  async #login(ev) {
    ev.preventDefault();
    await this.#run(async () => {
      const tokens = await login(this.username, this.password);
      setAuthTokens(tokens, { persist: true });
      this.authed = true;
      await this.#load();
    });
  }

  async #completeSSO(ticket) {
    await this.#run(async () => {
      const tokens = await exchangeSSOTicket(ticket);
      setAuthTokens(tokens, { persist: true });
      this.authed = true;
      await this.#load();
    });
  }

  async #logout() {
    await this.#run(async () => {
      try { await logout(); } finally {
        clearAccessToken();
        this.authed = false;
        this.dispatchEvent(new CustomEvent("session-ended", { bubbles: true, composed: true }));
      }
    });
  }

  /** 鉴权后选择可访问导航，并按当前页懒加载。 */
  async #load() {
    this.me = await authMe();
    const perms = this.me?.permissions || [];
    if (this.nav === "recordings") this.nav = "cdr";
    if (!canOpenAdminPage(perms, this.nav)) this.nav = firstAdminPage(perms) || "identity";
    await this.#loadNav(this.nav);
  }

  /** 按导航页加载所需数据。 */
  async #loadNav(nav = this.nav) {
    if (!this.authed && !getAccessToken()) return;
    if (nav === "runtime") {
      await this.#loadRuntime();
      return;
    }
    const allow = (code) => this.me?.permissions?.includes(code);
    const jobs = NAV_LOAD_JOBS[nav] || [];
    const tasks = jobs.filter(([, , , code]) => !code || allow(code)).map(async ([key, load, field]) => {
      const result = await load();
      if (!this.authed && !getAccessToken()) return;
      this[key] = field ? result?.[field] || [] : result;
    });
    if ((nav === "overview" || nav === "cdr") && allow("cdr.read")) tasks.push(this.#loadCdr());
    await Promise.allSettled(tasks);
  }

  /** 按当前筛选条件和页码从服务端拉取话单。 */
  async #loadCdr(page = this.cdrPage) {
    const result = await listCdr({ page, pageSize: CDR_PAGE_SIZE, caller: this.cdrCaller.trim(), result: this.cdrResult });
    if (!this.authed && !getAccessToken()) return;
    this.cdr = result?.items || [];
    this.cdrTotal = result?.total || 0;
    this.cdrPage = result?.page || page;
  }

  async #createQueue(ev, video = false) {
    ev.preventDefault();
    const key = video ? "newVideoQueue" : "newVoiceQueue";
    await this.#run(async () => {
      await createQueue({ ...this[key], video_enabled: !!video });
      this[key] = blankQueueForm(!!video);
      await this.#loadNav();
    });
  }

  async #bindAll(queueId) {
    const ids = this.users.filter((u) => u.agent_id).map((u) => u.agent_id);
    await this.#run(async () => {
      await bindQueueAgents(queueId, ids);
      await this.#loadNav();
    });
  }

  async #addHook() {
    await this.#run(async () => {
      await createWebhook(this.hookUrl, ["*"]);
      await this.#loadNav();
    });
  }

  async #addSkill() {
    await this.#run(async () => {
      await createSkill(this.skillName || "通用");
      await this.#loadNav();
    });
  }

  async #bindSkill() {
    if (!this.bindAgentId || !this.bindSkillId) return;
    await this.#run(async () => {
      await bindAgentSkills(this.bindAgentId, [this.bindSkillId]);
      await this.#loadNav();
    });
  }

  async #playRec(id) {
    await this.#run(async () => {
      const blob = await fetchRecordingBlob(id);
      if (this.playUrl) URL.revokeObjectURL(this.playUrl);
      this.playUrl = URL.createObjectURL(blob);
      this.playType = blob.type;
    });
  }

  async #downloadRec(id, format = "") {
    await this.#run(async () => {
      await downloadRecording(id, format);
    });
  }

  async #saveDid(ev) {
    ev.preventDefault();
    await this.#run(async () => {
      await upsertDid(this.didForm);
      await this.#loadNav();
    });
  }

  async #qa(ev) {
    ev.preventDefault();
    await this.#run(async () => {
      await addQaMark(this.qaCallId, 0, this.qaLabel);
      this.qaLabel = "";
    });
  }

  async #force() {
    await this.#run(async () => {
      await forceCheckout(this.forceAgentId);
      await this.#loadNav();
    });
  }

  async #toggleVip(q) {
    await this.#run(async () => {
      await patchQueue(q.id, { ...q, priority_enabled: !q.priority_enabled });
      await this.#loadNav();
    });
  }

  /** 当前页话单；筛选已在服务端完成。 */
  #filteredCdr() {
    return this.cdr || [];
  }

  #waiting() {
    return (this.live?.queues || []).reduce((n, q) => n + (q.waiting || 0), 0);
  }

  #crumb() {
    return NAV.find((n) => n.id === this.nav)?.label || "总览";
  }

  #idleAgents() {
    return (this.agents || []).filter((a) => a.state === "idle").length;
  }

  async #loadRuntime() {
    await this.#run(async () => {
      this.openCalls = await listOpenCalls();
    });
  }

  async #selectRuntimeCall(callId) {
    await this.#run(async () => {
      this.runtimeCall = await getCall(callId);
      this.bridgeForm = { bridge_id: "", leg_a: "", leg_b: "" };
    });
  }

  async #createBridge() {
    const { leg_a: a, leg_b: b } = this.bridgeForm;
    if (!this.runtimeCall?.id || !a || !b) return;
    await this.#run(async (epoch) => {
      await createBridge(this.runtimeCall.id, a, b);
      this.feedback.ok("已请求建立桥接", epoch);
      this.runtimeCall = await getCall(this.runtimeCall.id);
      this.bridgeForm = { bridge_id: "", leg_a: "", leg_b: "" };
    });
  }

  async #replaceBridge() {
    const { bridge_id, leg_a: a, leg_b: b } = this.bridgeForm;
    if (!this.runtimeCall?.id || !bridge_id || !a || !b) return;
    await this.#run(async (epoch) => {
      await replaceBridge(this.runtimeCall.id, bridge_id, a, b);
      this.feedback.ok("桥接腿已替换", epoch);
      this.runtimeCall = await getCall(this.runtimeCall.id);
      this.bridgeForm = { bridge_id: "", leg_a: "", leg_b: "" };
    });
  }

  async #endBridge() {
    const { bridge_id } = this.bridgeForm;
    if (!this.runtimeCall?.id || !bridge_id) return;
    await this.#run(async (epoch) => {
      await endBridge(this.runtimeCall.id, bridge_id);
      this.feedback.ok("桥接已拆除", epoch);
    });
  }

  render() {
    return renderApp(this, {
      addHook: (...args) => this.#addHook(...args),
      addSkill: (...args) => this.#addSkill(...args),
      bindAll: (...args) => this.#bindAll(...args),
      bindSkill: (...args) => this.#bindSkill(...args),
      createBridge: (...args) => this.#createBridge(...args),
      createQueue: (...args) => this.#createQueue(...args),
      endBridge: (...args) => this.#endBridge(...args),
      loadRuntime: (...args) => this.#loadRuntime(...args),
      replaceBridge: (...args) => this.#replaceBridge(...args),
      selectRuntimeCall: (...args) => this.#selectRuntimeCall(...args),
      crumb: (...args) => this.#crumb(...args),
      downloadRec: (...args) => this.#downloadRec(...args),
      exportCdr: () => downloadCdrCsv(),
      filteredCdr: (...args) => this.#filteredCdr(...args),
      loadCdr: (page) => this.#run(() => this.#loadCdr(page)),
      force: (...args) => this.#force(...args),
      idleAgents: (...args) => this.#idleAgents(...args),
      load: (...args) => this.#load(...args),
      loadNav: (...args) => this.#loadNav(...args),
      login: (...args) => this.#login(...args),
      startSSO: () => startSSO(),
      ...identityActions(this, () => this.#loadNav("identity")),
      logout: (...args) => this.#logout(...args),
      playRec: (...args) => this.#playRec(...args),
      qa: (...args) => this.#qa(...args),
      saveDid: (...args) => this.#saveDid(...args),
      toggleVip: (...args) => this.#toggleVip(...args),
      waiting: (...args) => this.#waiting(...args)
    });
  }
}

customElements.define("open-voip-admin-app", AdminApp);

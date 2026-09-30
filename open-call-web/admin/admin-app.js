// 管理端应用入口：身份、路由、队列/IVR/DID 等配置与发布。
import { NAV, renderApp } from "./views/shell.js";
import { identityActions, newIdentityUserDraft } from "./controllers/identity.js";
import { bindApiFeedback } from "../shared/http-client.js";
import { FeedbackController, runFeedbackAction } from "../shared/feedback.js";
import { formatDateTime } from "../shared/datetime.js";
import { LitElement } from "lit";
import { addQaMark, authMe, authOptions, bindQueueAgents, bindAgentSkills, createBridge, createQueue, createSkill, createWebhook, downloadCdrCsv, downloadRecording, endBridge, exchangeSSOTicket, fetchRecordingBlob, fetchAgentUtil, fetchHistoricalReport, fetchLiveReport, fetchStatus, forceCheckout, getCall, listAgents, listAudit, listCdr, listDids, listGroupMappings, listIvr, listOpenCalls, listPermissions, listQueues, listRecordings, listRoles, listSkills, listUsers, listWebhooks, listWrapUps, login, logout, patchQueue, popSSOTicket, replaceBridge, startSSO, upsertDid } from "../shared/api.js";
import { clearAccessToken, getAccessToken, setAuthTokens } from "../shared/auth-store.js";
import { appStyles } from "../shared/styles/index.js";
import "../shared/components/ivr/ivr-editor.js";

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
    clock: { type: String },
    authOptions: { type: Object }, me: { type: Object }, roles: { type: Array }, permissionsCatalog: { type: Array }, groupMappings: { type: Array }, selectedUser: { type: String }, identities: { type: Array }, newRole: { type: Object }, newMapping: { type: Object }, identityInput: { type: Object }, agentProfileDraft: { type: Object }, newIdentityUser: { type: Object },
    openCalls: { type: Array }, runtimeCall: { type: Object }, bridgeForm: { type: Object },
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
    this.clock = "";
    this.authOptions = null; this.me = null; this.roles = []; this.permissionsCatalog = []; this.groupMappings = []; this.selectedUser = ""; this.identities = []; this.newRole = { id: "", name: "", permissions: [] }; this.newMapping = { group: "", roles: [] }; this.identityInput = { issuer: "", subject: "" }; this.agentProfileDraft = { extension: "", terminal_type: "webrtc", video_capable: false };     this.newIdentityUser = newIdentityUserDraft();
    this.openCalls = [];
    this.runtimeCall = null;
    this.bridgeForm = { bridge_id: "", leg_a: "", leg_b: "" };
  }

  #run(fn) {
    return runFeedbackAction(this.feedback, fn);
  }

  connectedCallback() {
    super.connectedCallback();
    this._unbindApi = bindApiFeedback(this, () => { this.authed = false; });
    this.#tickClock();
    this.#clockTimer = setInterval(() => this.#tickClock(), 1000);
    authOptions().then((v) => { this.authOptions = v || {}; }).catch((e) => {
      this.authOptions = { unavailable: true };
      this.feedback.liveError(e instanceof Error ? e.message : "无法读取登录方式");
    });
    const ticket = popSSOTicket();
    if (ticket) this.#completeSSO(ticket);
    else if (this.authed) this.#load();
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

  /** 加载管理端总览所需的队列、坐席、话单和配置数据。 */
  async #load() {
    this.me = await authMe();
    const allow = (code) => this.me?.permissions?.includes(code);
    const navPermission = { overview: "status.read", runtime: "calls.read", queues: "queues.read", agents: "agents.read", dids: "dids.read", cdr: "cdr.read", recordings: "recordings.read", ivr: "ivr.read", webhooks: "webhooks.read", audit: "audit.read", identity: "users.read" };
    const canOpen = (key) => key === "identity" ? ["users.read", "users.create", "roles.read", "identity.read"].some(allow) : allow(navPermission[key]);
    if (!canOpen(this.nav)) this.nav = Object.keys(navPermission).find(canOpen) || "identity";
    const jobs = [
      ["status", fetchStatus, null, "status.read"], ["users", listUsers, "items", "users.read"], ["queues", listQueues, "items", "queues.read"],
      ["cdr", listCdr, "items", "cdr.read"], ["live", fetchLiveReport, null, "reports.read"], ["hist", fetchHistoricalReport, null, "reports.read"],
      ["recs", listRecordings, "items", "recordings.read"], ["audit", listAudit, "items", "audit.read"], ["ivrs", listIvr, null, "ivr.read"],
      ["hooks", listWebhooks, null, "webhooks.read"], ["agents", listAgents, "items", "agents.read"], ["utils", fetchAgentUtil, "items", "reports.read"],
      ["dids", listDids, null, "dids.read"], ["skills", listSkills, null, "skills.read"], ["wrapUps", listWrapUps, "items", "cdr.read"],
      ["roles", listRoles, "items", "roles.read"], ["permissionsCatalog", listPermissions, "items", "roles.read"], ["groupMappings", listGroupMappings, "items", "identity.read"],
    ];
    await Promise.allSettled(jobs.filter(([, , , code]) => allow(code)).map(async ([key, load, field]) => {
      const result = await load();
      if (!this.authed) return;
      this[key] = field ? result?.[field] || [] : result;
    }));
  }

  async #createQueue(ev, video = false) {
    ev.preventDefault();
    const key = video ? "newVideoQueue" : "newVoiceQueue";
    await this.#run(async () => {
      await createQueue({ ...this[key], video_enabled: !!video });
      this[key] = blankQueueForm(!!video);
      await this.#load();
    });
  }

  async #bindAll(queueId) {
    const ids = this.users.filter((u) => u.agent_id).map((u) => u.agent_id);
    await this.#run(async () => {
      await bindQueueAgents(queueId, ids);
      await this.#load();
    });
  }

  async #addHook() {
    await this.#run(async () => {
      await createWebhook(this.hookUrl, ["*"]);
      await this.#load();
    });
  }

  async #addSkill() {
    await this.#run(async () => {
      await createSkill(this.skillName || "通用");
      await this.#load();
    });
  }

  async #bindSkill() {
    if (!this.bindAgentId || !this.bindSkillId) return;
    await this.#run(async () => {
      await bindAgentSkills(this.bindAgentId, [this.bindSkillId]);
      await this.#load();
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
      await this.#load();
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
      await this.#load();
    });
  }

  async #toggleVip(q) {
    await this.#run(async () => {
      await patchQueue(q.id, { ...q, priority_enabled: !q.priority_enabled });
      await this.#load();
    });
  }

  /** 根据当前筛选条件生成页面显示的话单集合。 */
  #filteredCdr() {
    return (this.cdr || []).filter((c) => {
      const callerOk = !this.cdrCaller || String(c.caller || "").includes(this.cdrCaller);
      const resultOk = !this.cdrResult || String(c.result || "") === this.cdrResult;
      return callerOk && resultOk;
    });
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
      force: (...args) => this.#force(...args),
      idleAgents: (...args) => this.#idleAgents(...args),
      load: (...args) => this.#load(...args),
      login: (...args) => this.#login(...args),
      startSSO: () => startSSO(),
      ...identityActions(this, () => this.#load()),
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

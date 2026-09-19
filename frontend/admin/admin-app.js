import { LitElement, html } from "lit";
import {
  addQaMark,
  bindQueueAgents,
  bindAgentSkills,
  createIvrFlow,
  createQueue,
  createSkill,
  createUser,
  createWebhook,
  downloadCdrCsv,
  downloadRecording,
  fetchRecordingBlob,
  fetchAgentUtil,
  fetchHistoricalReport,
  fetchLiveReport,
  fetchStatus,
  forceCheckout,
  listAgents,
  listAudit,
  listCdr,
  listDids,
  listIvr,
  listQueues,
  listRecordings,
  listSkills,
  listUsers,
  listWebhooks,
  listWrapUps,
  login,
  patchQueue,
  publishIvr,
  upsertDid,
} from "../shared/api.js";
import { clearAccessToken, getAccessToken, setAccessToken } from "../shared/auth-store.js";
import { shellStyles } from "../shared/shell-styles.js";

const NAV = [
  { id: "overview", label: "总览" },
  { id: "queues", label: "队列" },
  { id: "agents", label: "坐席" },
  { id: "dids", label: "DID" },
  { id: "cdr", label: "CDR" },
  { id: "recordings", label: "录音" },
  { id: "ivr", label: "IVR" },
  { id: "webhooks", label: "Webhook" },
  { id: "audit", label: "审计" },
];

function agentTag(state) {
  if (state === "idle") return "ok";
  if (state === "on_call" || state === "ringing") return "info";
  if (state === "busy" || state === "acw") return "warn";
  return "muted";
}

function agentLabel(state) {
  return { idle: "空闲", busy: "示忙", ringing: "振铃", on_call: "通话中", acw: "事后处理", offline: "离线" }[state] || state || "离线";
}

export class AdminApp extends LitElement {
  static properties = {
    error: { type: String },
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
    newUser: { type: Object },
    newQueue: { type: Object },
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
    bindAgentId: { type: String },
    bindSkillId: { type: String },
    nav: { type: String },
    cdrCaller: { type: String },
    cdrResult: { type: String },
    clock: { type: String },
  };

  static styles = shellStyles;

  #clockTimer = null;

  constructor() {
    super();
    this.error = "";
    this.username = "admin";
    this.password = "changeme";
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
    this.newUser = { username: "", password: "changeme", role: "agent", extension: "", video_capable: true, display_name: "" };
    this.newQueue = {
      name: "",
      video_enabled: false,
      max_wait_sec: 300,
      strategy: "longest_idle",
      overflow_policy: "hangup",
      overflow_queue_id: "",
      recording_policy: "audio",
      announce_recording: true,
      wait_prompt: "您前面还有 {position} 位，请稍候",
      priority_enabled: false,
      listen_announce: false,
    };
    this.agents = [];
    this.utils = [];
    this.dids = [];
    this.didForm = { did: "", queue_id: "", display_name: "" };
    this.qaCallId = "";
    this.qaLabel = "";
    this.forceAgentId = "";
    this.skills = [];
    this.skillName = "通用";
    this.wrapUps = [];
    this.playUrl = "";
    this.bindAgentId = "";
    this.bindSkillId = "";
    this.nav = "overview";
    this.cdrCaller = "";
    this.cdrResult = "";
    this.clock = "";
  }

  connectedCallback() {
    super.connectedCallback();
    this.#tickClock();
    this.#clockTimer = setInterval(() => this.#tickClock(), 1000);
    if (this.authed) this.#load();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    clearInterval(this.#clockTimer);
  }

  #tickClock() {
    this.clock = new Date().toLocaleTimeString("zh-CN", { hour12: false });
  }

  async #login(ev) {
    ev.preventDefault();
    try {
      const tokens = await login(this.username, this.password);
      setAccessToken(tokens.access_token, { persist: true });
      this.authed = true;
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #load() {
    try {
      this.status = await fetchStatus();
      this.users = (await listUsers()).items || [];
      this.queues = (await listQueues()).items || [];
      this.cdr = (await listCdr()).items || [];
      this.live = await fetchLiveReport();
      this.hist = await fetchHistoricalReport();
      this.recs = (await listRecordings()).items || [];
      this.audit = (await listAudit()).items || [];
      this.ivrs = (await listIvr()) || [];
      this.hooks = (await listWebhooks()) || [];
      this.agents = (await listAgents()).items || [];
      this.utils = (await fetchAgentUtil()).items || [];
      const dids = await listDids();
      this.dids = Array.isArray(dids) ? dids : [];
      const skills = await listSkills();
      this.skills = Array.isArray(skills) ? skills : skills.items || [];
      this.wrapUps = (await listWrapUps()).items || [];
      this.error = "";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #createUser(ev) {
    ev.preventDefault();
    try {
      await createUser(this.newUser);
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #createQueue(ev) {
    ev.preventDefault();
    try {
      await createQueue(this.newQueue);
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #bindAll(queueId) {
    const ids = this.users.filter((u) => u.agent_id).map((u) => u.agent_id);
    try {
      await bindQueueAgents(queueId, ids);
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #ivrDemo() {
    try {
      const audio = this.queues.find((q) => !q.video_enabled);
      const video = this.queues.find((q) => q.video_enabled);
      const flow = await createIvrFlow("主菜单", {
        start: "menu",
        nodes: {
          menu: {
            type: "menu",
            prompt: "按1语音，按2视频",
            timeout_sec: 8,
            choices: { 1: "qa", 2: "qv" },
            default: "qa",
          },
          qa: { type: "route_queue", queue_id: audio?.id, session_type: "audio" },
          qv: { type: "route_queue", queue_id: video?.id || audio?.id, session_type: "video" },
        },
      });
      await publishIvr(flow.id);
      if (audio) {
        await patchQueue(audio.id, {
          name: audio.name,
          video_enabled: audio.video_enabled,
          max_wait_sec: audio.max_wait_sec,
          strategy: audio.strategy,
          ivr_flow_id: flow.id,
          overflow_policy: audio.overflow_policy || "hangup",
          recording_policy: audio.recording_policy || "audio",
          announce_recording: audio.announce_recording,
        });
      }
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #addHook() {
    try {
      await createWebhook(this.hookUrl, ["*"]);
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #addSkill() {
    try {
      await createSkill(this.skillName || "通用");
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #bindSkill() {
    if (!this.bindAgentId || !this.bindSkillId) return;
    try {
      await bindAgentSkills(this.bindAgentId, [this.bindSkillId]);
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #playRec(id) {
    try {
      const blob = await fetchRecordingBlob(id);
      if (this.playUrl) URL.revokeObjectURL(this.playUrl);
      this.playUrl = URL.createObjectURL(blob);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #saveDid(ev) {
    ev.preventDefault();
    try {
      await upsertDid(this.didForm);
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #qa(ev) {
    ev.preventDefault();
    try {
      await addQaMark(this.qaCallId, 0, this.qaLabel);
      this.error = "";
      this.qaLabel = "";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #force() {
    try {
      await forceCheckout(this.forceAgentId);
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #toggleVip(q) {
    try {
      await patchQueue(q.id, { ...q, priority_enabled: !q.priority_enabled });
      await this.#load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

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

  render() {
    if (!this.authed) {
      return html`
        <div class="login-page">
          <header class="topbar">
            <div class="brand"><span class="brand-mark">OV</span><span>Open VoIP</span><span class="brand-sub">管理控台</span></div>
          </header>
          <div class="login-wrap">
            <form class="login-card" @submit=${(e) => this.#login(e)}>
              <h2>管理员登录</h2>
              <p class="hint">演示账号 admin / changeme</p>
              <div class="field">
                <label>用户名</label>
                <input .value=${this.username} @input=${(e) => (this.username = e.target.value)} />
              </div>
              <div class="field">
                <label>密码</label>
                <input type="password" .value=${this.password} @input=${(e) => (this.password = e.target.value)} />
              </div>
              <button type="submit">登录</button>
              ${this.error ? html`<p class="error" style="margin-top:12px">${this.error}</p>` : ""}
            </form>
          </div>
        </div>
      `;
    }
    return html`
      <div class="layout">
        <header class="topbar">
          <div class="brand">
            <span class="brand-mark">OV</span>
            <span>Open VoIP</span>
            <span class="brand-sub">管理控台</span>
          </div>
          <span class="spacer"></span>
          <span class="topbar-meta">${this.clock}</span>
          <span class="topbar-meta">管理员</span>
          <button @click=${() => { clearAccessToken(); this.authed = false; }}>退出</button>
        </header>
        <div class="layout-body">
          <aside class="sidebar">
            ${NAV.map(
              (n) => html`<button class="nav-item ${this.nav === n.id ? "active" : ""}" @click=${() => (this.nav = n.id)}>${n.label}</button>`,
            )}
          </aside>
          <div class="content">
            <div class="breadcrumb">管理控台 / <strong>${this.#crumb()}</strong></div>
            ${this.error ? html`<p class="error">${this.error}</p>` : ""}
            ${this.nav === "overview" ? this.#overview() : ""}
            ${this.nav === "queues" ? this.#queues() : ""}
            ${this.nav === "agents" ? this.#agents() : ""}
            ${this.nav === "dids" ? this.#dids() : ""}
            ${this.nav === "cdr" ? this.#cdrPage() : ""}
            ${this.nav === "recordings" ? this.#recs() : ""}
            ${this.nav === "ivr" ? this.#ivr() : ""}
            ${this.nav === "webhooks" ? this.#hooks() : ""}
            ${this.nav === "audit" ? this.#audit() : ""}
          </div>
        </div>
      </div>
    `;
  }

  #overview() {
    const sipOk = this.status?.sip_listening ?? this.status?.sip_ok ?? true;
    return html`
      <div class="kpi-row">
        <div class="kpi"><span>在线坐席</span><strong>${this.live?.agents_online ?? 0}</strong></div>
        <div class="kpi"><span>排队</span><strong>${this.#waiting()}</strong></div>
        <div class="kpi"><span>今日接通</span><strong>${this.hist?.answered ?? this.cdr.filter((c) => c.result === "answered").length}</strong></div>
        <div class="kpi"><span>SIP</span><strong>${sipOk ? "已监听" : "关闭"}</strong></div>
      </div>
      <div class="split">
        <div class="panel">
          <h3>实时队列</h3>
          <table>
            <tr><th>队列</th><th>等待</th><th>空闲</th><th>策略</th><th>操作</th></tr>
            ${(this.live?.queues || []).map((q) => html`<tr>
              <td>${q.name}</td>
              <td>${q.waiting}</td>
              <td>${this.#idleAgents()}</td>
              <td>${this.queues.find((x) => x.name === q.name)?.strategy || "—"}</td>
              <td><button class="ghost" @click=${() => (this.nav = "queues")}>查看</button></td>
            </tr>`)}
            ${!(this.live?.queues || []).length ? html`<tr><td colspan="5" class="muted">暂无队列数据</td></tr>` : ""}
          </table>
        </div>
        <div class="panel">
          <h3>坐席状态</h3>
          <table>
            <tr><th>坐席</th><th>分机</th><th>状态</th></tr>
            ${this.agents.map((a) => html`<tr>
              <td>${a.display_name || a.extension}</td>
              <td>${a.extension || ""}</td>
              <td><span class="tag ${agentTag(a.state)}">${agentLabel(a.state)}</span></td>
            </tr>`)}
            ${!this.agents.length ? html`<tr><td colspan="3" class="muted">暂无坐席</td></tr>` : ""}
          </table>
        </div>
      </div>
      <div class="panel">
        <h3>话单</h3>
        ${this.#cdrQuery()}
        ${this.#cdrTable(this.#filteredCdr().slice(0, 12))}
      </div>
    `;
  }

  #cdrQuery() {
    return html`
      <div class="form-inline">
        <div class="field">
          <label>主叫</label>
          <input .value=${this.cdrCaller} @input=${(e) => (this.cdrCaller = e.target.value)} placeholder="主叫号码" />
        </div>
        <div class="field">
          <label>结果</label>
          <select .value=${this.cdrResult} @change=${(e) => (this.cdrResult = e.target.value)}>
            <option value="">全部</option>
            <option value="answered">answered</option>
            <option value="abandoned">abandoned</option>
            <option value="no_answer">no_answer</option>
            <option value="failed">failed</option>
          </select>
        </div>
        <button @click=${() => this.requestUpdate()}>查询</button>
        <button class="secondary" @click=${() => { this.cdrCaller = ""; this.cdrResult = ""; }}>重置</button>
        <button class="secondary" @click=${() => downloadCdrCsv()}>导出</button>
      </div>
    `;
  }

  #cdrTable(rows) {
    return html`
      <table>
        <tr><th>call_id</th><th>主叫</th><th>结果</th><th>媒介</th><th>开始</th><th>时长</th></tr>
        ${rows.map(
          (c) => html`<tr>
            <td>${c.call_id}</td>
            <td>${c.caller || "—"}</td>
            <td>${c.result}</td>
            <td>${c.session_type}</td>
            <td>${c.started_at || ""}</td>
            <td>${c.duration_sec ?? 0}s</td>
          </tr>`,
        )}
        ${!rows.length ? html`<tr><td colspan="6" class="muted">暂无话单</td></tr>` : ""}
      </table>
      <div class="pager">共 ${rows.length} 条</div>
    `;
  }

  #queues() {
    return html`
      <div class="panel">
        <h3>新建队列</h3>
        <form @submit=${(e) => this.#createQueue(e)}>
          <div class="form-inline">
            <div class="field">
              <label>名称</label>
              <input .value=${this.newQueue.name} @input=${(e) => (this.newQueue = { ...this.newQueue, name: e.target.value })} />
            </div>
            <div class="field">
              <label>溢出策略</label>
              <select .value=${this.newQueue.overflow_policy} @change=${(e) => (this.newQueue = { ...this.newQueue, overflow_policy: e.target.value })}>
                <option value="hangup">超时结束</option>
                <option value="queue">溢出到另一队列</option>
                <option value="voicemail">留言结束</option>
              </select>
            </div>
            <div class="field">
              <label>溢出目标队列 ID</label>
              <input .value=${this.newQueue.overflow_queue_id} @input=${(e) => (this.newQueue = { ...this.newQueue, overflow_queue_id: e.target.value })} />
            </div>
            <button type="submit">创建队列</button>
          </div>
          <label class="check"><input type="checkbox" .checked=${this.newQueue.video_enabled} @change=${(e) => (this.newQueue = { ...this.newQueue, video_enabled: e.target.checked })} /> 视频队列</label>
          <label class="check"><input type="checkbox" .checked=${this.newQueue.priority_enabled} @change=${(e) => (this.newQueue = { ...this.newQueue, priority_enabled: e.target.checked })} /> VIP 优先</label>
          <label class="check"><input type="checkbox" .checked=${this.newQueue.listen_announce} @change=${(e) => (this.newQueue = { ...this.newQueue, listen_announce: e.target.checked })} /> 监听提示客户</label>
        </form>
      </div>
      <div class="panel">
        <h3>技能</h3>
        <div class="form-inline">
          <div class="field">
            <label>技能名</label>
            <input .value=${this.skillName} @input=${(e) => (this.skillName = e.target.value)} />
          </div>
          <button class="secondary" @click=${() => this.#addSkill()}>创建技能</button>
          <div class="field">
            <label>坐席</label>
            <select @change=${(e) => (this.bindAgentId = e.target.value)}>
              <option value="">选择坐席</option>
              ${this.agents.map((a) => html`<option value=${a.id}>${a.display_name || a.extension}</option>`)}
            </select>
          </div>
          <div class="field">
            <label>技能</label>
            <select @change=${(e) => (this.bindSkillId = e.target.value)}>
              <option value="">选择技能</option>
              ${this.skills.map((s) => html`<option value=${s.id}>${s.name}</option>`)}
            </select>
          </div>
          <button class="secondary" @click=${() => this.#bindSkill()}>绑定技能</button>
        </div>
        <p class="muted">已有技能：${this.skills.map((s) => s.name).join("、") || "无"}</p>
      </div>
      <div class="panel">
        <h3>队列列表</h3>
        <table>
          <tr><th>名称</th><th>视频</th><th>VIP</th><th>策略</th><th>溢出</th><th>操作</th></tr>
          ${this.queues.map(
            (q) => html`<tr>
              <td>${q.name}</td>
              <td>${q.video_enabled ? "是" : "否"}</td>
              <td>${q.priority_enabled ? "是" : "否"}</td>
              <td>${q.strategy}</td>
              <td>${q.overflow_policy || "-"}</td>
              <td>
                <button class="secondary" @click=${() => this.#bindAll(q.id)}>绑定全部坐席</button>
                <button class="secondary" @click=${() => this.#toggleVip(q)}>VIP</button>
              </td>
            </tr>`,
          )}
        </table>
        <div class="pager">共 ${this.queues.length} 条</div>
      </div>
    `;
  }

  #agents() {
    return html`
      <div class="panel">
        <h3>创建用户 / 坐席</h3>
        <form @submit=${(e) => this.#createUser(e)}>
          <div class="form-inline">
            <div class="field"><label>用户名</label><input .value=${this.newUser.username} @input=${(e) => (this.newUser = { ...this.newUser, username: e.target.value })} /></div>
            <div class="field"><label>密码</label><input .value=${this.newUser.password} @input=${(e) => (this.newUser = { ...this.newUser, password: e.target.value })} /></div>
            <div class="field">
              <label>角色</label>
              <select .value=${this.newUser.role} @change=${(e) => (this.newUser = { ...this.newUser, role: e.target.value })}>
                <option value="agent">agent</option>
                <option value="admin">admin</option>
                <option value="supervisor">supervisor</option>
              </select>
            </div>
            <div class="field"><label>分机</label><input .value=${this.newUser.extension} @input=${(e) => (this.newUser = { ...this.newUser, extension: e.target.value })} /></div>
            <div class="field"><label>展示名</label><input .value=${this.newUser.display_name} @input=${(e) => (this.newUser = { ...this.newUser, display_name: e.target.value })} /></div>
            <button type="submit">创建</button>
          </div>
          <label class="check"><input type="checkbox" .checked=${this.newUser.video_capable} @change=${(e) => (this.newUser = { ...this.newUser, video_capable: e.target.checked })} /> 视频能力</label>
        </form>
      </div>
      <div class="panel">
        <h3>用户</h3>
        <table>
          <tr><th>用户</th><th>角色</th><th>分机</th><th>坐席 ID</th></tr>
          ${this.users.map((u) => html`<tr><td>${u.username}</td><td>${u.role}</td><td>${u.extension || ""}</td><td>${u.agent_id || ""}</td></tr>`)}
        </table>
        <div class="pager">共 ${this.users.length} 条</div>
      </div>
      <div class="panel">
        <h3>班长强制签出</h3>
        <div class="form-inline">
          <div class="field">
            <label>坐席</label>
            <select @change=${(e) => (this.forceAgentId = e.target.value)}>
              <option value="">选择坐席</option>
              ${this.agents.map((a) => html`<option value=${a.id}>${a.display_name || a.extension} (${a.state})</option>`)}
            </select>
          </div>
          <button class="danger" @click=${() => this.#force()}>强制签出</button>
        </div>
      </div>
      <div class="panel">
        <h3>坐席利用率</h3>
        <table>
          <tr><th>坐席</th><th>空闲</th><th>通话</th><th>示忙</th><th>利用率</th></tr>
          ${this.utils.map((u) => html`<tr><td>${u.agent_id}</td><td>${Math.round(u.idle_sec)}s</td><td>${Math.round(u.on_call_sec)}s</td><td>${Math.round(u.busy_sec)}s</td><td>${((u.utilization || 0) * 100).toFixed(0)}%</td></tr>`)}
        </table>
        <h4 style="margin-top:16px">历史报表</h4>
        <pre>${JSON.stringify(this.hist, null, 2)}</pre>
      </div>
    `;
  }

  #dids() {
    return html`
      <div class="panel">
        <h3>DID / 外显</h3>
        <form @submit=${(e) => this.#saveDid(e)}>
          <div class="form-inline">
            <div class="field"><label>DID</label><input .value=${this.didForm.did} @input=${(e) => (this.didForm = { ...this.didForm, did: e.target.value })} /></div>
            <div class="field"><label>队列 ID</label><input .value=${this.didForm.queue_id} @input=${(e) => (this.didForm = { ...this.didForm, queue_id: e.target.value })} /></div>
            <div class="field"><label>外显名称</label><input .value=${this.didForm.display_name} @input=${(e) => (this.didForm = { ...this.didForm, display_name: e.target.value })} /></div>
            <button type="submit">保存</button>
          </div>
        </form>
        <table>
          <tr><th>DID</th><th>队列</th><th>外显</th></tr>
          ${this.dids.map((d) => html`<tr>
            <td>${d.did || d.DID || d.d_id}</td>
            <td>${d.queue_id || d.QueueID || ""}</td>
            <td>${d.display_name || d.DisplayName || ""}</td>
          </tr>`)}
          ${!this.dids.length ? html`<tr><td colspan="3" class="muted">暂无 DID</td></tr>` : ""}
        </table>
        <div class="pager">共 ${this.dids.length} 条</div>
      </div>
    `;
  }

  #cdrPage() {
    const rows = this.#filteredCdr();
    return html`
      <div class="panel">
        <h3>CDR</h3>
        ${this.#cdrQuery()}
        ${this.#cdrTable(rows)}
      </div>
      <div class="panel">
        <h3>通话小结</h3>
        <table>
          <tr><th>时间</th><th>call_id</th><th>坐席</th><th>内容</th></tr>
          ${this.wrapUps.map((w) => html`<tr><td>${w.created_at}</td><td>${w.call_id}</td><td>${w.agent_id}</td><td>${w.notes}</td></tr>`)}
        </table>
      </div>
      <div class="panel">
        <h3>质检标记</h3>
        <form @submit=${(e) => this.#qa(e)}>
          <div class="form-inline">
            <div class="field"><label>call_id</label><input .value=${this.qaCallId} @input=${(e) => (this.qaCallId = e.target.value)} /></div>
            <div class="field"><label>标签</label><input .value=${this.qaLabel} @input=${(e) => (this.qaLabel = e.target.value)} /></div>
            <button type="submit">打点</button>
          </div>
        </form>
      </div>
    `;
  }

  #recs() {
    return html`
      <div class="panel">
        <h3>录音</h3>
        <table>
          <tr><th>ID</th><th>通话</th><th>类型</th><th>大小</th><th>操作</th></tr>
          ${this.recs.map((r) => html`<tr>
            <td>${r.id}</td><td>${r.call_id}</td><td>${r.media_type}</td><td>${r.file_size}</td>
            <td>
              <button class="secondary" @click=${() => downloadRecording(r.id)}>下载</button>
              <button class="secondary" @click=${() => this.#playRec(r.id)}>回放</button>
            </td>
          </tr>`)}
        </table>
        <div class="pager">共 ${this.recs.length} 条</div>
        ${this.playUrl ? html`<audio controls autoplay src=${this.playUrl}></audio><video controls src=${this.playUrl}></video>` : ""}
      </div>
    `;
  }

  #ivr() {
    const rows = Array.isArray(this.ivrs) ? this.ivrs : [];
    return html`
      <div class="panel">
        <h3>IVR</h3>
        <div class="toolbar"><button class="secondary" @click=${() => this.#ivrDemo()}>发布示例 IVR（1语音/2视频）</button></div>
        <table>
          <tr><th>ID</th><th>名称</th><th>状态</th></tr>
          ${rows.map((f) => html`<tr><td>${f.id || ""}</td><td>${f.name || ""}</td><td>${f.published ? "已发布" : "草稿"}</td></tr>`)}
          ${!rows.length ? html`<tr><td colspan="3" class="muted">暂无流程，可发布示例</td></tr>` : ""}
        </table>
        <pre>${JSON.stringify(this.ivrs, null, 2)}</pre>
      </div>
    `;
  }

  #hooks() {
    const rows = Array.isArray(this.hooks) ? this.hooks : [];
    return html`
      <div class="panel">
        <h3>Webhook</h3>
        <div class="form-inline">
          <div class="field" style="min-width:320px">
            <label>回调 URL</label>
            <input .value=${this.hookUrl} @input=${(e) => (this.hookUrl = e.target.value)} />
          </div>
          <button class="secondary" @click=${() => this.#addHook()}>订阅全部事件</button>
        </div>
        <table>
          <tr><th>URL</th><th>启用</th></tr>
          ${rows.map((h) => html`<tr><td>${h.url || h.URL || ""}</td><td>${h.enabled === false ? "否" : "是"}</td></tr>`)}
        </table>
        <pre>${JSON.stringify(this.hooks, null, 2)}</pre>
      </div>
    `;
  }

  #audit() {
    return html`
      <div class="panel">
        <h3>审计</h3>
        <table>
          <tr><th>时间</th><th>动作</th><th>人</th><th>资源</th></tr>
          ${this.audit.map((a) => html`<tr><td>${a.created_at}</td><td>${a.action}</td><td>${a.user_id || ""}</td><td>${a.resource || ""}</td></tr>`)}
        </table>
        <div class="pager">共 ${this.audit.length} 条</div>
      </div>
    `;
  }
}

customElements.define("open-voip-admin-app", AdminApp);

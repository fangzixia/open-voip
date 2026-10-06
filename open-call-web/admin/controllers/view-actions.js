// 本文件负责管理界面的视图操作回调。
/** 在应用边界处理视图草稿更新与导航。 */
export function adminViewActions(host, operations) {
  return {
    ...operations,
    navigate: (id) => {
      if (host.contentOnly) {
        host.dispatchEvent(new CustomEvent("staff-navigate", { detail: { id }, bubbles: true, composed: true }));
        return;
      }
      if (host.feedback) host.feedback.begin();
      else { host.error = ""; host.notice = ""; }
      host.nav = id;
      if (typeof operations.loadNav === "function") void operations.loadNav(id);
    },
    setUsername: (value) => { host.username = value; },
    setPassword: (value) => { host.password = value; },
    updateNewQueue: (video, patch) => {
      const key = video ? "newVideoQueue" : "newVoiceQueue";
      host[key] = { ...host[key], ...patch };
    },
    updateDidForm: (patch) => { host.didForm = { ...host.didForm, ...patch }; },
    setSkillName: (value) => { host.skillName = value; },
    setBindAgentId: (value) => { host.bindAgentId = value; },
    setBindSkillId: (value) => { host.bindSkillId = value; },
    setForceAgentId: (value) => { host.forceAgentId = value; },
    setHookUrl: (value) => { host.hookUrl = value; },
    setCdrCaller: (value) => { host.cdrCaller = value; },
    setCdrResult: (value) => { host.cdrResult = value; },
    search: () => operations.loadCdr?.(1),
    resetCdr: () => { host.cdrCaller = ""; host.cdrResult = ""; return operations.loadCdr?.(1); },
    cdrPrev: () => { if (host.cdrPage > 1) return operations.loadCdr?.(host.cdrPage - 1); },
    cdrNext: () => { if (host.cdrPage * host.cdrPageSize < host.cdrTotal) return operations.loadCdr?.(host.cdrPage + 1); },
    setQaCallId: (value) => { host.qaCallId = value; },
    setQaLabel: (value) => { host.qaLabel = value; },
    setBridgeForm: (patch) => { host.bridgeForm = { ...host.bridgeForm, ...patch }; },
  };
}

// 本文件负责管理界面的视图操作回调。
/** 在应用边界处理视图草稿更新与导航。 */
export function adminViewActions(host, operations) {
  return {
    ...operations,
    navigate: (id) => { host.nav = id; },
    setUsername: (value) => { host.username = value; },
    setPassword: (value) => { host.password = value; },
    updateNewQueue: (patch) => { host.newQueue = { ...host.newQueue, ...patch }; },
    updateDidForm: (patch) => { host.didForm = { ...host.didForm, ...patch }; },
    setSkillName: (value) => { host.skillName = value; },
    setBindAgentId: (value) => { host.bindAgentId = value; },
    setBindSkillId: (value) => { host.bindSkillId = value; },
    setForceAgentId: (value) => { host.forceAgentId = value; },
    setHookUrl: (value) => { host.hookUrl = value; },
    setCdrCaller: (value) => { host.cdrCaller = value; },
    setCdrResult: (value) => { host.cdrResult = value; },
    search: () => host.requestUpdate(),
    resetCdr: () => { host.cdrCaller = ""; host.cdrResult = ""; },
    setQaCallId: (value) => { host.qaCallId = value; },
    setQaLabel: (value) => { host.qaLabel = value; },
  };
}

// 本文件负责管理页使用的状态与策略文案。
export function strategyLabel(strategy) {
  switch (strategy) {
    case "longest_idle": return "最长空闲优先";
    case "round_robin": return "轮询分配";
    default: return strategy ? "未知策略" : "—";
  }
}

export function overflowPolicyLabel(policy) {
  switch (policy) {
    case "hangup": return "超时结束";
    case "queue": return "溢出到另一队列";
    case "voicemail": return "留言结束";
    default: return policy ? "未知溢出策略" : "—";
  }
}

export function roleLabel(role) {
  switch (role) {
    case "agent": return "坐席";
    case "admin": return "管理员";
    case "supervisor": return "班长";
    default: return role ? "未知角色" : "—";
  }
}

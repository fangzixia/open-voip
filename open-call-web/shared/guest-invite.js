// 本文件负责访客邀请信息解析与校验。
export function parseGuestInvite(search = "") {
  const params = new URLSearchParams(search);
  const token = params.get("token") || "";
  const media = params.get("media") === "video" ? "video" : "audio";
  return { token, media };
}

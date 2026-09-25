export function parseGuestInvite(search = "") {
  const params = new URLSearchParams(search);
  const token = params.get("token") || "";
  const media = params.get("media") === "video" ? "video" : "audio";
  return { token, media };
}

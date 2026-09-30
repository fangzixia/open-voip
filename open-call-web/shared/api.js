// 前端 API 门面：按域拆分于 ./api/，此处保持兼容再导出。
export * from "./api/index.js";
export { setCallVersion, getCallVersion } from "./call-mutation.js";

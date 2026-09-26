// 本文件负责IVR 编辑器接口适配。
import {
  createIvrFlow, deleteIvrFlow, fetchIvrAsset, getIvrFlow, listIvrAssets,
  listIvrVersions, patchQueue, publishIvr, rollbackIvrFlow, updateIvrFlow,
  uploadIvrAsset,
} from "../../shared/api.js";

/** 注入可复用 IVR 编辑器的接口实现。 */
export const ivrService = {
  createIvrFlow, deleteIvrFlow, fetchIvrAsset, getIvrFlow, listIvrAssets,
  listIvrVersions, patchQueue, publishIvr, rollbackIvrFlow, updateIvrFlow,
  uploadIvrAsset,
};

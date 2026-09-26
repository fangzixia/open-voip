// 本文件负责样式入口与组合。
import { tokenStyles } from "./tokens.js";
import { baseStyles } from "./base.js";
import { layoutStyles } from "./layout.js";
import { componentStyles } from "./components.js";
import { callStyles } from "./call.js";

export const appStyles = [tokenStyles, baseStyles, layoutStyles, componentStyles, callStyles];

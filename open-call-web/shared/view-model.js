// 本文件负责视图状态投影，限制子视图可访问的属性。
/** 仅向视图传入调用处声明的状态与回调，避免视图依赖整个应用状态。 */
export function project(source, names) {
  const result = {};
  for (const name of names) {
    if (!(name in source)) throw new Error(`Missing view property: ${name}`);
    result[name] = source[name];
  }
  return Object.freeze(result);
}

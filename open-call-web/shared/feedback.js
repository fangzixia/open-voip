// 应用级瞬时反馈：用 epoch 界定作用域，避免跨页/跨操作串扰。

let epochReader = () => 0;

/** 供 apiFetch 在发起请求时读取当前反馈世代。 */
export function installFeedbackEpochReader(reader) {
  epochReader = typeof reader === "function" ? reader : () => 0;
}

export function currentFeedbackEpoch() {
  return epochReader();
}

/**
 * Lit 宿主上的反馈控制器。
 * - begin()：新作用域（导航、用户操作开始），清空横幅并递增 epoch
 * - fail/ok：仅当 epoch 仍匹配时写入（过期异步结果丢弃）
 * - liveNotice/liveError：通话等实时事件，不 bump epoch
 */
export class FeedbackController {
  #host;
  #epoch = 0;

  constructor(host) {
    this.#host = host;
    host.addController(this);
    installFeedbackEpochReader(() => this.#epoch);
  }

  hostDisconnected() {
    installFeedbackEpochReader(() => 0);
  }

  get epoch() {
    return this.#epoch;
  }

  begin() {
    this.#epoch += 1;
    this.#host.error = "";
    this.#host.notice = "";
    return this.#epoch;
  }

  clear() {
    this.#host.error = "";
    this.#host.notice = "";
  }

  fail(err, epoch = this.#epoch) {
    if (epoch !== this.#epoch) return false;
    this.#host.notice = "";
    this.#host.error = err instanceof Error ? err.message : String(err ?? "");
    return true;
  }

  ok(msg = "", epoch = this.#epoch) {
    if (epoch !== this.#epoch) return false;
    this.#host.error = "";
    this.#host.notice = msg || "";
    return true;
  }

  liveNotice(msg) {
    this.#host.error = "";
    this.#host.notice = msg || "";
  }

  liveError(err) {
    this.#host.notice = "";
    this.#host.error = err instanceof Error ? err.message : String(err ?? "");
  }
}

/** 用户操作封装：开始即 begin，异常按该 epoch fail；成功不自动写 notice。 */
export async function runFeedbackAction(feedback, fn) {
  const epoch = feedback.begin();
  try {
    return await fn(epoch);
  } catch (err) {
    feedback.fail(err, epoch);
    return undefined;
  }
}

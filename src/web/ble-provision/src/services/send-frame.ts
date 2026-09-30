import { parseStatus, resultError } from "../protocol/status.ts";
import type { Frame } from "../protocol/frames.ts";

export class DeviceResultError extends Error {
  readonly result: number;

  constructor(result: number) {
    super(resultError(result));
    this.result = result;
  }
}

export function sendFrame(
  request: BluetoothRemoteGATTCharacteristic,
  reply: BluetoothRemoteGATTCharacteristic,
  frame: Frame,
  number: number,
  total: number,
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  return new Promise<void>((resolve, reject) => {
    let finished = false;
    let writeDone = false;
    let replyDone = false;
    let replyError: Error | undefined;
    const finish = (error?: Error) => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      reply.removeEventListener("characteristicvaluechanged", onReply);
      if (error) reject(error);
      else resolve();
    };
    const finishIfComplete = () => {
      if (writeDone && replyDone) finish(replyError);
    };
    const onReply = (event: Event) => {
      try {
        const value = (event.target as BluetoothRemoteGATTCharacteristic).value;
        if (!value) throw new Error("设备返回了空状态");
        const status = parseStatus(value);
        if (status.result >= 0x80) {
          replyError = new DeviceResultError(status.result);
          replyDone = true;
          finishIfComplete();
        } else if (status.result === frame.expected) {
          replyDone = true;
          finishIfComplete();
        }
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)));
      }
    };
    const onAbort = () => finish(new DOMException("操作已取消", "AbortError"));
    const timer = setTimeout(() => finish(new Error(`第 ${number}/${total} 个数据包等待设备响应超时`)), 8000);
    signal.addEventListener("abort", onAbort, { once: true });
    reply.addEventListener("characteristicvaluechanged", onReply);
    request.writeValueWithResponse(new Uint8Array(frame.bytes)).then(
      () => { writeDone = true; finishIfComplete(); },
      (error: unknown) => finish(error instanceof Error ? error : new Error(String(error))),
    );
  });
}

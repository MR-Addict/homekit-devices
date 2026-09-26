import assert from "node:assert/strict";
import test from "node:test";
import { RESULT, WIFI } from "../src/protocol/constants.ts";
import { sendFrame, DeviceResultError } from "../src/services/send-frame.ts";

class Reply extends EventTarget {
  value: DataView | undefined;

  emit(result: number): void {
    this.value = new DataView(Uint8Array.of(1, result, WIFI.CONNECTING).buffer);
    this.dispatchEvent(new Event("characteristicvaluechanged"));
  }
}

function characteristics() {
  const reply = new Reply();
  const writes: Uint8Array[] = [];
  const request = {
    writeValueWithResponse: async (value: Uint8Array) => { writes.push(value); },
  } as unknown as BluetoothRemoteGATTCharacteristic;
  return { request, reply: reply as unknown as BluetoothRemoteGATTCharacteristic, emitter: reply, writes };
}

const frame = { bytes: Uint8Array.of(1, 0, 0), expected: RESULT.AUTH_OK };

test("只在写入完成且收到匹配响应后完成，并忽略其他状态通知", async () => {
  const { request, reply, emitter, writes } = characteristics();
  const pending = sendFrame(request, reply, frame, 1, 1, new AbortController().signal);
  let settled = false;
  void pending.then(() => { settled = true; });
  emitter.emit(RESULT.READY);
  await Promise.resolve();
  assert.equal(settled, false);
  emitter.emit(RESULT.AUTH_OK);
  await pending;
  assert.deepEqual([...writes[0]!], [...frame.bytes]);
  assert.notEqual(writes[0], frame.bytes);
});

test("设备错误作为带结果码的错误返回", async () => {
  const { request, reply, emitter } = characteristics();
  const pending = sendFrame(request, reply, frame, 1, 1, new AbortController().signal);
  emitter.emit(RESULT.BAD_AUTH);
  await assert.rejects(pending, (error: unknown) =>
    error instanceof DeviceResultError && error.result === RESULT.BAD_AUTH);
});

test("取消操作时停止等待响应", async () => {
  const { request, reply, emitter } = characteristics();
  const controller = new AbortController();
  const pending = sendFrame(request, reply, frame, 1, 1, controller.signal);
  controller.abort();
  await assert.rejects(pending, { name: "AbortError" });
  emitter.emit(RESULT.AUTH_OK);
});

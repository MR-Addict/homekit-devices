import assert from "node:assert/strict";
import test from "node:test";
import { ProvisionController } from "../src/workflows/controller.ts";
import { RESULT } from "../src/protocol/constants.ts";
import type { Frame } from "../src/protocol/frames.ts";
import type { ProvisionView } from "../src/ui/view.ts";
import type { Phase } from "../src/workflows/flow.ts";

test("保存完成后设备重启断线仍显示完成界面", () => {
  const calls: string[] = [];
  const view = {
    clearWifiInputs: () => calls.push("clear-wifi"),
    setDeviceState: (message: string) => calls.push(message),
    render: (phase: Phase) => calls.push(`render:${phase}`),
  } as unknown as ProvisionView;
  const controller = new ProvisionController(view);
  const internals = controller as unknown as { phase: Phase; onDisconnected: () => void };
  internals.phase = "done";

  internals.onDisconnected();

  assert.deepEqual(calls, ["蓝牙已断开"]);
  assert.equal(internals.phase, "done");
});

test("完成后配置另一台设备会断开并清空当前设备状态", () => {
  const calls: string[] = [];
  let another: (() => void) | undefined;
  const view = {
    bind: (handlers: { another: () => void }) => { another = handlers.another; },
    render: (phase: Phase) => calls.push(`render:${phase}`),
    showStatus: () => {},
    clearWifiInputs: () => calls.push("clear-wifi"),
    clearOtaPassword: () => calls.push("clear-ota"),
    setDeviceName: (name: string) => calls.push(`device:${name}`),
    clearStatus: () => calls.push("clear-status"),
    focusConnect: () => calls.push("focus-connect"),
  } as unknown as ProvisionView;
  const controller = new ProvisionController(view);
  const internals = controller as unknown as { phase: Phase; session: { disconnect: () => void } };
  internals.phase = "done";
  internals.session = { disconnect: () => calls.push("disconnect") };
  controller.start();

  another?.();

  assert.deepEqual(calls, [
    "render:done", "disconnect", "clear-wifi", "clear-ota", "device:尚未选择设备",
    "clear-status", "render:disconnected", "focus-connect",
  ]);
  assert.equal(internals.phase, "disconnected");
});

function provisionHarness(sendFrame: (frame: Frame) => Promise<void>) {
  const calls: string[] = [];
  const view = {
    get ssid() { return "Test WiFi"; },
    get wifiPassword() { return "password"; },
    render: (phase: Phase) => calls.push(`render:${phase}`),
    showStatus: (message: string) => calls.push(`status:${message}`),
    clearStatus: () => calls.push("clear-status"),
    focusAnother: () => calls.push("focus-another"),
  } as unknown as ProvisionView;
  const controller = new ProvisionController(view);
  const internals = controller as unknown as {
    phase: Phase;
    authorized: boolean;
    session: { connected: boolean; sendFrame: (frame: Frame) => Promise<void> };
    submitProvision: () => Promise<void>;
  };
  internals.phase = "wifi";
  internals.authorized = true;
  internals.session = { connected: true, sendFrame };
  return { calls, internals };
}

test("完整提交收到 SAVED 回执后立即显示完成", async () => {
  let acknowledgeSaved: (() => void) | undefined;
  let commitSent: (() => void) | undefined;
  const commitStarted = new Promise<void>((resolve) => { commitSent = resolve; });
  const savedReply = new Promise<void>((resolve) => { acknowledgeSaved = resolve; });
  const { calls, internals } = provisionHarness(async (frame) => {
    if (frame.expected === RESULT.SAVED) {
      commitSent?.();
      await savedReply;
    }
  });
  const submission = internals.submitProvision();
  await commitStarted;
  assert.equal(internals.phase, "submitting");
  acknowledgeSaved?.();
  await submission;
  assert.equal(internals.phase, "done");
  assert.equal(calls.at(-2), "clear-status");
  assert.equal(calls.at(-1), "focus-another");
});

test("提交失败且未收到 SAVED 回执时保留表单和错误", async () => {
  const { calls, internals } = provisionHarness(async () => { throw new Error("BLE 写入失败"); });
  await internals.submitProvision();
  assert.equal(internals.phase, "wifi");
  assert.ok(calls.some((call) => call.includes("配网未完成：BLE 写入失败")));
  assert.ok(!calls.includes("render:done"));
});

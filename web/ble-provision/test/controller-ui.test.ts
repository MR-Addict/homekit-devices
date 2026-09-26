import assert from "node:assert/strict";
import test from "node:test";
import { ProvisionController } from "../src/workflows/controller.ts";
import type { ProvisionView } from "../src/ui/view.ts";
import type { Phase } from "../src/workflows/flow.ts";

test("设备重启时蓝牙断开仍停留在 Wi‑Fi 确认步骤", () => {
  const calls: string[] = [];
  const view = {
    clearWifiInputs: () => calls.push("clear-wifi"),
    setDeviceState: (message: string) => calls.push(message),
    render: (phase: Phase) => calls.push(`render:${phase}`),
  } as unknown as ProvisionView;
  const controller = new ProvisionController(view);
  const internals = controller as unknown as { phase: Phase; onDisconnected: () => void };
  internals.phase = "checking";

  internals.onDisconnected();

  assert.deepEqual(calls, ["clear-wifi", "设备重启中，等待蓝牙重连"]);
  assert.equal(internals.phase, "checking");
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
  const internals = controller as unknown as { phase: Phase; session: { disconnect: () => void }; saved: boolean; confirmed: boolean };
  internals.phase = "done";
  internals.saved = true;
  internals.confirmed = true;
  internals.session = { disconnect: () => calls.push("disconnect") };
  controller.start();

  another?.();

  assert.deepEqual(calls, [
    "render:done", "disconnect", "clear-wifi", "clear-ota", "device:尚未选择设备",
    "clear-status", "render:disconnected", "focus-connect",
  ]);
  assert.equal(internals.phase, "disconnected");
  assert.equal(internals.saved, false);
  assert.equal(internals.confirmed, false);
});

import assert from "node:assert/strict";
import test from "node:test";
import { canDisconnect, disconnectMessage, showAuthForm, showWifiForm } from "../src/flow.ts";

test("连接区始终可见，认证和 Wi‑Fi 表单按需展开", () => {
  assert.equal(canDisconnect("disconnected"), false);
  assert.equal(canDisconnect("connecting"), false);
  for (const phase of ["auth", "authenticating", "wifi", "submitting", "checking", "done"] as const) {
    assert.equal(canDisconnect(phase), true);
  }
  assert.equal(showAuthForm("auth"), true);
  assert.equal(showAuthForm("authenticating"), true);
  assert.equal(showAuthForm("wifi"), false);
  assert.equal(showWifiForm("auth"), false);
  for (const phase of ["wifi", "submitting", "checking", "done"] as const) {
    assert.equal(showWifiForm(phase), true);
  }
});

test("断开后区分未保存、已保存未核实与已确认", () => {
  assert.match(disconnectMessage(false, false), /蓝牙已断开/);
  assert.match(disconnectMessage(true, false), /结果尚未核实/);
  assert.match(disconnectMessage(true, true), /配网成功/);
});

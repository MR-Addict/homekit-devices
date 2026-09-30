import assert from "node:assert/strict";
import test from "node:test";
import { canDisconnect, disconnectMessage, showAuthForm, showConnectPanel, showSuccessPanel, showWifiForm } from "../src/workflows/flow.ts";

test("每个阶段只显示对应表单或完成画面", () => {
  assert.equal(canDisconnect("disconnected"), false);
  assert.equal(canDisconnect("connecting"), false);
  for (const phase of ["auth", "authenticating", "wifi", "submitting", "done"] as const) {
    assert.equal(canDisconnect(phase), true);
  }
  for (const phase of ["disconnected", "connecting"] as const) {
    assert.equal(showConnectPanel(phase), true);
  }
  assert.equal(showAuthForm("auth"), true);
  assert.equal(showAuthForm("authenticating"), true);
  assert.equal(showAuthForm("wifi"), false);
  assert.equal(showWifiForm("auth"), false);
  for (const phase of ["wifi", "submitting"] as const) {
    assert.equal(showWifiForm(phase), true);
  }
  assert.equal(showWifiForm("done"), false);
  assert.equal(showSuccessPanel("done"), true);
});

test("未完成时断开显示可重新搜索", () => {
  assert.match(disconnectMessage(), /蓝牙已断开/);
});

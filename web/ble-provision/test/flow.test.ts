import assert from "node:assert/strict";
import test from "node:test";
import { canDisconnect, currentStep, disconnectMessage, showAuthForm, showConnectPanel, showSuccessPanel, showWifiForm } from "../src/workflows/flow.ts";

test("每个阶段只显示当前步骤或完成画面", () => {
  assert.equal(canDisconnect("disconnected"), false);
  assert.equal(canDisconnect("connecting"), false);
  for (const phase of ["auth", "authenticating", "wifi", "submitting", "checking", "done"] as const) {
    assert.equal(canDisconnect(phase), true);
  }
  for (const phase of ["disconnected", "connecting"] as const) {
    assert.equal(showConnectPanel(phase), true);
    assert.equal(currentStep(phase), 1);
  }
  assert.equal(showAuthForm("auth"), true);
  assert.equal(showAuthForm("authenticating"), true);
  assert.equal(showAuthForm("wifi"), false);
  assert.equal(currentStep("auth"), 2);
  assert.equal(showWifiForm("auth"), false);
  for (const phase of ["wifi", "submitting", "checking"] as const) {
    assert.equal(showWifiForm(phase), true);
    assert.equal(currentStep(phase), 3);
  }
  assert.equal(showWifiForm("done"), false);
  assert.equal(showSuccessPanel("done"), true);
  assert.equal(currentStep("done"), 3);
});

test("断开后区分未保存、重启期间尚未核实与已确认", () => {
  assert.match(disconnectMessage(false, false), /蓝牙已断开/);
  assert.match(disconnectMessage(true, false), /结果尚未核实/);
  assert.match(disconnectMessage(true, true), /配网成功/);
});

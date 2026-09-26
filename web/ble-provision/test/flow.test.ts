import assert from "node:assert/strict";
import test from "node:test";
import { nextStep } from "../src/flow.ts";

test("连接、认证、提交和重新配置依次推进", () => {
  assert.equal(nextStep("connect", "authenticated"), "connect");
  assert.equal(nextStep("connect", "connected"), "auth");
  assert.equal(nextStep("auth", "submitting"), "auth");
  assert.equal(nextStep("auth", "authenticated"), "wifi");
  assert.equal(nextStep("wifi", "submitting"), "result");
  assert.equal(nextStep("result", "retry"), "auth");
});

test("断线和授权过期退回安全步骤", () => {
  for (const step of ["connect", "auth", "wifi", "result"] as const) {
    assert.equal(nextStep(step, "disconnected"), "connect");
    assert.equal(nextStep(step, "unauthorized"), "auth");
  }
});

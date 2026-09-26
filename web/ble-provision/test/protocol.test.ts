import assert from "node:assert/strict";
import test from "node:test";
import { createAuthFrame, createProvisionFrames, parseStatus, RESULT, WIFI } from "../src/protocol.ts";

test("OTA 密码单独验证，数据包不超过默认 BLE 写入长度", () => {
  const auth = createAuthFrame("ota");
  assert.deepEqual([...auth.bytes], [1, 0, 3, 111, 116, 97]);
  assert.equal(auth.expected, RESULT.AUTH_OK);
  assert.throws(() => createAuthFrame(""), /OTA 密码/);
  assert.throws(() => createAuthFrame("x".repeat(18)), /OTA 密码/);
  assert.throws(() => createAuthFrame("a\0b"), /空字符/);
});

test("凭据分包保持字节顺序与协议 v1 帧格式", () => {
  const frames = createProvisionFrames("测试网络", "a".repeat(64));
  assert.equal(frames[0]!.expected, RESULT.BEGIN_OK);
  assert.equal(frames.at(-1)!.expected, RESULT.SAVED);
  assert.ok(frames.every(({ bytes }) => bytes.length <= 20));
  const data = frames.slice(1, -1);
  data.forEach(({ bytes }, index) => assert.equal(bytes[1], index));
  const actual = Uint8Array.from(data.flatMap(({ bytes }) => [...bytes.slice(3)]));
  assert.deepEqual(actual, new TextEncoder().encode("测试网络" + "a".repeat(64)));
  assert.equal(frames[0]!.bytes[3], new TextEncoder().encode("测试网络").length);
  assert.equal(frames[0]!.bytes[4], 64);
});

test("支持无密码网络并拒绝超长或空字符输入", () => {
  assert.equal(createProvisionFrames("Guest", "").at(-1)!.bytes[0], 4);
  assert.throws(() => createProvisionFrames("", "password"), /Wi‑Fi 名称/);
  assert.throws(() => createProvisionFrames("a".repeat(33), "password"), /32 字节/);
  assert.throws(() => createProvisionFrames("Guest", "a".repeat(65)), /64 字节/);
  assert.throws(() => createProvisionFrames("Guest", "a\0b"), /空字符/);
});

test("只接受 v1 的三字节状态", () => {
  assert.deepEqual(parseStatus(new DataView(Uint8Array.of(1, RESULT.READY, WIFI.CONNECTED).buffer)), {
    result: RESULT.READY,
    wifi: WIFI.CONNECTED,
  });
  assert.throws(() => parseStatus(Uint8Array.of(2, 0, 0)), /不兼容/);
});

import { RESULT } from "./constants.ts";

export interface Frame {
  bytes: Uint8Array;
  expected: number;
}

const OP = { AUTH: 1, BEGIN: 2, DATA: 3, COMMIT: 4 } as const;
const MAX_DATA_BYTES = 17;
const encoder = new TextEncoder();

function frame(opcode: number, sequence: number, payload = new Uint8Array()): Uint8Array {
  if (payload.length > MAX_DATA_BYTES) throw new Error("BLE 数据包过长");
  return Uint8Array.of(opcode, sequence, payload.length, ...payload);
}

export function createAuthFrame(otaPassword: string): Frame {
  const auth = encoder.encode(otaPassword);
  if (auth.length === 0 || auth.length > MAX_DATA_BYTES || auth.includes(0)) {
    throw new Error("OTA 密码必须为 1–17 字节，且不能包含空字符");
  }
  const bytes = frame(OP.AUTH, 0, auth);
  auth.fill(0);
  return { bytes, expected: RESULT.AUTH_OK };
}

export function createProvisionFrames(ssid: string, wifiPassword: string): Frame[] {
  const name = encoder.encode(ssid);
  const password = encoder.encode(wifiPassword);
  if (name.length === 0 || name.length > 32) {
    throw new Error("Wi‑Fi 名称长度必须为 1–32 字节（按 UTF‑8 计算）");
  }
  if (password.length > 64) {
    throw new Error("Wi‑Fi 密码不能超过 64 字节（按 UTF‑8 计算）");
  }
  if (name.includes(0) || password.includes(0)) {
    throw new Error("输入不能包含空字符");
  }

  const credentials = new Uint8Array(name.length + password.length);
  credentials.set(name);
  credentials.set(password, name.length);
  const frames: Frame[] = [
    {
      bytes: frame(OP.BEGIN, 0, Uint8Array.of(name.length, password.length)),
      expected: RESULT.BEGIN_OK,
    },
  ];
  for (let offset = 0, sequence = 0; offset < credentials.length; offset += MAX_DATA_BYTES) {
    frames.push({
      bytes: frame(OP.DATA, sequence++, credentials.subarray(offset, offset + MAX_DATA_BYTES)),
      expected: RESULT.DATA_OK,
    });
  }
  frames.push({ bytes: frame(OP.COMMIT, 0), expected: RESULT.SAVED });
  credentials.fill(0);
  password.fill(0);
  return frames;
}

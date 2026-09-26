export const SERVICE_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c832";
export const COMMAND_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c833";
export const STATUS_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c834";
export const CREDENTIALS_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c835";
export const VERSION = 1;

export const RESULT = {
  READY: 0,
  AUTH_OK: 1,
  BEGIN_OK: 2,
  DATA_OK: 3,
  SAVED: 4,
  BAD_AUTH: 0x80,
  BAD_FORMAT: 0x81,
  BAD_SEQUENCE: 0x82,
  UNAUTHORIZED: 0x83,
  BUSY: 0x84,
} as const;

export const WIFI = {
  NO_CREDENTIALS: 0,
  CONNECTING: 1,
  CONNECTED: 2,
  NOT_CONNECTED: 3,
} as const;

const OP = { AUTH: 1, BEGIN: 2, DATA: 3, COMMIT: 4 } as const;
const MAX_DATA_BYTES = 17;
const encoder = new TextEncoder();
const decoder = new TextDecoder("utf-8", { fatal: true });

export interface Frame {
  bytes: Uint8Array;
  expected: number;
}

export interface DeviceStatus {
  result: number;
  wifi: number;
}

export interface WifiCredentials {
  ssid: string;
  password: string;
}

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

export function parseStatus(value: DataView | Uint8Array): DeviceStatus {
  const bytes = value instanceof DataView ? new Uint8Array(value.buffer, value.byteOffset, value.byteLength) : value;
  if (bytes.length !== 3 || bytes[0] !== VERSION) {
    throw new Error("设备返回了不兼容的 BLE 协议状态");
  }
  return { result: bytes[1]!, wifi: bytes[2]! };
}

export function parseWifiCredentials(value: DataView | Uint8Array): WifiCredentials {
  const bytes = value instanceof DataView ? new Uint8Array(value.buffer, value.byteOffset, value.byteLength) : value;
  if (bytes.length < 3 || bytes[0] !== VERSION) throw new Error("设备返回了不兼容的 Wi‑Fi 配置");
  const ssidLength = bytes[1]!;
  const passwordLength = bytes[2]!;
  if (ssidLength > 32 || passwordLength > 64 || bytes.length !== 3 + ssidLength + passwordLength ||
      (ssidLength === 0 && passwordLength !== 0) || bytes.includes(0, 3)) {
    throw new Error("设备返回的 Wi‑Fi 配置无效");
  }
  try {
    return {
      ssid: decoder.decode(bytes.subarray(3, 3 + ssidLength)),
      password: decoder.decode(bytes.subarray(3 + ssidLength)),
    };
  } catch {
    throw new Error("设备返回的 Wi‑Fi 配置不是有效的 UTF‑8 文本");
  }
}

export async function readWifiCredentials(service: Pick<BluetoothRemoteGATTService, "getCharacteristic">): Promise<WifiCredentials | null> {
  let characteristic: BluetoothRemoteGATTCharacteristic;
  try {
    characteristic = await service.getCharacteristic(CREDENTIALS_UUID);
  } catch (error) {
    if (error instanceof Error && error.name === "NotFoundError") return null;
    throw error;
  }
  return parseWifiCredentials(await characteristic.readValue());
}

export function resultError(result: number): string {
  switch (result) {
    case RESULT.BAD_AUTH:
      return "OTA 密码错误";
    case RESULT.BAD_FORMAT:
      return "设备拒绝了无效的数据格式";
    case RESULT.BAD_SEQUENCE:
      return "BLE 数据包顺序或长度有误，请重试";
    case RESULT.UNAUTHORIZED:
      return "配网授权已失效，请重新输入 OTA 密码";
    case RESULT.BUSY:
      return "设备正忙，请稍后重试";
    default:
      return `设备返回未知错误（${result}）`;
  }
}

import { RESULT, VERSION } from "./constants.ts";

const decoder = new TextDecoder("utf-8", { fatal: true });

export interface DeviceStatus {
  result: number;
  wifi: number;
}

export interface WifiCredentials {
  ssid: string;
  password: string;
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

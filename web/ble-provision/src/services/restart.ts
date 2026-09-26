import { WIFI } from "../protocol/constants.ts";
import { sleep } from "./sleep.ts";
import type { BleSession } from "./ble-session.ts";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export async function checkAfterRestart(
  session: BleSession,
  signal: AbortSignal,
  onWifiStatus: (wifi: number) => void,
): Promise<void> {
  await sleep(2500, signal);
  if (session.connected) {
    session.disconnectGatt();
    await sleep(500, signal);
  }
  if (!session.available) throw new Error("设备已不可用");

  const deadline = Date.now() + 65000;
  let lastError: unknown;
  while (Date.now() < deadline) {
    signal.throwIfAborted();
    try {
      const status = session.canReadStatus
        ? await session.readStatus()
        : await session.connectGatt(signal);
      signal.throwIfAborted();
      if (status.wifi === WIFI.CONNECTED) return;
      if (status.wifi === WIFI.NOT_CONNECTED) {
        throw new Error("设备仍未连接 Wi‑Fi。请核对网络名称和密码后重新配置。");
      }
      onWifiStatus(status.wifi);
      await sleep(3000, signal);
    } catch (error) {
      if (signal.aborted) throw error;
      if (error instanceof Error && error.message.startsWith("设备仍未连接 Wi‑Fi")) throw error;
      lastError = error;
      await sleep(2500, signal);
    }
  }
  throw new Error(`等待 Wi‑Fi 连接超时，可重新配置${lastError ? `（${errorMessage(lastError)}）` : ""}`);
}

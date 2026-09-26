import { RESULT } from "../protocol/constants.ts";
import { createAuthFrame, createProvisionFrames, type Frame } from "../protocol/frames.ts";
import { resultError, type DeviceStatus } from "../protocol/status.ts";
import { BleSession } from "../services/ble-session.ts";
import { savedOtaPassword, saveOtaPassword } from "../services/ota-storage.ts";
import { DeviceResultError } from "../services/send-frame.ts";
import { ProvisionView, wifiDescription } from "../ui/view.ts";
import { canDisconnect, disconnectMessage, type Phase } from "./flow.ts";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export class ProvisionController {
  private readonly view: ProvisionView;
  private phase: Phase = "disconnected";
  private operation: AbortController | undefined;
  private authorized = false;
  private readonly session = new BleSession(
    (status) => this.onStatus(status),
    () => this.onDisconnected(),
    () => { if (this.phase !== "disconnected") this.view.showStatus("设备返回了不兼容的状态数据", "error"); },
  );

  constructor(view: ProvisionView) { this.view = view; }

  start(): void {
    this.view.bind({
      connect: () => { void this.connect(); },
      disconnect: () => this.disconnect(),
      auth: () => { void this.submitAuth(); },
      provision: () => { void this.submitProvision(); },
      another: () => this.startAnother(),
    });
    this.view.render(this.phase);
    if (!("bluetooth" in navigator)) {
      this.view.showStatus("当前浏览器不支持 Web Bluetooth。请在 HTTPS 或 localhost 页面使用 Chrome 或 Edge。", "error");
    }
  }

  private setPhase(next: Phase): void {
    this.phase = next;
    this.view.render(next);
  }

  private onStatus(status: DeviceStatus): void {
    if (this.session.connected) this.view.setDeviceState(`蓝牙已连接 · ${wifiDescription(status.wifi)}`);
    if (status.result === RESULT.UNAUTHORIZED) {
      this.authorized = false;
      this.view.clearWifiInputs();
      if (this.phase === "wifi" || this.phase === "done") {
        this.setPhase("auth");
        this.view.showStatus(resultError(status.result), "error");
      }
    }
  }

  private onDisconnected(): void {
    this.view.setDeviceState("蓝牙已断开");
    if (this.phase === "done") {
      return;
    }
    this.view.clearWifiInputs();
    this.operation?.abort();
    this.authorized = false;
    this.setPhase("disconnected");
    this.view.showStatus(disconnectMessage(), "error");
  }

  private async fillCurrentWifiCredentials(signal: AbortSignal): Promise<string> {
    this.view.clearWifiInputs();
    try {
      const credentials = await this.session.readCurrentWifiCredentials(signal);
      if (!credentials) return "未找到 Wi‑Fi 配置读取接口。请确认设备已更新固件并重启；若已更新，请清除蓝牙服务缓存后重连。也可手动填写。";
      this.view.setWifiCredentials(credentials.ssid, credentials.password);
      return credentials.ssid ? "已回填设备当前的 Wi‑Fi 信息。" : "设备尚未保存 Wi‑Fi 信息，请填写网络名称和密码。";
    } catch (error) {
      if (signal.aborted) throw error;
      return `无法读取当前 Wi‑Fi 信息（${errorMessage(error)}），请手动填写。`;
    }
  }

  private async authenticate(password: string, signal: AbortSignal): Promise<void> {
    const frame = createAuthFrame(password);
    try {
      await this.session.sendFrame(frame, 1, 1, signal);
      signal.throwIfAborted();
      this.authorized = true;
      saveOtaPassword(password);
    } finally {
      frame.bytes.fill(0);
    }
  }

  private async connect(): Promise<void> {
    if (!("bluetooth" in navigator)) {
      this.view.showStatus("当前浏览器不支持 Web Bluetooth。请在 HTTPS 或 localhost 页面使用 Chrome 或 Edge。", "error");
      return;
    }
    const controller = new AbortController();
    this.operation = controller;
    this.setPhase("connecting");
    this.view.showStatus("请在浏览器弹出的列表中选择设备…", "progress");
    try {
      this.view.clearWifiInputs();
      await this.session.selectDevice(controller.signal);
      this.view.setDeviceName(this.session.displayName);
      const status = await this.session.connectGatt(controller.signal);
      this.setPhase("auth");
      const stored = savedOtaPassword();
      if (stored) {
        this.setPhase("authenticating");
        this.view.showStatus("正在使用已保存的 OTA 密码验证…", "progress");
        try {
          await this.authenticate(stored, controller.signal);
          controller.signal.throwIfAborted();
          const credentialsMessage = await this.fillCurrentWifiCredentials(controller.signal);
          this.setPhase("wifi");
          this.view.showStatus(`已连接 ${this.session.name}。${wifiDescription(status.wifi)}。${credentialsMessage}`, "success");
          this.view.focusSsid();
        } catch (error) {
          if (controller.signal.aborted) throw error;
          this.setPhase("auth");
          this.view.showStatus(`自动验证失败：${errorMessage(error)}。请重新输入 OTA 密码。`, "error");
          this.view.focusOta();
        }
      } else {
        this.view.showStatus(`已连接 ${this.session.name}。请验证 OTA 密码。`, "success");
        this.view.focusOta();
      }
    } catch (error) {
      if (controller.signal.aborted) return;
      this.setPhase("disconnected");
      if (error instanceof Error && error.name === "NotFoundError") {
        this.view.showStatus("未选择设备。请靠近设备后重试。", "error");
      } else {
        this.view.showStatus(`连接失败：${errorMessage(error)}`, "error");
      }
    } finally {
      if (this.operation === controller) this.operation = undefined;
    }
  }

  private disconnect(): void {
    if (!canDisconnect(this.phase)) return;
    this.operation?.abort();
    this.operation = undefined;
    this.session.disconnect();
    this.authorized = false;
    this.view.clearWifiInputs();
    this.setPhase("disconnected");
    this.view.showStatus(disconnectMessage());
  }

  private startAnother(): void {
    if (this.phase !== "done") return;
    this.operation?.abort();
    this.operation = undefined;
    this.session.disconnect();
    this.authorized = false;
    this.view.clearWifiInputs();
    this.view.clearOtaPassword();
    this.view.setDeviceName("尚未选择设备");
    this.view.clearStatus();
    this.setPhase("disconnected");
    this.view.focusConnect();
  }

  private async submitAuth(): Promise<void> {
    if (this.phase !== "auth" || !this.session.connected) return;
    const controller = new AbortController();
    this.operation = controller;
    this.setPhase("authenticating");
    this.view.showStatus("正在验证 OTA 密码…", "progress");
    try {
      await this.authenticate(this.view.otaPassword, controller.signal);
      controller.signal.throwIfAborted();
      this.view.clearOtaPassword();
      const credentialsMessage = await this.fillCurrentWifiCredentials(controller.signal);
      this.setPhase("wifi");
      this.view.showStatus(`验证成功。${credentialsMessage}`, "success");
      this.view.focusSsid();
    } catch (error) {
      if (controller.signal.aborted) return;
      this.setPhase(this.session.connected ? "auth" : "disconnected");
      this.view.showStatus(`验证失败：${errorMessage(error)}`, "error");
    } finally {
      if (this.operation === controller) this.operation = undefined;
    }
  }

  private async submitProvision(): Promise<void> {
    if ((this.phase !== "wifi" && this.phase !== "done") || !this.session.connected) return;
    const controller = new AbortController();
    this.operation = controller;
    let frames: Frame[] = [];
    this.setPhase("submitting");
    try {
      frames = createProvisionFrames(this.view.ssid, this.view.wifiPassword);
      if (!this.authorized) {
        const stored = savedOtaPassword();
        if (!stored) throw new Error("请重新验证 OTA 密码");
        this.view.showStatus("正在重新验证 OTA 密码…", "progress");
        await this.authenticate(stored, controller.signal);
      }
      for (const [index, frame] of frames.entries()) {
        controller.signal.throwIfAborted();
        this.view.showStatus(`正在发送 Wi‑Fi 信息（${index + 1}/${frames.length}）…`, "progress");
        await this.session.sendFrame(frame, index + 1, frames.length, controller.signal);
      }
      controller.signal.throwIfAborted();
      this.authorized = false;
      this.setPhase("done");
      this.view.clearStatus();
      this.view.focusAnother();
    } catch (error) {
      if (controller.signal.aborted) return;
      if (error instanceof DeviceResultError && error.result === RESULT.UNAUTHORIZED) this.authorized = false;
      if (!this.session.connected) {
        this.setPhase("disconnected");
      } else if (error instanceof DeviceResultError && (error.result === RESULT.UNAUTHORIZED || error.result === RESULT.BAD_AUTH)) {
        this.setPhase("auth");
      } else {
        this.setPhase("wifi");
      }
      this.view.showStatus(`配网未完成：${errorMessage(error)}`, "error");
    } finally {
      for (const frame of frames) frame.bytes.fill(0);
      if (this.operation === controller) this.operation = undefined;
    }
  }
}

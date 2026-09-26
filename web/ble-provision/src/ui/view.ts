import { WIFI } from "../protocol/constants.ts";
import { canDisconnect, showAuthForm, showWifiForm, type Phase } from "../workflows/flow.ts";

function element<T extends HTMLElement>(selector: string): T {
  const found = document.querySelector<T>(selector);
  if (!found) throw new Error(`页面缺少元素：${selector}`);
  return found;
}

export function wifiDescription(wifi: number): string {
  switch (wifi) {
    case WIFI.NO_CREDENTIALS: return "尚未保存 Wi‑Fi";
    case WIFI.CONNECTING: return "正在连接 Wi‑Fi";
    case WIFI.CONNECTED: return "Wi‑Fi 已连接";
    case WIFI.NOT_CONNECTED: return "Wi‑Fi 尚未连接，设备仍在重试";
    default: return "Wi‑Fi 状态未知";
  }
}

export class ProvisionView {
  private readonly connectButton = element<HTMLButtonElement>("#connect-button");
  private readonly disconnectButton = element<HTMLButtonElement>("#disconnect-button");
  private readonly authButton = element<HTMLButtonElement>("#auth-button");
  private readonly submitButton = element<HTMLButtonElement>("#submit-button");
  private readonly authForm = element<HTMLFormElement>("#auth-form");
  private readonly provisionForm = element<HTMLFormElement>("#provision-form");
  private readonly authSection = element<HTMLElement>("#auth-section");
  private readonly wifiSection = element<HTMLElement>("#wifi-section");
  private readonly deviceName = element<HTMLElement>("#device-name");
  private readonly deviceState = element<HTMLElement>("#device-state");
  private readonly statusElement = element<HTMLElement>("#status");
  private readonly otaInput = element<HTMLInputElement>("#ota-password");
  private readonly ssidInput = element<HTMLInputElement>("#ssid");
  private readonly wifiInput = element<HTMLInputElement>("#wifi-password");

  bind(handlers: { connect: () => void; disconnect: () => void; auth: () => void; provision: () => void }): void {
    this.connectButton.addEventListener("click", handlers.connect);
    this.disconnectButton.addEventListener("click", handlers.disconnect);
    this.authForm.addEventListener("submit", (event) => { event.preventDefault(); handlers.auth(); });
    this.provisionForm.addEventListener("submit", (event) => { event.preventDefault(); handlers.provision(); });
  }

  render(phase: Phase): void {
    const connected = canDisconnect(phase);
    this.connectButton.hidden = connected;
    this.connectButton.disabled = phase === "connecting";
    this.disconnectButton.hidden = !connected;
    this.authSection.hidden = !showAuthForm(phase);
    this.wifiSection.hidden = !showWifiForm(phase);
    this.authButton.disabled = phase !== "auth";
    this.submitButton.disabled = phase !== "wifi" && phase !== "done";
    if (phase === "disconnected") this.deviceState.textContent = "蓝牙未连接";
  }

  showStatus(message: string, kind = ""): void {
    this.statusElement.hidden = false;
    this.statusElement.textContent = message;
    this.statusElement.dataset.kind = kind;
  }

  setDeviceName(name: string): void { this.deviceName.textContent = name; }
  setDeviceState(message: string): void { this.deviceState.textContent = message; }
  setWifiCredentials(ssid: string, password: string): void {
    this.ssidInput.value = ssid;
    this.wifiInput.value = password;
  }
  clearWifiInputs(): void { this.setWifiCredentials("", ""); }
  get otaPassword(): string { return this.otaInput.value; }
  clearOtaPassword(): void { this.otaInput.value = ""; }
  get ssid(): string { return this.ssidInput.value; }
  get wifiPassword(): string { return this.wifiInput.value; }
  focusOta(): void { this.otaInput.focus(); }
  focusSsid(): void { this.ssidInput.focus(); }
}

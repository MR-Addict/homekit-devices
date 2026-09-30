import { WIFI } from "../protocol/constants.ts";
import { canDisconnect, showAuthForm, showConnectPanel, showSuccessPanel, showWifiForm, type Phase } from "../workflows/flow.ts";

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
  private readonly anotherButton = element<HTMLButtonElement>("#another-button");
  private readonly toggleWifiPasswordButton = element<HTMLButtonElement>("#toggle-wifi-password");
  private readonly authForm = element<HTMLFormElement>("#auth-form");
  private readonly provisionForm = element<HTMLFormElement>("#provision-form");
  private readonly connectSection = element<HTMLElement>("#connect-section");
  private readonly authSection = element<HTMLElement>("#auth-section");
  private readonly wifiSection = element<HTMLElement>("#wifi-section");
  private readonly successSection = element<HTMLElement>("#success-section");
  private readonly deviceSummary = element<HTMLElement>("#device-summary");
  private readonly deviceName = element<HTMLElement>("#device-name");
  private readonly successDeviceName = element<HTMLElement>("#success-device-name");
  private readonly deviceState = element<HTMLElement>("#device-state");
  private readonly statusElement = element<HTMLElement>("#status");
  private readonly otaInput = element<HTMLInputElement>("#ota-password");
  private readonly ssidInput = element<HTMLInputElement>("#ssid");
  private readonly wifiInput = element<HTMLInputElement>("#wifi-password");

  bind(handlers: { connect: () => void; disconnect: () => void; auth: () => void; provision: () => void; another: () => void }): void {
    this.connectButton.addEventListener("click", handlers.connect);
    this.disconnectButton.addEventListener("click", handlers.disconnect);
    this.anotherButton.addEventListener("click", handlers.another);
    this.toggleWifiPasswordButton.addEventListener("click", () => this.toggleWifiPassword());
    this.authForm.addEventListener("submit", (event) => { event.preventDefault(); handlers.auth(); });
    this.provisionForm.addEventListener("submit", (event) => { event.preventDefault(); handlers.provision(); });
  }

  render(phase: Phase): void {
    const connected = canDisconnect(phase);
    this.connectSection.hidden = !showConnectPanel(phase);
    this.connectButton.disabled = phase === "connecting";
    this.connectButton.textContent = phase === "connecting" ? "正在连接…" : "搜索附近设备";
    this.deviceSummary.hidden = !connected;
    this.authSection.hidden = !showAuthForm(phase);
    this.wifiSection.hidden = !showWifiForm(phase);
    this.successSection.hidden = !showSuccessPanel(phase);
    this.authButton.disabled = phase !== "auth";
    this.submitButton.disabled = phase !== "wifi" && phase !== "done";
    this.authButton.textContent = phase === "authenticating" ? "正在验证…" : "验证密码";
    this.submitButton.textContent = phase === "submitting" ? "正在保存…" : "保存并连接";
    if (phase === "disconnected") this.deviceState.textContent = "蓝牙未连接";
  }

  showStatus(message: string, kind = ""): void {
    this.statusElement.hidden = false;
    this.statusElement.textContent = message;
    this.statusElement.dataset.kind = kind;
  }

  clearStatus(): void { this.statusElement.hidden = true; this.statusElement.textContent = ""; }
  setDeviceName(name: string): void { this.deviceName.textContent = name; this.successDeviceName.textContent = name; }
  setDeviceState(message: string): void { this.deviceState.textContent = message; }
  setWifiCredentials(ssid: string, password: string): void {
    this.ssidInput.value = ssid;
    this.wifiInput.value = password;
  }
  clearWifiInputs(): void {
    this.setWifiCredentials("", "");
    this.setWifiPasswordVisible(false);
  }
  get otaPassword(): string { return this.otaInput.value; }
  clearOtaPassword(): void { this.otaInput.value = ""; }
  get ssid(): string { return this.ssidInput.value; }
  get wifiPassword(): string { return this.wifiInput.value; }
  focusOta(): void { this.otaInput.focus(); }
  focusSsid(): void { this.ssidInput.focus(); }
  focusConnect(): void { this.connectButton.focus(); }
  focusAnother(): void { this.anotherButton.focus(); }

  private toggleWifiPassword(): void {
    this.setWifiPasswordVisible(this.wifiInput.type === "password");
    this.toggleWifiPasswordButton.focus();
  }

  private setWifiPasswordVisible(visible: boolean): void {
    this.wifiInput.type = visible ? "text" : "password";
    this.toggleWifiPasswordButton.textContent = visible ? "隐藏" : "显示";
    this.toggleWifiPasswordButton.setAttribute("aria-label", visible ? "隐藏网络密码" : "显示网络密码");
    this.toggleWifiPasswordButton.setAttribute("aria-pressed", String(visible));
  }
}

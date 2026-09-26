import "./style.css";
import {
  SERVICE_UUID,
  COMMAND_UUID,
  STATUS_UUID,
  RESULT,
  WIFI,
  createAuthFrame,
  createProvisionFrames,
  parseStatus,
  readWifiCredentials,
  resultError,
  type Frame,
  type DeviceStatus,
} from "./protocol.ts";
import { canDisconnect, disconnectMessage, showAuthForm, showWifiForm, type Phase } from "./flow.ts";

const OTA_STORAGE_KEY = "homekit-ble-provision:ota-password:v1";

function element<T extends HTMLElement>(selector: string): T {
  const found = document.querySelector<T>(selector);
  if (!found) throw new Error(`页面缺少元素：${selector}`);
  return found;
}

const connectButton = element<HTMLButtonElement>("#connect-button");
const disconnectButton = element<HTMLButtonElement>("#disconnect-button");
const authButton = element<HTMLButtonElement>("#auth-button");
const submitButton = element<HTMLButtonElement>("#submit-button");
const authForm = element<HTMLFormElement>("#auth-form");
const provisionForm = element<HTMLFormElement>("#provision-form");
const authSection = element<HTMLElement>("#auth-section");
const wifiSection = element<HTMLElement>("#wifi-section");
const deviceName = element<HTMLElement>("#device-name");
const deviceState = element<HTMLElement>("#device-state");
const statusElement = element<HTMLElement>("#status");
const otaInput = element<HTMLInputElement>("#ota-password");
const ssidInput = element<HTMLInputElement>("#ssid");
const wifiInput = element<HTMLInputElement>("#wifi-password");

let phase: Phase = "disconnected";
let device: BluetoothDevice | undefined;
let commandCharacteristic: BluetoothRemoteGATTCharacteristic | undefined;
let statusCharacteristic: BluetoothRemoteGATTCharacteristic | undefined;
let operation: AbortController | undefined;
let authorized = false;
let saved = false;
let confirmed = false;

class DeviceResultError extends Error {
  constructor(readonly result: number) {
    super(resultError(result));
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function showStatus(message: string, kind = ""): void {
  statusElement.hidden = false;
  statusElement.textContent = message;
  statusElement.dataset.kind = kind;
}

function wifiDescription(wifi: number): string {
  switch (wifi) {
    case WIFI.NO_CREDENTIALS: return "尚未保存 Wi‑Fi";
    case WIFI.CONNECTING: return "正在连接 Wi‑Fi";
    case WIFI.CONNECTED: return "Wi‑Fi 已连接";
    case WIFI.NOT_CONNECTED: return "Wi‑Fi 尚未连接，设备仍在重试";
    default: return "Wi‑Fi 状态未知";
  }
}

function render(): void {
  const connected = canDisconnect(phase);
  connectButton.hidden = connected;
  connectButton.disabled = phase === "connecting";
  disconnectButton.hidden = !connected;
  authSection.hidden = !showAuthForm(phase);
  wifiSection.hidden = !showWifiForm(phase);
  authButton.disabled = phase !== "auth";
  submitButton.disabled = phase !== "wifi" && phase !== "done";
  if (phase === "disconnected") deviceState.textContent = "蓝牙未连接";
}

function setPhase(next: Phase): void {
  phase = next;
  render();
}

function savedOtaPassword(): string | null {
  try { return localStorage.getItem(OTA_STORAGE_KEY); }
  catch { return null; }
}

function saveOtaPassword(password: string): void {
  try { localStorage.setItem(OTA_STORAGE_KEY, password); }
  catch { /* Storage may be unavailable; the current session can continue. */ }
}

function clearWifiInputs(): void {
  ssidInput.value = "";
  wifiInput.value = "";
}

async function fillCurrentWifiCredentials(signal: AbortSignal): Promise<string> {
  clearWifiInputs();
  if (!device?.gatt?.connected || !commandCharacteristic) throw new Error("蓝牙连接已断开");
  try {
    const service = await device.gatt.getPrimaryService(SERVICE_UUID);
    signal.throwIfAborted();
    const credentials = await readWifiCredentials(service);
    signal.throwIfAborted();
    if (!credentials) return "设备固件不支持读取当前 Wi‑Fi 信息，请手动填写。";
    ssidInput.value = credentials.ssid;
    wifiInput.value = credentials.password;
    return credentials.ssid ? "已回填设备当前的 Wi‑Fi 信息。" : "设备尚未保存 Wi‑Fi 信息，请填写网络名称和密码。";
  } catch (error) {
    if (signal.aborted) throw error;
    return `无法读取当前 Wi‑Fi 信息（${errorMessage(error)}），请手动填写。`;
  }
}

function clearCharacteristics(): void {
  statusCharacteristic?.removeEventListener("characteristicvaluechanged", onStatusChanged);
  commandCharacteristic = undefined;
  statusCharacteristic = undefined;
  authorized = false;
}

function onDisconnected(): void {
  clearCharacteristics();
  clearWifiInputs();
  if (phase === "checking") {
    deviceState.textContent = "设备重启中，等待蓝牙重连";
    return;
  }
  operation?.abort();
  setPhase("disconnected");
  showStatus(disconnectMessage(saved, confirmed), saved && !confirmed ? "progress" : "error");
}

async function connectGatt(signal: AbortSignal): Promise<DeviceStatus> {
  if (!device?.gatt) throw new Error("请先选择设备");
  clearCharacteristics();
  const server = await device.gatt.connect();
  signal.throwIfAborted();
  const service = await server.getPrimaryService(SERVICE_UUID);
  signal.throwIfAborted();
  const command = await service.getCharacteristic(COMMAND_UUID);
  const status = await service.getCharacteristic(STATUS_UUID);
  signal.throwIfAborted();
  commandCharacteristic = command;
  statusCharacteristic = status;
  status.addEventListener("characteristicvaluechanged", onStatusChanged);
  await status.startNotifications();
  signal.throwIfAborted();
  const current = parseStatus(await status.readValue());
  signal.throwIfAborted();
  deviceState.textContent = `蓝牙已连接 · ${wifiDescription(current.wifi)}`;
  return current;
}

function onStatusChanged(event: Event): void {
  try {
    const value = (event.target as BluetoothRemoteGATTCharacteristic).value;
    if (!value) return;
    const status = parseStatus(value);
    if (device?.gatt?.connected) deviceState.textContent = `蓝牙已连接 · ${wifiDescription(status.wifi)}`;
    if (status.result === RESULT.SAVED && phase === "submitting") saved = true;
    if (status.result === RESULT.UNAUTHORIZED && phase !== "checking") {
      authorized = false;
      clearWifiInputs();
      if (phase === "wifi" || phase === "done") {
        setPhase("auth");
        showStatus(resultError(status.result), "error");
      }
    }
  } catch {
    if (phase !== "disconnected") showStatus("设备返回了不兼容的状态数据", "error");
  }
}

function sendFrame(frame: Frame, number: number, total: number, signal: AbortSignal): Promise<void> {
  const replyCharacteristic = statusCharacteristic;
  const requestCharacteristic = commandCharacteristic;
  if (!replyCharacteristic || !requestCharacteristic) return Promise.reject(new Error("蓝牙连接已断开"));
  signal.throwIfAborted();
  return new Promise<void>((resolve, reject) => {
    let finished = false;
    let writeDone = false;
    let replyDone = false;
    let replyError: Error | undefined;
    const finish = (error?: Error) => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      replyCharacteristic.removeEventListener("characteristicvaluechanged", onReply);
      if (error) reject(error);
      else resolve();
    };
    const finishIfComplete = () => {
      if (writeDone && replyDone) finish(replyError);
    };
    const onReply = (event: Event) => {
      try {
        const value = (event.target as BluetoothRemoteGATTCharacteristic).value;
        if (!value) throw new Error("设备返回了空状态");
        const status = parseStatus(value);
        if (status.result >= 0x80) {
          replyError = new DeviceResultError(status.result);
          replyDone = true;
          finishIfComplete();
        } else if (status.result === frame.expected) {
          replyDone = true;
          finishIfComplete();
        }
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)));
      }
    };
    const onAbort = () => finish(new DOMException("操作已取消", "AbortError"));
    const timer = setTimeout(() => finish(new Error(`第 ${number}/${total} 个数据包等待设备响应超时`)), 8000);
    signal.addEventListener("abort", onAbort, { once: true });
    replyCharacteristic.addEventListener("characteristicvaluechanged", onReply);
    requestCharacteristic.writeValueWithResponse(new Uint8Array(frame.bytes)).then(
      () => { writeDone = true; finishIfComplete(); },
      (error: unknown) => finish(error instanceof Error ? error : new Error(String(error))),
    );
  });
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException("操作已取消", "AbortError"));
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

async function authenticate(password: string, signal: AbortSignal): Promise<void> {
  const frame = createAuthFrame(password);
  try {
    await sendFrame(frame, 1, 1, signal);
    signal.throwIfAborted();
    authorized = true;
    saveOtaPassword(password);
  } finally {
    frame.bytes.fill(0);
  }
}

async function checkAfterRestart(signal: AbortSignal): Promise<void> {
  showStatus("信息已保存，等待设备重启并连接 Wi‑Fi…", "progress");
  await sleep(2500, signal);
  if (device?.gatt?.connected) {
    device.gatt.disconnect();
    await sleep(500, signal);
  }
  if (!device?.gatt) throw new Error("设备已不可用");

  const deadline = Date.now() + 65000;
  let lastError: unknown;
  while (Date.now() < deadline) {
    signal.throwIfAborted();
    try {
      const status = device.gatt.connected && statusCharacteristic
        ? parseStatus(await statusCharacteristic.readValue())
        : await connectGatt(signal);
      signal.throwIfAborted();
      if (status.wifi === WIFI.CONNECTED) return;
      if (status.wifi === WIFI.NOT_CONNECTED) {
        throw new Error("设备仍未连接 Wi‑Fi。请核对网络名称和密码后重新配置。");
      }
      showStatus(`设备已重启，${wifiDescription(status.wifi)}…`, "progress");
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

connectButton.addEventListener("click", async () => {
  if (!("bluetooth" in navigator)) {
    showStatus("当前浏览器不支持 Web Bluetooth。请在 HTTPS 或 localhost 页面使用 Chrome 或 Edge。", "error");
    return;
  }
  const controller = new AbortController();
  operation = controller;
  setPhase("connecting");
  showStatus("请在浏览器弹出的列表中选择设备…", "progress");
  try {
    clearWifiInputs();
    if (device) {
      device.removeEventListener("gattserverdisconnected", onDisconnected);
      if (device.gatt?.connected) device.gatt.disconnect();
    }
    device = await navigator.bluetooth.requestDevice({ filters: [{ services: [SERVICE_UUID] }] });
    controller.signal.throwIfAborted();
    device.addEventListener("gattserverdisconnected", onDisconnected);
    deviceName.textContent = device.name || "未命名设备";
    saved = false;
    confirmed = false;
    const status = await connectGatt(controller.signal);
    setPhase("auth");
    const stored = savedOtaPassword();
    if (stored) {
      setPhase("authenticating");
      showStatus("正在使用已保存的 OTA 密码验证…", "progress");
      try {
        await authenticate(stored, controller.signal);
        controller.signal.throwIfAborted();
        const credentialsMessage = await fillCurrentWifiCredentials(controller.signal);
        setPhase("wifi");
        showStatus(`已连接 ${device.name || "设备"}。${wifiDescription(status.wifi)}。${credentialsMessage}`, "success");
        ssidInput.focus();
      } catch (error) {
        if (controller.signal.aborted) throw error;
        setPhase("auth");
        showStatus(`自动验证失败：${errorMessage(error)}。请重新输入 OTA 密码。`, "error");
        otaInput.focus();
      }
    } else {
      showStatus(`已连接 ${device.name || "设备"}。请验证 OTA 密码。`, "success");
      otaInput.focus();
    }
  } catch (error) {
    if (controller.signal.aborted) return;
    setPhase("disconnected");
    if (error instanceof Error && error.name === "NotFoundError") {
      showStatus("未选择设备。请靠近设备后重试。", "error");
    } else {
      showStatus(`连接失败：${errorMessage(error)}`, "error");
    }
  } finally {
    if (operation === controller) operation = undefined;
  }
});

disconnectButton.addEventListener("click", () => {
  if (!canDisconnect(phase)) return;
  operation?.abort();
  operation = undefined;
  const oldDevice = device;
  oldDevice?.removeEventListener("gattserverdisconnected", onDisconnected);
  clearCharacteristics();
  clearWifiInputs();
  device = undefined;
  setPhase("disconnected");
  showStatus(disconnectMessage(saved, confirmed), saved && !confirmed ? "progress" : "");
  if (oldDevice?.gatt?.connected) oldDevice.gatt.disconnect();
});

authForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (phase !== "auth" || !device?.gatt?.connected) return;
  const controller = new AbortController();
  operation = controller;
  setPhase("authenticating");
  showStatus("正在验证 OTA 密码…", "progress");
  try {
    await authenticate(otaInput.value, controller.signal);
    controller.signal.throwIfAborted();
    otaInput.value = "";
    const credentialsMessage = await fillCurrentWifiCredentials(controller.signal);
    setPhase("wifi");
    showStatus(`验证成功。${credentialsMessage}`, "success");
    ssidInput.focus();
  } catch (error) {
    if (controller.signal.aborted) return;
    setPhase(device?.gatt?.connected ? "auth" : "disconnected");
    showStatus(`验证失败：${errorMessage(error)}`, "error");
  } finally {
    if (operation === controller) operation = undefined;
  }
});

provisionForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  if ((phase !== "wifi" && phase !== "done") || !device?.gatt?.connected) return;
  const controller = new AbortController();
  operation = controller;
  let frames: Frame[] = [];
  saved = false;
  confirmed = false;
  setPhase("submitting");
  try {
    frames = createProvisionFrames(ssidInput.value, wifiInput.value);
    if (!authorized) {
      const stored = savedOtaPassword();
      if (!stored) throw new Error("请重新验证 OTA 密码");
      showStatus("正在重新验证 OTA 密码…", "progress");
      await authenticate(stored, controller.signal);
    }
    for (const [index, frame] of frames.entries()) {
      controller.signal.throwIfAborted();
      showStatus(`正在发送 Wi‑Fi 信息（${index + 1}/${frames.length}）…`, "progress");
      await sendFrame(frame, index + 1, frames.length, controller.signal);
      if (frame.expected === RESULT.SAVED) saved = true;
    }
    controller.signal.throwIfAborted();
    authorized = false;
    setPhase("checking");
    await checkAfterRestart(controller.signal);
    controller.signal.throwIfAborted();
    confirmed = true;
    setPhase("done");
    showStatus("配网成功，设备已连接 Wi‑Fi。", "success");
  } catch (error) {
    if (controller.signal.aborted) return;
    if (error instanceof DeviceResultError && error.result === RESULT.UNAUTHORIZED) authorized = false;
    if (!device?.gatt?.connected) {
      setPhase("disconnected");
    } else if (error instanceof DeviceResultError && error.result === RESULT.UNAUTHORIZED) {
      setPhase("auth");
    } else if (error instanceof DeviceResultError && error.result === RESULT.BAD_AUTH) {
      setPhase("auth");
    } else {
      setPhase("wifi");
    }
    showStatus(`配网未完成：${errorMessage(error)}`, "error");
  } finally {
    for (const frame of frames) frame.bytes.fill(0);
    if (operation === controller) operation = undefined;
  }
});

render();
if (!("bluetooth" in navigator)) {
  showStatus("当前浏览器不支持 Web Bluetooth。请在 HTTPS 或 localhost 页面使用 Chrome 或 Edge。", "error");
}

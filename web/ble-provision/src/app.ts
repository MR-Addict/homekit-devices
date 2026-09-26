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
  resultError,
  type Frame,
  type DeviceStatus,
} from "./protocol.ts";
import { nextStep, type Step, type FlowEvent } from "./flow.ts";

function element<T extends HTMLElement>(selector: string): T {
  const found = document.querySelector<T>(selector);
  if (!found) throw new Error(`页面缺少元素：${selector}`);
  return found;
}

const connectButton = element<HTMLButtonElement>("#connect-button");
const authButton = element<HTMLButtonElement>("#auth-button");
const submitButton = element<HTMLButtonElement>("#submit-button");
const retryButton = element<HTMLButtonElement>("#retry-button");
const authForm = element<HTMLFormElement>("#auth-form");
const provisionForm = element<HTMLFormElement>("#provision-form");
const deviceName = element<HTMLElement>("#device-name");
const deviceState = element<HTMLElement>("#device-state");
const authDeviceDescription = element<HTMLElement>("#auth-device-description");
const statusElement = element<HTMLElement>("#status");
const otaInput = element<HTMLInputElement>("#ota-password");
const ssidInput = element<HTMLInputElement>("#ssid");
const wifiInput = element<HTMLInputElement>("#wifi-password");
const panels = [...document.querySelectorAll<HTMLElement>("[data-step-panel]")];
const indicators = [...document.querySelectorAll<HTMLElement>("[data-step-indicator]")];

let step: Step = "connect";
let device: BluetoothDevice | undefined;
let commandCharacteristic: BluetoothRemoteGATTCharacteristic | undefined;
let statusCharacteristic: BluetoothRemoteGATTCharacteristic | undefined;
let busy = false;
let checkingAfterRestart = false;

class DeviceResultError extends Error {
  constructor(readonly result: number) {
    super(resultError(result));
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function showStatus(message: string, kind = ""): void {
  statusElement.textContent = message;
  statusElement.dataset.kind = kind;
}

function wifiDescription(wifi: number): string {
  switch (wifi) {
    case WIFI.NO_CREDENTIALS:
      return "尚未保存 Wi‑Fi";
    case WIFI.CONNECTING:
      return "正在连接 Wi‑Fi";
    case WIFI.CONNECTED:
      return "Wi‑Fi 已连接";
    case WIFI.NOT_CONNECTED:
      return "Wi‑Fi 尚未连接，设备仍在重试";
    default:
      return "Wi‑Fi 状态未知";
  }
}

function updateButtons(): void {
  connectButton.disabled = busy;
  authButton.disabled = busy || !device?.gatt?.connected;
  submitButton.disabled = busy || !device?.gatt?.connected;
  retryButton.disabled = busy;
}

function renderStep(): void {
  for (const panel of panels) panel.hidden = panel.dataset.stepPanel !== step;
  for (const indicator of indicators) {
    indicator.setAttribute("aria-current", indicator.dataset.stepIndicator === step ? "step" : "false");
  }
  updateButtons();
}

function transition(event: FlowEvent): void {
  step = nextStep(step, event);
  renderStep();
}

function onDisconnected(): void {
  commandCharacteristic = undefined;
  statusCharacteristic = undefined;
  deviceState.textContent = "蓝牙已断开";
  if (!checkingAfterRestart) {
    transition("disconnected");
    showStatus("连接已断开。请重新搜索设备。", "error");
  }
  updateButtons();
}

async function connectGatt(): Promise<DeviceStatus> {
  if (!device?.gatt) throw new Error("请先选择设备");
  statusCharacteristic?.removeEventListener("characteristicvaluechanged", onStatusChanged);
  const server = await device.gatt.connect();
  const service = await server.getPrimaryService(SERVICE_UUID);
  commandCharacteristic = await service.getCharacteristic(COMMAND_UUID);
  statusCharacteristic = await service.getCharacteristic(STATUS_UUID);
  statusCharacteristic.addEventListener("characteristicvaluechanged", onStatusChanged);
  await statusCharacteristic.startNotifications();
  const status = parseStatus(await statusCharacteristic.readValue());
  deviceState.textContent = `蓝牙已连接 · ${wifiDescription(status.wifi)}`;
  updateButtons();
  return status;
}

function onStatusChanged(event: Event): void {
  try {
    const value = (event.target as BluetoothRemoteGATTCharacteristic).value;
    if (!value) return;
    const status = parseStatus(value);
    deviceState.textContent = `蓝牙已连接 · ${wifiDescription(status.wifi)}`;
    if (status.result === RESULT.UNAUTHORIZED && !checkingAfterRestart) {
      transition("unauthorized");
      showStatus(resultError(status.result), "error");
    }
  } catch {
    showStatus("设备返回了不兼容的状态数据", "error");
  }
}

function sendFrame(frame: Frame, number: number, total: number): Promise<void> {
  const replyCharacteristic = statusCharacteristic;
  const requestCharacteristic = commandCharacteristic;
  if (!replyCharacteristic || !requestCharacteristic) {
    return Promise.reject(new Error("蓝牙连接已断开"));
  }
  return new Promise<void>((resolve, reject) => {
    let finished = false;
    let writeDone = false;
    let replyDone = false;
    let replyError: Error | undefined;
    const finish = (error?: Error) => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
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
    const timer = setTimeout(() => finish(new Error(`第 ${number}/${total} 个数据包等待设备响应超时`)), 8000);
    replyCharacteristic.addEventListener("characteristicvaluechanged", onReply);
    requestCharacteristic.writeValueWithResponse(new Uint8Array(frame.bytes)).then(
      () => {
        writeDone = true;
        finishIfComplete();
      },
      (error: unknown) => finish(error instanceof Error ? error : new Error(String(error))),
    );
  });
}

const sleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

async function checkAfterRestart(): Promise<void> {
  checkingAfterRestart = true;
  showStatus("凭据已保存，等待设备重启并连接 Wi‑Fi…", "progress");
  await sleep(2500);
  if (device?.gatt?.connected) {
    device.gatt.disconnect();
    await sleep(500);
  }
  if (!device?.gatt) throw new Error("设备已不可用");

  const deadline = Date.now() + 65000;
  let lastError: unknown;
  while (Date.now() < deadline) {
    try {
      const status =
        device.gatt.connected && statusCharacteristic
          ? parseStatus(await statusCharacteristic.readValue())
          : await connectGatt();
      if (status.wifi === WIFI.CONNECTED) {
        showStatus("配网成功，设备已连接 Wi‑Fi。", "success");
        return;
      }
      if (status.wifi === WIFI.NOT_CONNECTED) {
        throw new Error("设备仍未连接 Wi‑Fi。请核对网络名称和密码后重新配置。");
      }
      showStatus(`设备已重启，${wifiDescription(status.wifi)}…`, "progress");
      await sleep(3000);
    } catch (error) {
      if (error instanceof Error && error.message.startsWith("设备仍未连接 Wi‑Fi")) throw error;
      lastError = error;
      await sleep(2500);
    }
  }
  throw new Error(`等待 Wi‑Fi 连接超时，可重新配置${lastError ? `（${errorMessage(lastError)}）` : ""}`);
}

connectButton.addEventListener("click", async () => {
  if (!("bluetooth" in navigator)) {
    showStatus("当前浏览器不支持 Web Bluetooth。请在 HTTPS 或 localhost 页面使用 Chrome 或 Edge。", "error");
    return;
  }
  try {
    busy = true;
    updateButtons();
    showStatus("请在浏览器弹出的列表中选择设备…", "progress");
    if (device) {
      device.removeEventListener("gattserverdisconnected", onDisconnected);
      if (device.gatt?.connected) device.gatt.disconnect();
    }
    device = await navigator.bluetooth.requestDevice({ filters: [{ services: [SERVICE_UUID] }] });
    device.addEventListener("gattserverdisconnected", onDisconnected);
    deviceName.textContent = device.name || "未命名设备";
    authDeviceDescription.textContent = device.name || "设备";
    const status = await connectGatt();
    transition("connected");
    showStatus(`已连接 ${device.name || "设备"}。${wifiDescription(status.wifi)}。请输入 OTA 密码。`, "success");
    otaInput.focus();
  } catch (error) {
    if (error instanceof Error && error.name === "NotFoundError") {
      showStatus("未选择设备。请靠近设备后重试。", "error");
    } else {
      showStatus(`连接失败：${errorMessage(error)}`, "error");
    }
  } finally {
    busy = false;
    updateButtons();
  }
});

authForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (busy || step !== "auth" || !device?.gatt?.connected) return;
  let frame: Frame | undefined;
  try {
    frame = createAuthFrame(otaInput.value);
    busy = true;
    updateButtons();
    showStatus("正在验证 OTA 密码…", "progress");
    await sendFrame(frame, 1, 1);
    transition("authenticated");
    showStatus("密码已验证。请填写 Wi‑Fi 信息。", "success");
    ssidInput.focus();
  } catch (error) {
    showStatus(`验证失败：${errorMessage(error)}`, "error");
    if (!device?.gatt?.connected) transition("disconnected");
  } finally {
    frame?.bytes.fill(0);
    otaInput.value = "";
    busy = false;
    updateButtons();
  }
});

provisionForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (busy || step !== "wifi" || !device?.gatt?.connected) return;
  let frames: Frame[] = [];
  let saved = false;
  try {
    frames = createProvisionFrames(ssidInput.value, wifiInput.value);
    busy = true;
    transition("submitting");
    for (const [index, frame] of frames.entries()) {
      showStatus(`正在向设备发送配网信息（${index + 1}/${frames.length}）…`, "progress");
      await sendFrame(frame, index + 1, frames.length);
    }
    saved = true;
    ssidInput.value = "";
    wifiInput.value = "";
    await checkAfterRestart();
  } catch (error) {
    if (error instanceof DeviceResultError && error.result === RESULT.UNAUTHORIZED) {
      transition("unauthorized");
    } else if (!saved && !device?.gatt?.connected) {
      transition("disconnected");
    } else if (!saved) {
      step = "wifi";
      renderStep();
    }
    showStatus(`配网未完成：${errorMessage(error)}`, "error");
  } finally {
    checkingAfterRestart = false;
    for (const frame of frames) frame.bytes.fill(0);
    busy = false;
    updateButtons();
  }
});

retryButton.addEventListener("click", () => {
  if (device?.gatt?.connected) {
    transition("retry");
    showStatus("请重新输入 OTA 密码，然后填写 Wi‑Fi 信息。", "");
    otaInput.focus();
  } else {
    transition("disconnected");
    showStatus("请重新连接设备。", "");
  }
});

renderStep();
if (!("bluetooth" in navigator)) {
  showStatus("当前浏览器不支持 Web Bluetooth。请在 HTTPS 或 localhost 页面使用 Chrome 或 Edge。", "error");
}

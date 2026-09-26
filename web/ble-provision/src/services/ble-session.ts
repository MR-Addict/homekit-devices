import { COMMAND_UUID, SERVICE_UUID, STATUS_UUID } from "../protocol/constants.ts";
import { parseStatus, type DeviceStatus, type WifiCredentials } from "../protocol/status.ts";
import type { Frame } from "../protocol/frames.ts";
import { readWifiCredentials } from "./credentials.ts";
import { sendFrame } from "./send-frame.ts";

export class BleSession {
  private device: BluetoothDevice | undefined;
  private command: BluetoothRemoteGATTCharacteristic | undefined;
  private status: BluetoothRemoteGATTCharacteristic | undefined;
  private readonly onStatus: (status: DeviceStatus) => void;
  private readonly onDisconnected: () => void;
  private readonly onInvalidStatus: () => void;

  constructor(
    onStatus: (status: DeviceStatus) => void,
    onDisconnected: () => void,
    onInvalidStatus: () => void,
  ) {
    this.onStatus = onStatus;
    this.onDisconnected = onDisconnected;
    this.onInvalidStatus = onInvalidStatus;
  }

  get connected(): boolean { return !!this.device?.gatt?.connected; }
  get name(): string { return this.device?.name || "设备"; }
  get displayName(): string { return this.device?.name || "未命名设备"; }

  async selectDevice(signal: AbortSignal): Promise<void> {
    if (this.device) {
      this.device.removeEventListener("gattserverdisconnected", this.handleDisconnected);
      if (this.device.gatt?.connected) this.device.gatt.disconnect();
    }
    this.device = await navigator.bluetooth.requestDevice({ filters: [{ services: [SERVICE_UUID] }] });
    signal.throwIfAborted();
    this.device.addEventListener("gattserverdisconnected", this.handleDisconnected);
  }

  async connectGatt(signal: AbortSignal): Promise<DeviceStatus> {
    if (!this.device?.gatt) throw new Error("请先选择设备");
    this.clearCharacteristics();
    const server = await this.device.gatt.connect();
    signal.throwIfAborted();
    const service = await server.getPrimaryService(SERVICE_UUID);
    signal.throwIfAborted();
    const command = await service.getCharacteristic(COMMAND_UUID);
    const status = await service.getCharacteristic(STATUS_UUID);
    signal.throwIfAborted();
    this.command = command;
    this.status = status;
    status.addEventListener("characteristicvaluechanged", this.handleStatusChanged);
    await status.startNotifications();
    signal.throwIfAborted();
    const current = parseStatus(await status.readValue());
    signal.throwIfAborted();
    this.onStatus(current);
    return current;
  }

  async readCurrentWifiCredentials(signal: AbortSignal): Promise<WifiCredentials | null> {
    if (!this.device?.gatt?.connected || !this.command) throw new Error("蓝牙连接已断开");
    const service = await this.device.gatt.getPrimaryService(SERVICE_UUID);
    signal.throwIfAborted();
    const credentials = await readWifiCredentials(service);
    signal.throwIfAborted();
    return credentials;
  }

  sendFrame(frame: Frame, number: number, total: number, signal: AbortSignal): Promise<void> {
    if (!this.command || !this.status) return Promise.reject(new Error("蓝牙连接已断开"));
    return sendFrame(this.command, this.status, frame, number, total, signal);
  }

  disconnect(): void {
    const oldDevice = this.device;
    oldDevice?.removeEventListener("gattserverdisconnected", this.handleDisconnected);
    this.clearCharacteristics();
    this.device = undefined;
    if (oldDevice?.gatt?.connected) oldDevice.gatt.disconnect();
  }

  private clearCharacteristics(): void {
    this.status?.removeEventListener("characteristicvaluechanged", this.handleStatusChanged);
    this.command = undefined;
    this.status = undefined;
  }

  private readonly handleDisconnected = (): void => {
    this.clearCharacteristics();
    this.onDisconnected();
  };

  private readonly handleStatusChanged = (event: Event): void => {
    try {
      const value = (event.target as BluetoothRemoteGATTCharacteristic).value;
      if (value) this.onStatus(parseStatus(value));
    } catch {
      this.onInvalidStatus();
    }
  };
}

export type Phase = "disconnected" | "connecting" | "auth" | "authenticating" | "wifi" | "submitting" | "checking" | "done";

export function canDisconnect(phase: Phase): boolean {
  return phase !== "disconnected" && phase !== "connecting";
}

export function showAuthForm(phase: Phase): boolean {
  return phase === "auth" || phase === "authenticating";
}

export function showWifiForm(phase: Phase): boolean {
  return phase === "wifi" || phase === "submitting" || phase === "checking";
}

export function showConnectPanel(phase: Phase): boolean {
  return phase === "disconnected" || phase === "connecting";
}

export function showSuccessPanel(phase: Phase): boolean {
  return phase === "done";
}

export function currentStep(phase: Phase): number {
  if (showConnectPanel(phase)) return 1;
  if (showAuthForm(phase)) return 2;
  return 3;
}

export function disconnectMessage(saved: boolean, confirmed: boolean): string {
  if (confirmed) return "配网成功，蓝牙已断开。";
  if (saved) return "设备已保存 Wi‑Fi 信息，但连接结果尚未核实。";
  return "蓝牙已断开，可重新搜索设备。";
}

export type Phase = "disconnected" | "connecting" | "auth" | "authenticating" | "wifi" | "submitting" | "done";

export function canDisconnect(phase: Phase): boolean {
  return phase !== "disconnected" && phase !== "connecting";
}

export function showAuthForm(phase: Phase): boolean {
  return phase === "auth" || phase === "authenticating";
}

export function showWifiForm(phase: Phase): boolean {
  return phase === "wifi" || phase === "submitting";
}

export function showConnectPanel(phase: Phase): boolean {
  return phase === "disconnected" || phase === "connecting";
}

export function showSuccessPanel(phase: Phase): boolean {
  return phase === "done";
}

export function disconnectMessage(): string {
  return "蓝牙已断开，可重新搜索设备。";
}

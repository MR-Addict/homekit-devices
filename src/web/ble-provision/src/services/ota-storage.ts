const OTA_STORAGE_KEY = "homekit-ble-provision:ota-password:v1";

export function savedOtaPassword(): string | null {
  try { return localStorage.getItem(OTA_STORAGE_KEY); }
  catch { return null; }
}

export function saveOtaPassword(password: string): void {
  try { localStorage.setItem(OTA_STORAGE_KEY, password); }
  catch { /* Storage may be unavailable; the current session can continue. */ }
}

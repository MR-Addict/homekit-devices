import { CREDENTIALS_UUID } from "../protocol/constants.ts";
import { parseWifiCredentials, type WifiCredentials } from "../protocol/status.ts";

export async function readWifiCredentials(service: Pick<BluetoothRemoteGATTService, "getCharacteristic">): Promise<WifiCredentials | null> {
  let characteristic: BluetoothRemoteGATTCharacteristic;
  try {
    characteristic = await service.getCharacteristic(CREDENTIALS_UUID);
  } catch (error) {
    if (error instanceof Error && error.name === "NotFoundError") return null;
    throw error;
  }
  return parseWifiCredentials(await characteristic.readValue());
}

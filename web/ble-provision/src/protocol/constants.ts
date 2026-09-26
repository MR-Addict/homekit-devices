export const SERVICE_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c832";
export const COMMAND_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c833";
export const STATUS_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c834";
export const CREDENTIALS_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c835";
export const VERSION = 1;

export const RESULT = {
  READY: 0,
  AUTH_OK: 1,
  BEGIN_OK: 2,
  DATA_OK: 3,
  SAVED: 4,
  BAD_AUTH: 0x80,
  BAD_FORMAT: 0x81,
  BAD_SEQUENCE: 0x82,
  UNAUTHORIZED: 0x83,
  BUSY: 0x84,
} as const;

export const WIFI = {
  NO_CREDENTIALS: 0,
  CONNECTING: 1,
  CONNECTED: 2,
  NOT_CONNECTED: 3,
} as const;


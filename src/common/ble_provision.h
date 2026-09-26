#pragma once

#include <HomeSpan.h>
#include <NimBLEDevice.h>
#include <WiFi.h>
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/task.h>
#include <atomic>
#include <mutex>

static constexpr char HS_OTA_PASSWORD[] = "ota";

// BLE 配网协议 v1。网页端的 UUID、操作码和状态码必须与此处一致。
class HSBleProvision {
  static constexpr const char *SERVICE_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c832";
  static constexpr const char *COMMAND_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c833";
  static constexpr const char *STATUS_UUID = "8f4c69c0-8c56-4bd9-9862-c45913a1c834";
  static constexpr uint8_t PROTOCOL_VERSION = 1;
  static constexpr size_t MAX_FRAME = 20;
  static constexpr size_t MAX_SSID_BYTES = 32;
  static constexpr size_t MAX_PASSWORD_BYTES = 64;
  static constexpr uint32_t SESSION_TIMEOUT_MS = 120000;
  static constexpr uint32_t WIFI_TIMEOUT_MS = 45000;

  enum EventType : uint8_t { CONNECT, DISCONNECT, FRAME };
  enum Opcode : uint8_t { AUTH = 1, BEGIN = 2, DATA = 3, COMMIT = 4 };
  enum Result : uint8_t {
    READY = 0,
    AUTH_OK = 1,
    BEGIN_OK = 2,
    DATA_OK = 3,
    SAVED = 4,
    BAD_AUTH = 0x80,
    BAD_FORMAT = 0x81,
    BAD_SEQUENCE = 0x82,
    UNAUTHORIZED = 0x83,
    BUSY = 0x84
  };
  enum WifiState : uint8_t { NO_CREDENTIALS, CONNECTING, CONNECTED, NOT_CONNECTED };

  struct Event {
    EventType type;
    uint16_t connection;
    uint8_t length;
    uint8_t bytes[MAX_FRAME];
  };

  class ServerCallbacks : public NimBLEServerCallbacks {
    HSBleProvision &owner;

  public:
    explicit ServerCallbacks(HSBleProvision &owner) : owner(owner) {}
    void onConnect(NimBLEServer *, NimBLEConnInfo &info) override {
      Event event = {CONNECT, info.getConnHandle(), 0, {0}};
      owner.enqueue(event);
    }
    void onDisconnect(NimBLEServer *, NimBLEConnInfo &info, int) override {
      Event event = {DISCONNECT, info.getConnHandle(), 0, {0}};
      owner.enqueue(event);
    }
  };

  class CommandCallbacks : public NimBLECharacteristicCallbacks {
    HSBleProvision &owner;

  public:
    explicit CommandCallbacks(HSBleProvision &owner) : owner(owner) {}
    void onWrite(NimBLECharacteristic *characteristic, NimBLEConnInfo &info) override {
      std::string value = characteristic->getValue();
      Event event = {FRAME, info.getConnHandle(), 0, {0}};
      if (value.length() > MAX_FRAME) {
        owner.queueFull.store(true);
        return;
      }
      event.length = value.length();
      memcpy(event.bytes, value.data(), event.length);
      owner.enqueue(event);
    }
  };

  QueueHandle_t events = nullptr;
  TaskHandle_t autoPollTask = nullptr;
  NimBLEServer *server = nullptr;
  NimBLECharacteristic *status = nullptr;
  std::atomic<bool> queueFull{false};
  bool connected = false;
  bool authorized = false;
  bool receiving = false;
  uint16_t connection = 0;
  uint8_t ssidLength = 0;
  uint8_t passwordLength = 0;
  uint8_t nextSequence = 0;
  size_t received = 0;
  uint8_t credentials[MAX_SSID_BYTES + MAX_PASSWORD_BYTES] = {0};
  uint8_t lastResult = READY;
  uint8_t lastWifiState = NO_CREDENTIALS;
  uint32_t lastActivity = 0;
  uint32_t lastWifiCheck = 0;
  uint32_t restartAt = 0;

  void enqueue(const Event &event) {
    if (xQueueSend(events, &event, 0) != pdTRUE) queueFull.store(true);
  }

  void clearTransfer() {
    receiving = false;
    ssidLength = 0;
    passwordLength = 0;
    nextSequence = 0;
    received = 0;
    memset(credentials, 0, sizeof(credentials));
  }

  void clearSession() {
    authorized = false;
    clearTransfer();
  }

  uint8_t wifiState() {
    if (WiFi.status() == WL_CONNECTED) return CONNECTED;
    if (homeSpan.getStatus().first == HS_WIFI_NEEDED) return NO_CREDENTIALS;
    return millis() < WIFI_TIMEOUT_MS ? CONNECTING : NOT_CONNECTED;
  }

  void publish(uint8_t result) {
    lastResult = result;
    lastWifiState = wifiState();
    uint8_t value[] = {PROTOCOL_VERSION, lastResult, lastWifiState};
    status->setValue(value, sizeof(value));
    if (connected) status->notify();
  }

  void handleFrame(const Event &event) {
    if (!connected || event.connection != connection) return;
    lastActivity = millis();

    if (event.length < 3 || event.length > MAX_FRAME || event.bytes[2] != event.length - 3) {
      publish(BAD_FORMAT);
      return;
    }

    const uint8_t opcode = event.bytes[0];
    const uint8_t sequence = event.bytes[1];
    const uint8_t length = event.bytes[2];
    const uint8_t *data = event.bytes + 3;

    if (opcode == AUTH) {
      clearSession();
      if (sequence != 0 || length != strlen(HS_OTA_PASSWORD) ||
          memcmp(data, HS_OTA_PASSWORD, length) != 0) {
        publish(BAD_AUTH);
        return;
      }
      authorized = true;
      publish(AUTH_OK);
      return;
    }

    if (!authorized) {
      publish(UNAUTHORIZED);
      return;
    }

    switch (opcode) {
      case BEGIN:
        if (sequence != 0 || length != 2 || data[0] == 0 || data[0] > MAX_SSID_BYTES ||
            data[1] > MAX_PASSWORD_BYTES) {
          publish(BAD_FORMAT);
          return;
        }
        clearTransfer();
        ssidLength = data[0];
        passwordLength = data[1];
        receiving = true;
        publish(BEGIN_OK);
        return;

      case DATA:
        if (!receiving || length == 0 || sequence != nextSequence ||
            received + length > ssidLength + passwordLength) {
          clearTransfer();
          publish(BAD_SEQUENCE);
          return;
        }
        memcpy(credentials + received, data, length);
        received += length;
        nextSequence++;
        publish(DATA_OK);
        return;

      case COMMIT: {
        if (!receiving || sequence != 0 || length != 0 ||
            received != ssidLength + passwordLength ||
            memchr(credentials, 0, received) != nullptr) {
          clearTransfer();
          publish(BAD_FORMAT);
          return;
        }
        char ssid[MAX_SSID_BYTES + 1] = {0};
        char password[MAX_PASSWORD_BYTES + 1] = {0};
        memcpy(ssid, credentials, ssidLength);
        memcpy(password, credentials + ssidLength, passwordLength);
        clearSession();

        // HomeSpan 的自动轮询在另一任务中运行；写入凭据时持有同一把锁。
        {
          std::lock_guard<std::shared_mutex> lock(homeSpan.getMutex());
          homeSpan.setWifiCredentials(ssid, password);
        }
        memset(ssid, 0, sizeof(ssid));
        memset(password, 0, sizeof(password));
        publish(SAVED);
        restartAt = millis() + 1500;
        return;
      }

      default:
        publish(BAD_FORMAT);
        return;
    }
  }

public:
  void begin(const char *namePrefix) {
    events = xQueueCreate(8, sizeof(Event));
    if (!events) {
      Serial.println("BLE provisioning: cannot create event queue");
      return;
    }

    char name[20];
    snprintf(name, sizeof(name), "%s-%04X", namePrefix, (unsigned)(ESP.getEfuseMac() & 0xFFFF));
    NimBLEDevice::init(name);
    NimBLEDevice::setSecurityAuth(true, false, true);
    NimBLEDevice::setSecurityIOCap(BLE_HS_IO_NO_INPUT_OUTPUT);

    server = NimBLEDevice::createServer();
    server->setCallbacks(new ServerCallbacks(*this));
    server->advertiseOnDisconnect(true);
    NimBLEService *service = server->createService(SERVICE_UUID);
    NimBLECharacteristic *command = service->createCharacteristic(
        COMMAND_UUID, NIMBLE_PROPERTY::WRITE | NIMBLE_PROPERTY::WRITE_ENC);
    command->setCallbacks(new CommandCallbacks(*this));
    status = service->createCharacteristic(
        STATUS_UUID, NIMBLE_PROPERTY::READ | NIMBLE_PROPERTY::NOTIFY);
    publish(READY);
    service->start();

    NimBLEAdvertising *advertising = NimBLEDevice::getAdvertising();
    advertising->setName(name);
    advertising->addServiceUUID(SERVICE_UUID);
    advertising->enableScanResponse(true);
    advertising->start();
    Serial.printf("BLE provisioning ready: %s\n", name);
  }

  void autoPoll(uint32_t stackSize = 8192, UBaseType_t priority = 1, BaseType_t core = 0) {
    if (!events || !server) {
      Serial.println("BLE provisioning: cannot start auto-poll before begin");
      return;
    }
    if (autoPollTask) return;

    BaseType_t result = xTaskCreateUniversal(
        [](void *context) {
          auto *provision = static_cast<HSBleProvision *>(context);
          const TickType_t interval = pdMS_TO_TICKS(10) ? pdMS_TO_TICKS(10) : 1;
          for (;;) {
            provision->poll();
            vTaskDelay(interval);
          }
        },
        "bleProvision", stackSize, this, priority, &autoPollTask, core);
    if (result != pdPASS) {
      autoPollTask = nullptr;
      Serial.println("BLE provisioning: cannot create auto-poll task");
    }
  }

  void poll() {
    if (!events) return;
    Event event;
    while (xQueueReceive(events, &event, 0) == pdTRUE) {
      if (event.type == CONNECT) {
        if (connected && event.connection != connection) {
          server->disconnect(event.connection);
          continue;
        }
        connection = event.connection;
        connected = true;
        clearSession();
        lastActivity = millis();
        publish(READY);
      } else if (event.type == DISCONNECT) {
        if (connected && event.connection == connection) {
          connected = false;
          clearSession();
          publish(READY);
        }
      } else {
        handleFrame(event);
      }
    }

    if (queueFull.exchange(false)) {
      clearSession();
      publish(BUSY);
    }
    if (connected && authorized && millis() - lastActivity > SESSION_TIMEOUT_MS) {
      clearSession();
      publish(UNAUTHORIZED);
    }
    if (millis() - lastWifiCheck >= 1000) {
      lastWifiCheck = millis();
      uint8_t current = wifiState();
      if (current != lastWifiState) publish(READY);
    }
    if (restartAt && (int32_t)(millis() - restartAt) >= 0) ESP.restart();
  }
};

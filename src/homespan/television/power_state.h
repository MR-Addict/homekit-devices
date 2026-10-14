#pragma once

#include <WiFi.h>
#include <atomic>
#include <ping/ping_sock.h>

class TVPowerState {
private:
  esp_ping_handle_t ping = nullptr;
  ip_addr_t targetAddr{};
  bool configured = false;
  std::atomic<int> value{-1};
  std::atomic<unsigned> misses{0};
  uint32_t retryAt = 0;

public:
  bool begin(const char *ip) {
    if (configured || !ip) return false;
    configured = ipaddr_aton(ip, &targetAddr) != 0;
    return configured;
  }

  int state() {
    if (!configured) return -1;
    if (WiFi.status() != WL_CONNECTED) {
      misses = 0;
      value = -1;
      return -1;
    }
    if (!ping && (int32_t)(millis() - retryAt) >= 0) {
      retryAt = millis() + 5000;
      esp_ping_config_t config = ESP_PING_DEFAULT_CONFIG();
      config.target_addr = targetAddr;
      config.count = ESP_PING_COUNT_INFINITE;
      config.interval_ms = 2000;
      config.timeout_ms = 1000;

      esp_ping_callbacks_t callbacks = {};
      callbacks.cb_args = this;
      callbacks.on_ping_success = [](esp_ping_handle_t, void *arg) {
        auto *tv = static_cast<TVPowerState *>(arg);
        tv->misses = 0;
        tv->value = 1;
      };
      callbacks.on_ping_timeout = [](esp_ping_handle_t, void *arg) {
        auto *tv = static_cast<TVPowerState *>(arg);
        if (WiFi.status() != WL_CONNECTED) {
          tv->misses = 0;
          tv->value = -1;
        } else if (tv->misses < 3 && ++tv->misses >= 3) {
          tv->value = 0;
        }
      };
      if (esp_ping_new_session(&config, &callbacks, &ping) == ESP_OK &&
          esp_ping_start(ping) != ESP_OK) {
        esp_ping_delete_session(ping);
        ping = nullptr;
      }
    }
    return value;
  }
};

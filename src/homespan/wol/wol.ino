#include "HomeSpan.h"

#include <WiFiUdp.h>
#include <WakeOnLan.h>

WiFiUDP UDP;
WakeOnLan WOL(UDP);

#include "wol.h"
#include "../common/ble_provision.h"

HSBleProvision bleProvision;

void setup() {
  Serial.begin(115200);

  homeSpan.enableOTA(HS_OTA_PASSWORD);
  homeSpan.begin(Category::Switches, "HSWOL", "HomeSpan-HSWOL");

  // 第一台设备（PVE 主机）
  new SpanAccessory();
  new Service::AccessoryInformation();
  new Characteristic::Identify();
  new HSWOL("E0:51:D8:11:3D:CE");

  // 第二台设备（出租屋电脑）
  new SpanAccessory();
  new Service::AccessoryInformation();
  new Characteristic::Identify();
  new HSWOL("74:56:3C:D3:65:74");

  bleProvision.begin("HS-WOL");
  bleProvision.autoPoll();
  homeSpan.autoPoll();
}

void loop() {}

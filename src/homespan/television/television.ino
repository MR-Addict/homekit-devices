#include "HomeSpan.h"

#include "television.h"
#include "../common/ble_provision.h"

HSBleProvision bleProvision;

void setup() {
  Serial.begin(115200);

  homeSpan.enableOTA(HS_OTA_PASSWORD);
  homeSpan.begin(Category::Television, "Xiaomi TV", "HomeSpan-IRTV");

  new SpanAccessory();
  new Service::AccessoryInformation();
  new Characteristic::Identify();

  auto *tv = new HSTelevision();
  if (!tv->begin("192.168.10.173"))
    Serial.println("TV power state initialization failed");

  bleProvision.begin("HS-IRTV");
  bleProvision.autoPoll();
  homeSpan.autoPoll();
}

void loop() {}

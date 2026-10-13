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

  new HSTelevision();

  bleProvision.begin("HS-IRTV");
  bleProvision.autoPoll();
  homeSpan.autoPoll();
}

void loop() {}

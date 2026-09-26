#include "HomeSpan.h"

#include "lamp.h"
#include "../common/ble_provision.h"

HSBleProvision bleProvision;

void setup() {
  Serial.begin(115200);

  homeSpan.enableOTA(HS_OTA_PASSWORD);
  homeSpan.begin(Category::Lighting, "Table Lamp", "HomeSpan-TableLamp");

  new SpanAccessory();
  new Service::AccessoryInformation();
  new Characteristic::Identify();

  new HSTableLamp(25, 26, 27, 15);

  bleProvision.begin("HS-Lamp");
  homeSpan.autoPoll();
}

void loop() {
  bleProvision.poll();
  delay(10);
}

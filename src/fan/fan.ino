#include "HomeSpan.h"

#include "fan.h"
#include "../common/ble_provision.h"

HSBleProvision bleProvision;

void setup() {
  Serial.begin(115200);

  homeSpan.enableOTA(HS_OTA_PASSWORD);
  homeSpan.begin(Category::Fans, "Fan", "HomeSpan-Fan");

  new SpanAccessory();
  new Service::AccessoryInformation();
  new Characteristic::Identify();

  new HSFan(4, 3);

  bleProvision.begin("HS-Fan");
  bleProvision.autoPoll();
  homeSpan.autoPoll();
}

void loop() {}

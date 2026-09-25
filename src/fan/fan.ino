#include "HomeSpan.h"

#include "fan.h"

void setup() {
  Serial.begin(115200);

  homeSpan.enableOTA("ota");
  homeSpan.begin(Category::Fans, "Fan", "HomeSpan-Fan");

  new SpanAccessory();
  new Service::AccessoryInformation();
  new Characteristic::Identify();

  new HSFan(4, 3);

  homeSpan.autoPoll();
}

void loop() {}

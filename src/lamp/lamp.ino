#include "HomeSpan.h"

#include "lamp.h"

void setup() {
  Serial.begin(115200);

  homeSpan.enableOTA("ota");
  homeSpan.begin(Category::Lighting, "Table Lamp", "HomeSpan-TableLamp");

  new SpanAccessory();
  new Service::AccessoryInformation();
  new Characteristic::Identify();

  new HSTableLamp(25, 26, 27, 15);

  homeSpan.autoPoll();
}

void loop() {}

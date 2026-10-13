#pragma once

#include "ir_codes.h"
#include "power_state.h"

struct HSTelevisionSpeaker : Service::TelevisionSpeaker {
private:
  SpanCharacteristic *volume;

public:
  HSTelevisionSpeaker() {
    new Characteristic::VolumeControlType(3);
    volume = new Characteristic::VolumeSelector();
  }

  boolean update() override {
    return volume->getNewVal() ? TelevisionIR::send(TelevisionIR::volume_down)
                              : TelevisionIR::send(TelevisionIR::volume_up);
  }
};

struct HSTelevision : Service::Television {
private:
  SpanCharacteristic *active;
  SpanCharacteristic *remote;
  TVPowerState powerState;
  int pendingPower = -1;
  uint32_t powerDeadline = 0;

public:
  HSTelevision() {
    pinMode(21, OUTPUT);
    digitalWrite(21, HIGH);
    if (!TelevisionIR::begin()) Serial.println("IR initialization failed");

    setPrimary();
    (new Characteristic::ConfiguredName("Xiaomi TV"))->removePerms(PW);
    active = new Characteristic::Active(0);
    remote = new Characteristic::RemoteKey();
    addLink(new HSTelevisionSpeaker());
  }

  boolean update() override {
    if (active->updated()) {
      int power = powerState.state();
      int target = active->getNewVal();
      if (power < 0) return false;
      if (pendingPower >= 0) return target == pendingPower;
      if (power != target) {
        if (!TelevisionIR::send(TelevisionIR::power)) return false;
        pendingPower = target;
        powerDeadline = millis() + 45000;
      }
    }
    if (!remote->updated()) return true;

    switch (remote->getNewVal()) {
      case 4: return TelevisionIR::send(TelevisionIR::up);
      case 5: return TelevisionIR::send(TelevisionIR::down);
      case 6: return TelevisionIR::send(TelevisionIR::left);
      case 7: return TelevisionIR::send(TelevisionIR::right);
      case 8: return TelevisionIR::send(TelevisionIR::select);
      case 9: return TelevisionIR::send(TelevisionIR::back);
      case 10: case 15: return TelevisionIR::send(TelevisionIR::home);
      default: return false;
    }
  }

  void loop() override {
    int power = powerState.state();
    if (pendingPower >= 0) {
      if (power == pendingPower || (int32_t)(millis() - powerDeadline) >= 0)
        pendingPower = -1;
      else return;
    }
    if (power >= 0 && active->getVal() != power) active->setVal(power);
  }
};

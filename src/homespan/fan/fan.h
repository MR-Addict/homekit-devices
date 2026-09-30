#pragma once

// 风扇服务：HomeKit 和实体按键共用同一个持久化开关状态
struct HSFan : Service::Fan {
private:
  int fanPin;
  SpanCharacteristic *active;

  void setFanState(bool on) {
    digitalWrite(fanPin, on ? HIGH : LOW);
  }

public:
  HSFan(int outputPin, int buttonPin) : Service::Fan(), fanPin(outputPin) {
    // 默认关闭；之后从 NVS 恢复上次由 HomeKit 或按键设置的状态
    active = new Characteristic::Active(0, true);

    digitalWrite(fanPin, LOW);
    pinMode(fanPin, OUTPUT);
    setFanState(active->getVal());

    // 按键连接 GPIO 与 GND，内部上拉；禁用双击以便每次短按都切换一次
    new SpanButton(buttonPin, 2000, 5, 0, SpanButton::TRIGGER_ON_LOW);
  }

  void button(int pin, int pressType) override {
    if (pressType != SpanButton::SINGLE) return;

    bool on = !active->getVal();
    setFanState(on);
    active->setVal(on);
  }

  boolean update() override {
    setFanState(active->getNewVal());
    return true;
  }
};

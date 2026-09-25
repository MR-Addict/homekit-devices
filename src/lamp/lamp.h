#pragma once

// 台灯服务：通过三个驱动引脚控制灯板，并支持实体按键本地开关
struct HSTableLamp : Service::LightBulb {
private:
  int lampPinA;
  int lampPinB;
  int lampPinC;

  SpanCharacteristic *power;

  // 开灯时 B、C 拉高，关灯时全部拉低；A 始终保持低电平
  void setLampState(bool state) {
    digitalWrite(lampPinA, LOW);
    digitalWrite(lampPinB, state ? HIGH : LOW);
    digitalWrite(lampPinC, state ? HIGH : LOW);
  }

public:
  HSTableLamp(int pinA, int pinB, int pinC, int button) : Service::LightBulb() {
    power = new Characteristic::On(false, true);

    pinMode(pinA, OUTPUT);
    pinMode(pinB, OUTPUT);
    pinMode(pinC, OUTPUT);

    lampPinA = pinA;
    lampPinB = pinB;
    lampPinC = pinC;

    new SpanButton(button, SpanButton::TRIGGER_ON_HIGH);
    setLampState(power->getVal());
  }

  // 实体按键按下时翻转开关状态
  void button(int pin, int pressType) override {
    bool state = !power->getVal();
    setLampState(state);
    power->setVal(state);
  }

  boolean update() override {
    setLampState(power->getNewVal());
    return true;
  }
};

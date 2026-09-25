#pragma once

// 网络唤醒开关：收到开机请求时向局域网广播 Wake-on-LAN Magic Packet
struct HSWOL : Service::Switch {
private:
  const char *mac;
  SpanCharacteristic *power;

public:
  HSWOL(const char *mac) : Service::Switch() {
    this->mac = mac;
    power = new Characteristic::On();
  }

  boolean update() override {
    // 只处理开机请求，关机请求无对应动作
    if (power->getNewVal()) {
      Serial.println(String("Waking up machine: ") + mac);
      WOL.calculateBroadcastAddress(WiFi.localIP(), WiFi.subnetMask());
      return WOL.sendMagicPacket(mac);
    }
    return false;
  }

  void loop() override {
    // 唤醒包发出 3 秒后把开关复位，便于连续触发
    if (power->getVal() && power->timeVal() > 3000) power->setVal(false);
  }
};

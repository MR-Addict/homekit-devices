#pragma once

#include <esp32-hal-rmt.h>

namespace TelevisionIR {

// 小米 L43M5-EK 原装遥控器波形，单位为微秒。
static constexpr uint16_t power[] = {
  980, 620, 580, 620, 530, 1520, 580, 1470, 580, 620, 530, 1520,
  580, 620, 530, 1520, 580, 570, 580, 1520, 530, 1520, 580
};
static constexpr uint16_t volume_up[] = {
  980, 620, 580, 1170, 580, 620, 580, 920, 580, 1170, 580, 620,
  580, 570, 580, 1520, 580, 1170, 580, 620, 580, 620, 530
};
static constexpr uint16_t volume_down[] = {
  980, 620, 580, 1170, 580, 620, 580, 920, 580, 1170, 580, 620,
  580, 570, 580, 1520, 580, 1470, 580, 620, 580, 870, 580
};
static constexpr uint16_t up[] = {
  1030, 620, 580, 1170, 580, 620, 580, 870, 580, 1170, 580, 620,
  580, 620, 580, 920, 530, 920, 580, 1170, 580, 1470, 580
};
static constexpr uint16_t down[] = {
  980, 620, 580, 1170, 580, 620, 580, 870, 580, 1220, 530, 620,
  580, 620, 580, 920, 580, 1170, 580, 1170, 580, 620, 580
};
static constexpr uint16_t left[] = {
  980, 620, 580, 1170, 580, 620, 580, 870, 580, 1220, 530, 620,
  580, 620, 580, 1170, 580, 1520, 530, 920, 580, 920, 530
};
static constexpr uint16_t right[] = {
  1030, 620, 580, 1170, 580, 620, 580, 920, 530, 1220, 580, 620,
  580, 570, 580, 1520, 580, 570, 580, 620, 580, 1220, 530
};
static constexpr uint16_t select[] = {
  980, 620, 580, 1220, 530, 620, 580, 920, 580, 1170, 580, 620,
  580, 620, 580, 1470, 580, 870, 580, 620, 580, 1470, 580
};
static constexpr uint16_t back[] = {
  980, 620, 580, 1170, 580, 620, 580, 870, 580, 1220, 580, 570,
  580, 620, 580, 920, 580, 1470, 580, 1170, 580, 920, 580
};
static constexpr uint16_t home[] = {
  1030, 620, 580, 1170, 580, 620, 580, 870, 580, 1220, 530, 620,
  580, 620, 580, 1220, 530, 620, 580, 920, 580, 1170, 580
};

bool begin() {
  return rmtInit(4, RMT_TX_MODE, RMT_MEM_NUM_BLOCKS_1, 1000000) &&
         rmtSetCarrier(4, true, false, 38000, 0.3);
}

template <size_t N>
bool send(const uint16_t (&code)[N]) {
  rmt_data_t symbols[(N + 1) / 2] = {};
  for (size_t i = 0; i < N; i += 2) {
    symbols[i / 2].duration0 = code[i];
    symbols[i / 2].level0 = 1;
    symbols[i / 2].duration1 = i + 1 < N ? code[i + 1] : 1;
    symbols[i / 2].level1 = 0;
  }
  return rmtWrite(4, symbols, (N + 1) / 2, RMT_WAIT_FOR_EVER);
}

}

# Замедлить нельзя ускорить: как мы сократили память Scraper в Prom++ в 3.33× и не потеряли в скорости

> Уровень сложности: сложный
> Время на прочтение: ~ 15–20 мин

> [РИСУНОК-ОБЛОЖКА] — иллюстрация-метафора: «упаковали чемодан плотнее, но успели на самолёт». Можно сделать схему «байт-в-байт» — слева жирный layout, справа компактный.

Всем привет! Меня зовут Глеб Шигин, я C++-разработчик в команде Deckhouse Prom++. Это статья про то, как мы прошли путь от «сделали наивный POC, выиграли в памяти, но просели в скорости» до «выиграли и в памяти, и в чтении» — на одном hot-path куске кода и за неделю последовательных коммитов.

Кратко предыстория. Prom++ — наш форк Prometheus, в котором ядро хранения и обработки горячих данных переписано на C++, при сохранении полной совместимости с Prometheus и его periphery на Go (см. предыдущие статьи: «Deckhouse Prom++: мы добавили плюсы к Prometheus и сократили потребление памяти в 7,8 раза», «FastCGo»). После ряда оптимизаций storage, индексов и WAL, очередным «жирным» местом по памяти оказался **Scraper** — компонент, который превращает «сырой» текстовый дамп `/metrics` от target-а в наш markup-буфер.

Что будет в статье:

- Разберём, что такое Scraper в Prom++ и как у него устроен markup.
- Посмотрим на распределение реальных данных и сделаем дубовый POC, который ужимает память в 3.65×, но ломает чтение.
- Превратим POC в рабочий формат с тестами, заплатив за это просадкой по скорости.
- Отыграем скорость обратно — с помощью unrolled-декодера, `over-allocate-then-shrink` и LZCNT.
- На каждом этапе будем мерить Google Benchmark + смотреть Tracy-трейс.

Содержание:

- Контекст и мотивация
- Исходное решение
- Гипотеза и POC
- Превращаем POC в рабочий формат
- Отыгрываем скорость: read/write opt
- Дочищаем sample и parse_metric
- Итог
- Выводы

> [ЗАМЕР-СТЕНД] Все бенчмарки проводились на одной машине: укажи реальную модель CPU, OS, версию clang/gcc, флаг `-c opt --copt=-march=native`, отсутствие ASan/TSan, отсутствие частотных скачков. Это блок «как у Владимира в FastCGo-статье»: nice -n -20, фиксированный CPU governor performance, и т.п.

## TL;DR

- Profiled Scraper, увидели, что:
  - на каждую метрику тратится **28 + 16·N** байт на markup (N — число labels) + ~50% overshoot за счёт `reserve(buffer.size()/2)`;
  - реальные `name.length`, `value.length`, относительные offset-ы почти всегда умещаются в 1–2 байта;
  - >95% значений — маленькие положительные целые counters, timestamp в большинстве случаев отсутствует.
- Сделали variable-length encoding для labels и samples + относительные offset'ы + отдельный header-массив `Vector<MarkedMetric>` поверх variable-tail `Vector<char>`.
- POC просел по скорости, но мы вернули её обратно за счёт unrolled-загрузок переменной длины, `resize+write+shrink` и `countl_zero` для расчёта длины.
- Итог: память **−70% (×3.33)**, read **+33..42%**, parse — небольшой проигрыш в худшем кейсе.

> [ЗАМЕР-ИТОГ] График / таблица из 4 столбцов: parse, read, allocated_memory, ось X — ревизия. Две панели: cehp-like и m3-like дампы. Это ровно тот самый итоговый сводный график, на который дальше ссылаемся.

## Контекст и мотивация

Scraper — компонент, который раз в `scrape_interval` секунд получает HTTP-ответ от target-а в формате Prometheus text exposition (или OpenMetrics) и распарсивает его в нашу внутреннюю модель. Дальше его данные забирают шарды и складывают в head/WAL. Между «парсингом» и «забором шардом» данные живут в **markup-буфере** Scraper-а.

```
target /metrics  ──HTTP──▶  PrometheusParser  ──tokenize──▶  Scraper::parse  ──▶  markup buffer
                                                                                       │
                                                                                       ├── shard 0 ── metric.read(ts)
                                                                                       ├── shard 1 ── metric.read(ts)
                                                                                       └── ...
```

> [РИСУНОК-1] Аккуратная схема пайплайна. Хорошо бы показать, что markup-буфер живёт целиком, пока все шарды не вычитают свою половину/треть.

Сколько мы сейчас платим за этот буфер? Берём типичный дамп `kube-apiserver /metrics` (~13 МБ текста, ~85 тысяч метрик) и пишем простой бенчмарк:

```cpp
void ScraperParse(benchmark::State& state) {
  ZoneScoped;
  const auto str = get_file_content();
  std::string tmp_str;
  tmp_str.resize(str.size());

  for ([[maybe_unused]] auto _ : state) {
    std::memcpy(tmp_str.data(), str.data(), str.size());
    PrometheusScraper scraper;
    std::ignore = scraper.parse(tmp_str, 0);
  }

  PrometheusScraper scraper;
  auto tmp_str2 = str;
  std::ignore = scraper.parse(tmp_str2, 0);
  state.counters["Alloc"] =
      benchmark::Counter(static_cast<double>(scraper.allocated_memory()),
                         benchmark::Counter::kDefaults,
                         benchmark::Counter::OneK::kIs1024);
}
```

> [ЗАМЕР-BASELINE] Стартовые числа на baseline-коммите `3f0b2bd72`:
>
> - cehp-like дамп: ScraperParse ≈ 38 ms, ScraperRead ≈ 11 ms, Alloc ≈ **13.7 MiB**.
> - m3-like дамп: ScraperParse ≈ 23 ms, ScraperRead ≈ 1.8 ms, Alloc ≈ **13.7 MiB**.

Чтобы было удобно, сразу зафиксируем три бенчмарка:

- `Parser` — голая токенизация. Не KPI, нужна только чтобы вычитать «тело» из `ScraperParse`.
- `ScraperParse` — полный `Scraper::parse`. KPI по записи + `Alloc`.
- `ScraperRead` — полный проход `for (auto& m : metrics) m.read(ts)`. KPI по чтению.

«Чистое время скрейпера» дальше будет фигурировать как `Plain Scraper Time = ScraperParse - Parser`. Это позволяет не путать прирост на токенизаторе с приростом на самом скрейпере.

## Исходное решение

Внутри Scraper лежит `MetricMarkupBuffer` — обычный `BareBones::Vector<char>` (наша обёртка над контейнером с управляемой памятью). В него подряд складываются метрики:

```cpp
#pragma pack(push, 1)
struct MarkedString { uint32_t offset; uint32_t length; };          // 8 B
struct MarkedLabel  { MarkedString name; MarkedString value; };      // 16 B

struct MarkedLabelSet {
  uint32_t count;
  MarkedLabel labels[];
};

struct MarkedMetric {
  uint64_t hash;            // 8
  Primitives::Sample sample; // 16  (double value + int64 timestamp)
  MarkedLabelSet label_set;  // 4 + 16 * N
};
#pragma pack(pop)
```

Итого на одну метрику: **28 + 16 · N** байт. Плюс при старте парсинга буфер резервируется как `metric_buffer_.initialize(buffer.size() / 2)`, что для 13 МБ дампа сразу даёт ~6.5 МБ и ещё 3.25 МБ на metadata.

> [РИСУНОК-LAYOUT-V0] Картинка байт-в-байт: один `MarkedMetric` с тремя `MarkedLabel` подряд, цветом обозначить, что хранится. Подсветить, что на `__name__` тоже тратится 16 байт.

На что мы посмотрели в первую очередь:

> [ЗАМЕР-РАСПРЕДЕЛЕНИЕ] Гистограммы по реальному дампу:
>
> - длины `name`/`value` (доля укладывающихся в 1, 2, 3, 4 байта) — ожидаем, что 1–2 байта покрывают 95%+;
> - распределение типов значений: zero / uint8 / uint16 / uint32 / float / double / NaN — ожидаем доминирование small uint;
> - доля метрик с собственным timestamp — ожидаем, что подавляющее большинство без него.

Эти три картинки и есть «топливо» для дальнейших гипотез. В нашей нагрузке:

- 95+% `name.length` ≤ 255, 99+% `value.length` ≤ 65535;
- ~99% значений — целые положительные counters;
- timestamp в самой строке — единичные случаи (например, `pushgateway`).

То есть мы платим за worst-case, которого почти никогда не бывает.

## Гипотеза и POC

«Если данных мало, давайте кодировать переменной длиной». Проблема в том, что переменная длина — это либо varint (с continue-битами), либо префиксы-длины. И то и другое — лишние ветки в hot loop. Поэтому первая попытка — два независимых POC: один — переменная упаковка labels, другой — переменная упаковка sample. Их **специально** делали без read-пути и без тестов, чтобы быстро увидеть «потолок» по памяти.

### POC 1 — labels varint

Идея простая. Каждый `MarkedString` хранит `offset` и `length`, оба — `uint32`. Вместо этого:

- `offset`-ы делаем **относительными** к началу самой метрики. Тогда они почти всегда влезают в 1 байт.
- Каждое из 4 полей label-а (`name.offset`, `name.length`, `value.offset`, `value.length`) кодируем 1, 2, 3 или 4 байтами в зависимости от величины.
- В начале label-а кладём 1-байтовый **layout descriptor**, в котором по 2 бита на каждое из 4 полей описывают, сколько байт оно занимает.
- `__name__` — это специальный label, который встречается ровно один раз на метрику. Его имя не нужно хранить вовсе, обозначаем «(0, 0)» в полях имени и при чтении подставляем константу `Prometheus::kMetricLabelName`.
- Количество labels тоже varint: 1–5 байт.

```cpp
const uint8_t sz0 = encode_size(label.name.offset);
const uint8_t sz1 = encode_size(label.name.length);
const uint8_t sz2 = encode_size(label.value.offset);
const uint8_t sz3 = encode_size(label.value.length);

const uint8_t layout = (sz0) | (sz1 << 2) | (sz2 << 4) | (sz3 << 6);

std::array<char, 17> tmp{};
char* out = tmp.data();
*out++ = static_cast<char>(layout);
*reinterpret_cast<uint32_t*>(out) = label.name.offset;  out += sz0 + 1;
*reinterpret_cast<uint32_t*>(out) = label.name.length;  out += sz1 + 1;
*reinterpret_cast<uint32_t*>(out) = label.value.offset; out += sz2 + 1;
*reinterpret_cast<uint32_t*>(out) = label.value.length; out += sz3 + 1;

this->buffer_.push_back(tmp.data(), out);
```

Best case: **5 байт/label** против 16. Worst: 17.

> [РИСУНОК-LABEL-LAYOUT] Картинка одного label-а: 1 байт layout + 4 переменных поля, цветом подсветить, какое поле сколько байт. На втором кадре — то же самое для `__name__` (ноль байт на name).

Чтобы лейблы можно было упаковать в правильном порядке (с одинаковым хешем по сравнению с baseline), приходится сначала собрать их в скретч-вектор `BareBones::Vector<MarkedLabel> labels_` (один на класс, `reserve(255)`), отсортировать по name, посчитать xxhash, и только потом батчем эмитить в основной буфер. Это +1 проход по labels на каждую метрику.

### POC 2 — sample encoding

Идея ещё проще. `Sample` — это всегда 16 байт (8 на double, 8 на timestamp). При том, что почти всегда:

- timestamp равен default-у (т.е. его нет в дампе),
- value — небольшое положительное целое.

Кодируем 1 байтом маркера + переменное тело:

```
bit 7      : has_ts  (если 1 — за value идут 8 байт timestamp)
bits 0..3  : тип значения
   0000  zero        (0 байт)
   0001  uint8       (1 байт)
   0010  uint16      (2 байта)
   0011  uint32      (4 байта)
   0100  staleNaN    (0 байт)
   1000  float32     (4 байта)
   1001  double      (8 байт)
```

Best case: **1 байт** на sample (нулевой counter без TS); worst — 17 байт (double + ts), почти не хуже фиксированных 16.

```cpp
if (std::isnan(val)) [[unlikely]]               { flush(0b00000100); return; }   // NaN
if (val == 0.0)      [[unlikely]]               { flush(0b00000000); return; }   // zero
if (std::trunc(val) == val && val > 0.0) [[likely]] {
  auto ival = static_cast<int64_t>(val);
  if (ival <= 0xFF)     { flush(0b00000001); append((uint8_t)ival);  return; }
  if (ival <= 0xFFFF)   { flush(0b00000010); append((uint16_t)ival); return; }
  if (ival <= 0xFFFFFFFFULL) [[likely]] { flush(0b00000011); append((uint32_t)ival); return; }
}
float f = static_cast<float>(val);
if (static_cast<double>(f) == val) [[unlikely]] { flush(0b00001000); append(f);   return; }  // float32
flush(0b00001001); append(val);                                                              // double
```

> [РИСУНОК-SAMPLE-LAYOUT] Маркер-байт + варианты тел. Хорошо смотрится «дерево» из 7 веток с подписанными байтами на каждом листе.

### Что показал POC

> [ЗАМЕР-POC] Прогон только `ScraperParse` + `Alloc` (read-пути ещё нет, поэтому соответствующие колонки пустые). Стиль — как у Владимира в FastCGo с benchstat-выводом.
>
> | Ревизия | Parse, ms (×) | Alloc, MiB (×) | Read |
> |---|---:|---:|---|
> | baseline `3f0b2bd72` | 38.2 (1.00) | 13.7 (1.00) | 11 ms |
> | + labels varint | 45.8 (0.83) | 5.0 (×2.74) | n/a |
> | + sample encoding | 47.8 (0.80) | 3.8 (×3.65) | n/a |
> | оба POC вместе (`4656f7137`) | 47.5 (0.80) | 4.15 (×3.31) | сломан |

Итог POC: память жёстко сэкономили (×3.65), но:

1. за это заплатили ~20% времени `parse` — добавились ветвистая запись label-а, цикл по 4 разрядностям, цикл по marker-типам;
2. сломали read-путь — он не реализован под новый формат, а старый perevich `Sample`/`MarkedLabel` уже не лежит в буфере;
3. тесты, естественно, красные.

Это нормально и даже полезно: мы увидели **потолок** по памяти. Дальше задача — вернуть скорость и чтение, не отдав обратно эти 9.5 МБ.

## Превращаем POC в рабочий формат

Первое, что мы сделали — реализовали полный read-декодер и сделали тесты зелёными. Параллельно встретились с архитектурным неудобством: чтобы итерироваться по такому variable-length буферу, нужно либо знать длину каждого записанного элемента (т.е. ещё раз декодировать заголовок), либо хранить длину рядом.

Мы выбрали третий путь — **разнесли header и tail по разным контейнерам**:

```cpp
class MetricMarkupBuffer {
  ...
  BareBones::Vector<MarkedMetric> metric_buffer_;  // фиксированный header per metric (16 B)
  BareBones::Vector<char>         bytes_buffer_;   // variable tail: count + labels + sample
};

struct MarkedMetric {
  uint64_t hash;          // 8
  uint32_t base_offset;   // 4 — начало метрики в исходном буфере (для относительных offset-ов)
  uint32_t data_offset;   // 4 — начало tail в bytes_buffer_
};
```

> [РИСУНОК-LAYOUT-V1] Двухэтажная схема: сверху `Vector<MarkedMetric>` — все «шапки» подряд, снизу `Vector<char>` — все упакованные «хвосты» подряд. Стрелочка `data_offset` от шапки к хвосту.

Что это даёт:

- `++iterator` теперь — это `++ptr_` по плотному `MarkedMetric*`. Не нужно декодировать tail только чтобы посчитать длину.
- На `metric_buffer_` (16 B/запись) шарды ходят кеш-friendly при чисто «hash-обходах».
- `Sample` целиком переехал в variable tail; `default_timestamp` поднялся на уровень всего `Scraper`-а — один на дамп. Это ещё минус 8 байт «обычной» метрики.

Read-декодер на эту схему получается прямолинейно:

```cpp
template <class Timeseries>
void read(Timeseries& ts) const {
  const char* ptr  = bytes_buffer_.data() + item_->data_offset;
  const char* base = buffer_.data()       + item_->base_offset;

  uint32_t labels_count = decode_varint(ptr);
  ts.label_set().reserve(labels_count);

  for (uint32_t i = 0; i < labels_count; ++i) {
    const uint8_t layout = static_cast<uint8_t>(*ptr++);
    const uint8_t sz0 = (layout >> 0) & 0x3;
    const uint8_t sz1 = (layout >> 2) & 0x3;
    const uint8_t sz2 = (layout >> 4) & 0x3;
    const uint8_t sz3 = (layout >> 6) & 0x3;

    auto read_val = [&](uint8_t sz) PROMPP_LAMBDA_INLINE -> uint32_t {
      uint32_t v = 0;
      memcpy(&v, ptr, sz + 1);  // <-- внимание на эту строку
      ptr += sz + 1;
      return v;
    };

    uint32_t name_off  = read_val(sz0);
    uint32_t name_len  = read_val(sz1);
    uint32_t value_off = read_val(sz2);
    uint32_t value_len = read_val(sz3);

    // ... append ...
  }
  // ... sample ...
}
```

Тесты зелёные. Сериализация и десериализация совместимы. Smoke-pass идёт.

> [ЗАМЕР-READ-WORKS] Та же таблица, но добавились колонки `Pass` и `Read`:
>
> | Ревизия | Parse (×) | Pass (×) | Read (×) | Alloc (×) |
> |---|---:|---:|---:|---:|
> | baseline | 1.00 | 1.00 | 1.00 | 1.00 |
> | `4656f7137` initial encoding | 0.80 | n/a | broken | 3.31 |
> | `0027219f0` read works | 0.76 | 0.70 | **0.65** | 3.33 |

Read стал **медленнее** baseline. Не на единицы процентов — на 35%.

> «Просчитался, но где?!»

Заглядываем в Tracy (включаем `--//bazel/toolchain:profiling=1`, в `Metric::read` и `parse_metric` выставляем `ZoneScopedN` на под-фазы — `decode_count`, `decode_labels`, `decode_sample`):

> [РИСУНОК-TRACY-READ-V1] Скриншот Tracy для `Metric::read` на `0027219f0`. Видно, что `decode_labels` занимает абсолютное большинство времени, а внутри него — узкое место в строке `memcpy(&v, ptr, sz + 1)`.

Дело в той самой строчке `memcpy(&v, ptr, sz + 1)` с **переменным** размером. Компилятор не разворачивает её в простой load — он эмитит `memcpy`-fallback (или generic loop). Каждый label-байт стоит отдельной инструкции call/branch. Аналогично `add_label` на write-пути сначала пишет в `std::array<char, 17> tmp`, потом делает `push_back(tmp, len)` — лишний копирующий проход.

## Отыгрываем скорость: read/write opt

Это самый важный коммит во всей серии — `9b62e6ae`. Здесь мы возвращаем скорость, не теряя ни байта памяти.

### Read: разворачиваем «memcpy с переменной длиной» руками

```cpp
PROMPP_ALWAYS_INLINE
static uint32_t read_val_partial(const char*& p, uint8_t sz) noexcept {
  if (sz == 0) [[likely]] {
    const uint32_t v = static_cast<uint8_t>(p[0]);
    p += 1;
    return v;
  }
  if (sz == 1) {
    const uint32_t v =  static_cast<uint32_t>(static_cast<uint8_t>(p[0]))
                     | (static_cast<uint32_t>(static_cast<uint8_t>(p[1])) << 8);
    p += 2;
    return v;
  }
  if (sz == 2) {
    const uint32_t v =  static_cast<uint32_t>(static_cast<uint8_t>(p[0]))
                     | (static_cast<uint32_t>(static_cast<uint8_t>(p[1])) << 8)
                     | (static_cast<uint32_t>(static_cast<uint8_t>(p[2])) << 16);
    p += 3;
    return v;
  }
  uint32_t v;
  std::memcpy(&v, p, 4);
  p += 4;
  return v;
}
```

После этого декодирование одного label-а становится цепочкой прямых нагружений + сдвигов. Switch по типу sample заодно отсортировали по частоте: `uint8 / zero / uint16 / uint32 / staleNaN / float / double`. `decode_varint` и `read_val_partial` пометили `PROMPP_ALWAYS_INLINE` — без этого компилятор для коротких функций делал отдельные `call`.

### Write: `over-allocate-then-shrink`

Та же идея, но в зеркале. Раньше `add_label` писал в локальный `std::array<char, 17> tmp` и делал `push_back(tmp, used_size)`. Это два прохода.

Стало:

```cpp
const uint32_t bytes_needed = sizeof(layout) + (sz0 + sz1 + sz2 + sz3) + 4;
const uint32_t offset = bytes_count();
bytes_buffer_.resize(bytes_buffer_.size() + 17);  // worst-case заранее
char* out = bytes_buffer_.data() + offset;

*out++ = static_cast<char>(layout);
std::memcpy(out, &label.name.offset,  sz0 + 1); out += sz0 + 1;
std::memcpy(out, &label.name.length,  sz1 + 1); out += sz1 + 1;
std::memcpy(out, &label.value.offset, sz2 + 1); out += sz2 + 1;
std::memcpy(out, &label.value.length, sz3 + 1);

bytes_buffer_.resize(offset + bytes_needed);  // shrink to actual
```

Что мы здесь выиграли:

- В `bytes_buffer_` пишем **сразу**, без промежуточного буфера.
- `resize(+17)` обеспечивает «есть куда писать»; внутри `memcpy(out, &val, sz+1)` нет проверок границ.
- В конце `resize` сжимает до фактически записанного размера. У `BareBones::Vector` это просто корректировка `size_`, без пересборки.

Тот же приём применили к `add_count` (varint длиной до 5 байт).

### Write: `encode_size` через LZCNT

Раньше:

```cpp
static uint8_t encode_size(uint32_t v) noexcept {
  if (v <= 0xFF)     return 0;
  if (v <= 0xFFFF)   return 1;
  if (v <= 0xFFFFFF) return 2;
  return 3;
}
```

Стало:

```cpp
PROMPP_ALWAYS_INLINE static uint8_t encode_size(uint32_t v) noexcept {
  const uint32_t msb = (v == 0 ? 0 : 31 - std::countl_zero(v));
  return msb >> 3;
}
```

Каскад из 3 веток заменили на одну инструкцию `lzcnt`/`bsr` + сдвиг. На hot-path этот вызов идёт ровно 4 раза на каждый label, поэтому экономия чувствуется.

### Что это дало

> [ЗАМЕР-RW-OPT] Прогон 3 бенчмарков:
>
> | Ревизия | Parse (×) | Pass (×) | Read (×) | Alloc (×) |
> |---|---:|---:|---:|---:|
> | `0027219f0` read works | 0.76 | 0.70 | 0.65 | 3.33 |
> | `9b62e6ae` r/w opt | **0.83** | **0.79** | **1.39** (cehp) / **1.39** (m3) | 3.33 |

Read **+114%** на cehp-like дампе и **+289%** на m3-like (за счёт более коротких labels). Pass прибавил ~12%. Память — ровно на месте.

> [РИСУНОК-TRACY-READ-V2] Тот же скриншот Tracy `Metric::read`, но после `9b62e6ae`. `decode_labels` ужалось примерно в 2 раза.

## Дочищаем sample и parse_metric

После главного «win-back» осталось два аккуратных шага.

### `aca48508` — тот же приём для samples

`add_sample` страдал от той же болезни, что и `add_label` до `9b62e6ae`: внутренний `flush()` лямбдой, отдельные `push_back` для маркера, для значения, для timestamp. Применяем ровно то же — `resize(+17)`, последовательный курсор, в конце `resize(actual)`:

```cpp
constexpr uint32_t max_sample_bytes = 1 + sizeof(sample.sample);  // 1 marker + 8 value + 8 ts
bytes_buffer_.resize(offset + max_sample_bytes);
char* out = bytes_buffer_.data() + offset;
char* start = out;

if (std::isnan(val)) [[unlikely]] {
  *out++ = static_cast<char>((has_ts ? 0b10000000 : 0) | 0b00000100);
} else if (val == 0.0) [[unlikely]] {
  *out++ = static_cast<char>((has_ts ? 0b10000000 : 0) | 0b00000000);
} else if (std::trunc(val) == val && val > 0.0) [[likely]] {
  // ... uint8 / uint16 / uint32 / fallback double ...
} else {
  // ... float32 / double ...
}

if (has_ts) {
  std::memcpy(out, &sample.sample.timestamp(), 8);
  out += 8;
}

bytes_buffer_.resize(offset + (out - start));
```

Параллельно поменяли layout: timestamp **после** value (раньше — до). На декодер влияет минимально, но позволяет в записи использовать один общий курсор без переустановок.

> [ЗАМЕР-SAMPLE-OPT] Очень небольшая прибавка в pass (cehp 34.3 → 33.6 ms, m3 16.9 → 16.3 ms), read — почти шум. Но это «бесплатное» улучшение в стиле уже сделанного.

### `2036d7ea` — `MetricParser` → метод

В старой схеме на каждую метрику стек-локально создавался объект:

```cpp
if (const auto error = MetricParser{parser_, metric_buffer_, labels_,
                                    static_cast<uint32_t>(tokenizer.token_str().data() - tokenizer.buffer().data()),
                                    default_timestamp}.parse();
    error != Error::kNoError) [[unlikely]] {
  metric_buffer_.remove_item();
  return error;
}
```

`MetricParser` — это вложенный класс с 4 ссылками-членами. Компилятор имеет полное право его не инлайнить — каждая метрика стоит лишнего конструктора + indirection через ref-поля.

Превратили в обычный метод `Scraper`:

```cpp
[[nodiscard]] PROMPP_ALWAYS_INLINE Error parse_metric() {
  ...
}
```

Все «ссылки» теперь — обычные `this->labels_`, `this->metric_buffer_`. `PROMPP_ALWAYS_INLINE` теперь работает по-настоящему.

> [ЗАМЕР-INLINE] Pass cehp 34.0 → 33.2 ms, m3 17.4 → 16.3 ms. Прибавка маленькая, но стабильная и видна на каждом прогоне. На Tracy исчез отдельный фрейм конструктора `MetricParser`.

> [РИСУНОК-TRACY-INLINE] Сравнение двух Tracy-кадров: до и после, видно отсутствие лишнего фрейма.

Дальше (`1113d04b`) — чистая косметика: вынес `process_labels_buffer`, `sort_and_filter_labels`, `append_labels_hash` в отдельные методы. На скорости/памяти не сказывается, но код становится читаемым.

## Итог

> [ТАБЛИЦА-ИТОГ] Сводная таблица из исследования. Структура: ревизия → ключевое изменение → Parse, Pass, Read, Alloc. Можно вставить из `2026-04-29-scraper-opt-research.md` секция 7 — её достаточно «причесать».

Чтобы было нагляднее, итог одной картинкой:

> [РИСУНОК-ИТОГ] 4-панельный график (или 2×2): по оси X — ревизии в хронологическом порядке, по Y — нормированное к baseline время/память. Панели:
>
> 1. Plain Scraper Time (cehp)
> 2. Plain Scraper Time (m3)
> 3. Read time (cehp/m3 на одной картинке)
> 4. Allocated memory
>
> Пунктирной линией — baseline = 1.0. Видны три «горки»: подъём `4656f7137` / `0027219f0`, спуск `9b62e6ae`, плоский конец.

Финальный layout одной метрики:

> [РИСУНОК-LAYOUT-FINAL] Сверху — массив `MarkedMetric` (16 B на запись). Снизу — variable tail в `bytes_buffer_`: varint count → последовательность labels (1 байт layout + 4..16 байт полей) → 1 байт marker + value + опц. timestamp.

Финальные числа:

- На наших дампах **markup-память ужалась в 3.33×** (с 13.7 МБ до 4.13 МБ).
- На «толстом» дампе read стал на **33% быстрее**, на плотном — на **42% быстрее**.
- На write — небольшой проигрыш (от −5% до −20% относительно baseline в зависимости от дампа). В нашем pipeline write делается один раз на интервал scrape-а, read — N раз на N шардов, поэтому сделка для нас — отличная.

## Выводы

- **Domain-specific knowledge — главный инструмент**. Все приёмы в этой статье — `varint`, layout-byte, `__name__`-elision, `default_timestamp` per-scraper — обоснованы тем, **как реально выглядят** Prometheus-дампы. Без гистограмм по реальным данным мы бы не выбрали 4-разрядное layout-кодирование (а взяли бы скучный `LEB128`/`varint`, и проиграли бы по скорости из-за continue-битов).
- **`memcpy` с переменной длиной — не SIMD**. Если длина — известный compile-time-набор из 1..4 байт, разверните руками. Это вернуло нам ~2× на read.
- **Over-allocate-then-shrink**. Если максимальная длина записи известна и невелика, проще сделать `resize(+max)`, писать линейно без проверок границ, а в конце `resize(actual)`. У нас это вторая половина выигрыша.
- **Считайте байты вычислениями**. `(31 - countl_zero(v)) >> 3` короче и быстрее каскада `if`, и читается ничем не хуже.
- **Inline-friendly код**. Вложенные классы с ref-членами на hot-path — ловушка. Если функция вызывается на каждой метрике — лучше метод того же класса с `ALWAYS_INLINE`.
- **Профилируйте на каждом шаге**. Tracy + Google Benchmark показывают разницу между «кажется, должно быть быстрее» и «вот тут стоит ровно тот memcpy». В нашем случае POC, который выглядел как «победа», на read-пути оказался регрессом — и без трейсов мы бы это пропустили.

> [ЗАМЕР-FINAL-SUMMARY] Здесь хорошо смотрится один итоговый бар-чарт: «было — стало» по 4 метрикам, желательно на двух типах нагрузки.

## Что осталось за кадром

В этой статье мы не разбираем:

- что происходит с `metadata_buffer_` (`# HELP / # TYPE / # UNIT`) — там оптимизаций пока не делали;
- как именно `BareBones::Vector` обрабатывает `resize(+N)` без полной реаллокации;
- интеграцию Scraper-а с шардами (это отдельная история про `hash() % N`-распределение).

Если интересно про что-то из этого — пишите в комментариях.

## P. S.

Читайте также в нашем блоге:

- Deckhouse Prom++: мы добавили плюсы к Prometheus и сократили потребление памяти в 7,8 раза
- FastCGo: как мы ускорили вызов C-кода в Go в 16,5 раза
- (другие наши статьи по Prom++)

Теги: `prom++`, `prometheus`, `c++`, `optimization`, `bit packing`, `varint`, `tracy`, `google benchmark`

---

## Чек-лист правок и допросов перед публикацией

> Это служебный блок для нас, в финальный текст не идёт.

- [ ] Проверить, что числа в TL;DR сходятся с финальным графиком.
- [ ] Подтвердить термины: «scraper» / «скрейпер» — выбрать одно написание и придерживаться.
- [ ] Все имена коммитов оставить как есть (в стиле автора-оригинала) либо переименовать в человекопонятные (например, `4656f7137` → «POC: initial encoding»).
- [ ] Решить, показываем ли мы фактические Tracy-скриншоты или схематично рисуем «как было / как стало».
- [ ] Снять все три гистограммы распределения с реальных дампов.
- [ ] Перепрогнать все замеры на одной чистой машине, единым прогоном через `bench_sweep.sh`.
- [ ] Свести итоговый график.
- [ ] Указать ссылку на репозиторий с примерами / на сам Prom++.

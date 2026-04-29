# Scraper optimization — research dump

Сводка по серии коммитов на ветке `scraper-rw-opt`, в которой `Scraper`
в `pp/wal/hashdex/scraper/scraper.h` был оптимизирован по памяти и
скорости. Используется как рабочий black-book перед написанием статьи
на Хабр.

Период: `3f0b2bd72` (2025-08-25) → `1113d04b` (2025-09-01).

## 1. Что измеряется

Бенчмарки лежат в `pp/wal/benchmarks/scraper_benchmark.cpp`.

- `BenchmarkParser` — голая токенизация (`PrometheusParser::tokenizer().tokenize` + `next()` в цикле). Это **не KPI**, а вычитаемая база, чтобы получить «чистое» время скрейпера: `Plain Scraper Time = ScraperParse - Parser`.
- `BenchmarkScraperParse` — полный `Scraper::parse(buffer, 0)` + замер `allocated_memory()`.
- `BenchmarkScraperRead` — после одного `parse()` итерируем по `scraper.metrics()` и каждому `metric.read(ts)` заполняем `TimeseriesSemiview`.

Две колонки в твоей CSV:

- `cehp-runner` — более «толстый» промовский дамп (длинные labels, смешанные значения; `parse ≈ 38..50 ms`, alloc baseline ≈ 13.7 MiB).
- `m3` — более плотный дамп (короткие labels, в основном целые counters; `parse ≈ 23..30 ms`, alloc такой же).

Файлы фикстур у тебя локальные. В `scraper.flags` указан путь
`…/wal/benchmarks/data/blobs.txt`, в репо его нет. Сейчас в `/prompp/pp/`
лежит только один дамп — `kube-api.metrics` (~13M, 85k строк, стиль
ближе к cehp-runner).

## 2. Базовый layout (`3f0b2bd72`)

```cpp
struct MarkedString { uint32 offset; uint32 length; };           // 8 B
struct MarkedLabel  { MarkedString name; MarkedString value; };   // 16 B
struct MarkedLabelSet { uint32 count; MarkedLabel labels[]; };
struct MarkedMetric  {
  uint64 hash;          // 8
  Sample sample;        // 16 (double + int64)
  MarkedLabelSet ls;    // 4 + 16*N
};
```

Итого на одну метрику: **28 + 16·N байт** + `metric_buffer_.initialize(buffer.size()/2)` overshoot.

Поверх — `MetricParser{...}.parse()` инстанцируется на каждую метрику,
читает токены, по ходу вызывает `markup_buffer_.add_label(...)`, в
конце — `calculate_hash()` (sort + xxhash).

Сам коммит `3f0b2bd72` — это разделение одного `BenchmarkScraper` на
`Parse`/`Read` + добавление счётчика `Alloc`. Это и есть baseline.

«Жирное» в этом виде:
1. На `__name__` тратится полный `MarkedLabel` 16 B — у каждой метрики.
2. Большинство `offset`/`length` в реальной нагрузке умещаются в 1–2 байта, а резервируются 4.
3. `Sample` всегда 16 B, хотя ~99% значений в Prom-text — маленькие положительные целые counters/gauges.
4. `metric_buffer_.initialize(buffer.size()/2)` берёт «с запасом», что и даёт 13.7 MiB.

## 3. Хронология коммитов

| # | Hash | Тема |
|---|------|------|
| 0 | `3f0b2bd72` | benchmark update (baseline) |
| POC₁ | (не закоммичено) | **labels varint** — POC переменной упаковки label-полей |
| POC₂ | (не закоммичено) | **sample encoding** — POC переменной упаковки value/timestamp |
| 1 | `4656f7137` | initial encoding — оба POC сведены воедино, тесты ещё не проходят, read «болт» |
| 2 | `0027219f0` | read + passing all tests — реализован декодер, разбили хранилище на `metric_buffer_` (header) + `bytes_buffer_` (variable tail) |
| 3 | `9b62e6ae` | read/write optimization — главный прирост по скорости |
| 4 | `aca48508` | sample optimization — тот же приём для samples |
| 5 | `26d0af47` | benchmark only: read лишь чётных hash (имитация sharding) |
| 6 | `2036d7ea` | `MetricParser` → метод `parse_metric()` (inline-friendly) |
| 7 | `1113d04b` | extract-method косметика над `parse_metric` |

POC-шаги `labels varint` и `sample encoding` в git-истории отдельно
не сохранились — их собрали в коммит `4656f7137` («initial encoding»).
Сами замеры по ним есть в CSV: только `parse, ns` и `alloc, Mi`,
без `pass` и без `read`. Это согласуется с тем, что:

- read-декодера на тот момент ещё не было (в `MarkedMetric` стоял placeholder `uint8_t bytes[]` без реального сериализатора);
- `Plain Scraper Time` зависит от рабочего read-пути, который тоже ещё не считался.

То есть POC-числа — это ровно те моменты, когда уже сжимали запись и
хотели увидеть, **на сколько падает alloc** и **сколько за это платим
в parse**, не задумываясь про корректность чтения.

## 4. Что и зачем в каждом коммите

### 4.1. POC₁ «labels varint» (часть `4656f7137`)

**Гипотеза.** На реальном Prom-тексте `offset`/`length` для name и value
почти всегда умещаются в 1 байт; offset метрики тоже невелик, если
хранить его относительно начала самой метрики; `__name__` встречается
ровно один раз на метрику и его имя не нужно хранить вовсе.

**Что сделано.**
- Дополнительное поле в `MarkedMetric`: `offset` — глобальное смещение «первого» токена метрики в исходном буфере. Все остальные `MarkedString::offset` хранятся относительно него.
- Каждая `MarkedLabel` сериализуется как:
  - 1 байт **layout descriptor**: `(sz0)|(sz1<<2)|(sz2<<4)|(sz3<<6)`, где каждое `szk ∈ [0..3]` означает «занимает `szk+1` байтов».
  - далее 4 поля `name.offset`, `name.length`, `value.offset`, `value.length`, по 1..4 байта каждое.
  - best case: **5 байт/label** (vs 16); worst case: 17.
- `__name__` детектируется через `is_reserved_name()` → name.offset/length = 0/0 (декодер потом восстановит).
- Количество label'ов — **varint 1..5 байт** (вместо 4 фиксированных).
- Лейблы сначала собираются в скретч-вектор `labels_` (`reserve(255)` на класс), сортируются по name, по ним считается xxhash, и только потом батчем пишутся в основной буфер.
- Снят `metric_buffer_.initialize(buffer.size()/2)` overshoot. Буфер растёт по ходу.

**Цена.** Ветвистая по 4 разрядностям запись + копирование labels через скретч + сортировка. По CSV: parse 38.2 → 45.8 ms (cehp), 22.8 → 28.8 ms (m3). Зато **alloc 13.7 → 5.0 MiB (×2.74)**.

### 4.2. POC₂ «sample encoding» (часть `4656f7137`)

**Гипотеза.** `Sample` 16 B всегда — расточительно: TS обычно
отсутствует (используется default), value — почти всегда маленький
положительный uint, реже — float, изредка — NaN/staleNaN.

**Что сделано.** Один маркер-байт:

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

Best case: **1 байт** на sample (нулевой counter без TS); worst:
17 байт (double + ts) — почти не хуже фиксированных 16.

Ветвление при записи: `isnan` → `==0.0` → `trunc(val)==val && val>0`
(uint-фастпас) → `float32-fits-double` → `double`.

**Числа.** Alloc 5.02 → 3.77 MiB (×3.65). Скорость parse чуть просела:
45.8 → 47.8 ms. Видимо именно из-за добавленных ветвлений в hot loop.

### 4.3. `4656f7137` «initial encoding» — оба POC + первая нормализация

Свели обе схемы вместе и привели код к виду, в котором уже можно
писать read-декодер. Read-декодера в нём ещё нет (`MarkedMetric::occupied_size()`
оставлен с FIXME-комментом и `bytes[]` placeholder), а `read()` ничего
разумного не делает. Поэтому числа Read/Pass для него у тебя «N/A»
в CSV (только parse и alloc).

Замеры на cehp: parse 47.5 ms, alloc 4.15 MiB (×3.31). Это и есть
«дешёвая фаза» — мы максимально экономим память, не заботясь о
скорости чтения.

### 4.4. `0027219f0` «read + passing all tests»

**Главное архитектурное решение.** `MetricMarkupBuffer` перестал быть
одним сплошным `Vector<char>`. Стало два контейнера:

```cpp
BareBones::Vector<MarkedMetric> metric_buffer_;  // фиксированный header per metric
BareBones::Vector<char>         bytes_buffer_;   // variable tail (varint count + labels + sample)
```

`MarkedMetric { uint64 hash; uint32 base_offset; uint32 data_offset; }` — фиксированные **16 байт**.

**Зачем это.**
- `++Iterator` теперь это `++ptr_` по массиву `MarkedMetric*`. До этого нужно было читать `occupied_size()`, который для упакованного формата требовал бы декодировать tail только чтобы посчитать длину.
- На `metric_buffer_` можно сделать `std::sort` без перетаскивания variable tail.
- Hash, base/data offsets лежат плотно — кеш-friendly при чисто «по hash» проходах.

**Кроме этого.**
- Реализован полный `Metric::read(ts)`: декодирует varint count → распаковывает labels по layout-байту → разбирает sample по маркеру. Под `__name__` подставляет `Prometheus::kMetricLabelName`.
- `default_timestamp` поднят на уровень `Scraper` (один на дамп) и подставляется в `read()`, если у sample не было `has_ts`. Это минус 8 байт на «обычную» метрику.

**Числа.** Read-декодер «как написан» — медленнее baseline (16.9 ms vs 10.9 на cehp), потому что цикл декодирования label'а делает `memcpy(&v, ptr, sz+1)` с **переменной длиной**, и компилятор не может развернуть это в простую загрузку. Pass тоже просел (38.7 vs 27.0). Alloc — практически тот же 4.13 MiB. Это «честный» промежуточный шаг: формат заработал end-to-end, но дорого по времени. Тесты идут зелёные.

### 4.5. `9b62e6ae` «read/write optimization» — самый важный коммит по скорости

**Read.**
- `read_val_partial(ptr, sz)` развёрнут на 4 ветки: sz=0 → один байт, sz=1 → `b0|b1<<8`, sz=2 → `b0|b1<<8|b2<<16`, sz=3 → `memcpy 4 байт`. Это убирает `memcpy` с переменным размером.
- `decode_varint` и `read_val_partial` — `PROMPP_ALWAYS_INLINE`.
- Switch по типу sample отсортирован по частоте (`uint8/zero/uint16/uint32/staleNaN/float/double`).
- Для uint16 — ручная сборка из двух байт со сдвигом (а не memcpy).

**Write.** Та же идея, что в read, только наоборот:
- `add_count`: вместо `push_back(tmp_array, len)` стало `bytes_buffer_.resize(size + N); *out = …` — пишем сразу в буфер, без промежуточного массива. Размер `N` известен в каждой ветке.
- `add_label`: `resize(size + 17)` (worst case заранее), пишем 4 поля по `memcpy(out, &val, sz+1)`, в конце `resize(offset + bytes_needed)` — «over-allocate then shrink». Это позволяет компилятору эмитить безусловные `mov` без проверок границ внутри.
- `encode_size(v)` — заменён на `(31 - countl_zero(v)) >> 3` (LZCNT/BSR), вместо каскада `if`-ов.

**Числа.** Cehp: read **16.9 → 7.9 ms (×2.14)**, pass 38.7 → 34.3, parse 50.0 → 46.1. M3: read **4.98 → 1.29 ms (×3.86)**.

### 4.6. `aca48508` «sample optimization»

Применили тот же приём `resize+write+shrink` к `add_sample`:

```cpp
constexpr uint32_t max_sample_bytes = 1 + sizeof(Sample);  // 17
bytes_buffer_.resize(offset + max_sample_bytes);
char* out = ...; *out++ = marker; std::memcpy(out, &val, sizeof(T));
if (has_ts) { memcpy(out, &ts, 8); out += 8; }
bytes_buffer_.resize(offset + (out - start));
```

Плюс: timestamp кодируется **после** value (раньше — до). На декодер
влияет минимально, но позволяет использовать один общий `out` курсор
без условных переустановок.

**Числа.** Cehp: pass 34.3 → 33.6, read небольшой шум. M3: pass 16.9 → 16.3.

### 4.7. `26d0af47` «scraper read benchmark 2 shards»

Только бенчмарк: `if (metric.hash() % 2 == 0) metric.read(ts);`. Это
меняет **базовую цифру** read и pass — здесь у тебя в CSV вторая
«серия» с новым 3f0b2bd72-baseline (33 ms на pass cehp, 11.3 ms m3).
Все три числа после этого замерены в этой новой системе координат.

### 4.8. `2036d7ea` «MetricParser → parse_metric method»

Ранее на каждую метрику конструировался
`MetricParser{parser_, markup_buffer_, labels_, global_offset, default_timestamp}.parse()`.
У вложенного класса с ссылками на 4 объекта компилятор не всегда может
полноценно заинлайнить `parse()`.

После рефакторинга `parse_metric()` — обычный метод `Scraper`, помеченный
`PROMPP_ALWAYS_INLINE`. Все ссылки — это просто члены того же класса.

**Числа.** Pass cehp 34.0 → 33.2, m3 17.4 → 16.3. Виден реальный, хоть и небольшой, прирост.

### 4.9. `1113d04b` «parse_metric refactoring»

Чистая косметика: вынес `process_labels_buffer`, `sort_and_filter_labels`,
`append_labels_hash` из тела `parse_metric` в отдельные методы. Поведение
и численно — то же.

## 5. Что зашло, а что нет

| Коммит | Идея | Память | Скорость parse | Скорость read | Стоит включать в статью |
|---|---|---|---|---|---|
| 4656f7137 (POC labels varint) | переменная упаковка 4 полей label'а + offset relative + дроп `__name__` | ✅ ×2.74 | ⛔ просел | n/a (нет read) | **Да, ключевой шаг** |
| 4656f7137 (POC sample) | 1-байт маркер + 0..8 байт value + опц. ts | ✅ ×3.65 | ⛔ ещё просел | n/a | **Да** |
| 4656f7137 (initial encoding) | свод POC + relative-offset через `base_offset` + дроп overshoot reserve | ✅ ×3.31 (стабильное) | ⛔ медленный pass | ⛔ read «битый» | **Да, как «end of POC»** |
| 0027219f0 | split на `metric_buffer_`+`bytes_buffer_` + рабочий read | = | ⛔ дальше просел | ⛔ медленный | **Да, объясняет архитектуру и трейдоф** |
| 9b62e6ae | read_val_partial unroll, encode_size через LZCNT, resize+write+shrink на write | = | ✅ -10..15% | ✅ ×2.1 (cehp), ×3.9 (m3) | **Да, главный «win-back» по скорости** |
| aca48508 | тот же приём для samples | = | ≈ | ≈ | средне, упомянуть как «продолжение приёма» |
| 26d0af47 | bench 2 shards | n/a | n/a | n/a | методологический момент в статье |
| 2036d7ea | inline-friendly parse_metric | = | ✅ -2..5% | ≈ | **Да, как «чистый» рефактор с замером** |
| 1113d04b | косметика | = | ≈ | ≈ | можно опустить |

**Итог по серии 1 vs baseline (cehp):** alloc ×3.33, read ×1.33, plain pass ×0.81 — то есть по памяти выиграли ~3.3×, по времени read получили буст ~1.3×, но потеряли ~20% на pass.

**Итог по серии 1 vs baseline (m3):** alloc ×3.33, read ×1.42, plain pass ×0.72 — m3 «короткий и плотный», поэтому фиксированные накладные новых форматов видны заметнее.

## 6. Стенд для повторных замеров и трейсов

### 6.1. Что нужно зафиксировать

1. Один и тот же бенчмарк-cpp на всех коммитах. Текущий `pp/wal/benchmarks/scraper_benchmark.cpp` уже идеально подходит — он есть в исходном виде в каждом из 8 коммитов после `3f0b2bd72`. Чтобы не зависеть от его эволюции, копируем последнюю версию (с 2-shards read) во временное место и применяем перед сборкой каждой ревизии.
2. Один и тот же набор фикстур. Сейчас в репо лежит только `pp/kube-api.metrics`. Для статьи минимум 2 файла:
   - «cehp-runner-like» — длинные labels, разнообразные значения (можно использовать текущий `kube-api.metrics`).
   - «m3-like» — плотный, в основном integer counters (можно сделать дамп с `node_exporter` или `cadvisor`, либо синтезировать).
   - Опционально третий — пограничный (много float-ов с timestamp'ами).
3. `-c opt --copt=-march=native`, без ASan, без TSan. `pp-workflow.mdc` явно требует `-c opt` для перфметрик.
4. Tracy уже завязан в код (`profiling/profiling.h`, `ZoneScoped` в обеих бенч-функциях). Достаточно собрать с `--//bazel/toolchain:profiling=1` и подключиться `tracy-profiler`-ом.

### 6.2. Скрипт прогона по ревизиям (черновик)

```bash
# bench_sweep.sh — запускать из /prompp/pp
set -euo pipefail
COMMITS=(
  3f0b2bd72  # baseline
  4656f7137  # initial encoding (POC merged)
  0027219f0  # read + passing
  9b62e6ae   # read/write opt
  aca48508   # sample opt
  26d0af47   # 2 shards bench
  2036d7ea   # parse_metric method
  1113d04b   # parse_metric refactor
)

FIXTURES=(
  "cehp:performance_tests/test_data/cehp.metrics"
  "m3:performance_tests/test_data/m3.metrics"
)

mkdir -p bench_out
cp wal/benchmarks/scraper_benchmark.cpp /tmp/scraper_benchmark.cpp.frozen

for h in "${COMMITS[@]}"; do
  git checkout -q "$h"
  cp /tmp/scraper_benchmark.cpp.frozen wal/benchmarks/scraper_benchmark.cpp

  bazel build -c opt --copt=-march=native //wal/benchmarks:scraper

  for kv in "${FIXTURES[@]}"; do
    name="${kv%%:*}"; file="${kv##*:}"
    out="bench_out/${h}_${name}.json"
    ./bazel-bin/wal/benchmarks/scraper \
      --benchmark_repetitions=10 \
      --benchmark_min_time=1s \
      --benchmark_time_unit=ns \
      --benchmark_format=json \
      --benchmark_out="$out" \
      --benchmark_context=prom_scraper_file="$file" \
      --benchmark_filter='Parser|ScraperParse|ScraperRead'
  done

  git checkout -q -- wal/benchmarks/scraper_benchmark.cpp
done

git checkout -q -
```

Парсилка JSON-ов в одну CSV — отдельный шаг (`benchmark_repetitions=10` + `aggregate_name == "min"`).

### 6.3. Tracy-прогоны для статьи

| Ревизия | Что показать | Идея для скриншота |
|---|---|---|
| `3f0b2bd72` | baseline | один большой `ZoneScoped` в `ScraperParse`, плоско |
| `4656f7137` | POC: память упала, скорость просела | те же зоны, но видно широкие зелёные интервалы внутри `parse_metric` (ветвление в `add_label` / `add_sample`) |
| `0027219f0` | read заработал, но медленный | `ScraperRead` зона: показать дорогой `memcpy` с переменной длиной |
| `9b62e6ae` | главный win-back | те же `read_label` / `add_label` зоны, но в 2–3× уже |
| `2036d7ea` | inlining | пропал отдельный кадр конструктора `MetricParser` |

Сборка под Tracy:
```bash
bazel build -c opt --copt=-march=native --//bazel/toolchain:profiling=1 //wal/benchmarks:scraper
```

В коде уже стоят `ZoneScoped` в `Parser`/`ScraperParse`/`ScraperRead`, и
есть закомментированные `ZoneScopedN("Scraper::parse")` /
`ZoneScopedN("MetricParser::parse")` / `ZoneScopedN("Metric::read()")` в
`scraper.h` (видно в `9b62e6ae` / `aca48508` диффах). Для трейсов их
нужно включить локально — это даст разбивку по фазам.

Полезно ещё добавить более тонкие зоны (на время статьи, не в master):

- внутри `parse_metric`: `ZoneScopedN("collect_labels")`, `ZoneScopedN("sort+hash")`, `ZoneScopedN("encode_count")`, `ZoneScopedN("encode_labels")`, `ZoneScopedN("encode_sample")`;
- внутри `Metric::read`: `ZoneScopedN("decode_count")`, `ZoneScopedN("decode_labels")`, `ZoneScopedN("decode_sample")`.

С такими зонами трейс на 9b62e6ae против 0027219f0 наглядно покажет,
что выигрыш сидит именно в `decode_labels` и `encode_labels`/`encode_sample`.

### 6.4. Валидация корректности на каждом шаге

`pp-testing.mdc` требует ASan-прогон тестов перед мерджем; для статьи
это тоже подстраховка от скрытых багов в записи/чтении формата:

```bash
bazel test -c dbg --asan //wal/hashdex/scraper:scraper_test
```

Для `4656f7137` тесты падают (по описанию коммита, тесты «прошли все»
только в `0027219f0`). Это можно использовать честно — «после POC
тесты красные, мы сначала делаем рабочую сериализацию, потом
разгоняем».

### 6.5. Что ещё имеет смысл достать перед публикацией

- **Распределение размеров полей** на тестовых дампах: гистограмма `name.length`, `value.length`, `offset_relative_to_metric` — чтобы доказать читателю гипотезу «всё умещается в 1–2 байта». Это `xxd`/python-скрипт по файлу.
- **Распределение sample-типов**: сколько uint8/uint16/uint32/double/NaN. Это и обоснует выбор маркеров.
- **Ratio**: средний размер метрики до/после в байтах. Можно прямо в бенчмарке вывести `bytes_count() / metric_count`.

## 7. CSV из исходных замеров (нормализованная)

Колонки: `Parse` — `BenchmarkScraperParse, ns`; `Pass` — `Plain Scraper Time, ns` (parse - parser); `Read` — `BenchmarkScraperRead, ns`; `Alloc` — `ScraperAllocMem, Mi`. В скобках — отношение к baseline.

### Серия 1 (baseline `3f0b2bd72`)

#### cehp-runner

| Commit | Key change | Parse ns (×) | Pass ns (×) | Read ns (×) | Alloc Mi (×) |
|---|---|---:|---:|---:|---:|
| `3f0b2bd72` | baseline | 38171007 (1.00) | 27080951 (1.00) | 10927821 (1.00) | 13.746 (1.00) |
| `labels varint` | labels varint | 45775620 (0.83) | n/a | n/a | 5.01953 (2.74) |
| `sample encoding` | sample encoding | 47803708 (0.80) | n/a | n/a | 3.76953 (3.65) |
| `4656f7137` | initial encoding | 47564602 (0.80) | n/a | n/a | 4.14844 (3.31) |
| `0027219f0` | reading + extra offset added | 50035042 (0.76) | 38723316 (0.70) | 16912478 (0.65) | 4.12891 (3.33) |
| `9b62e6ae` | r/w optimizations | 46106593 (0.83) | 34340283 (0.79) | 7887484 (1.39) | 4.12891 (3.33) |
| `aca48508` | sample opt | 44739040 (0.85) | 33586988 (0.81) | 8198244 (1.33) | 4.12891 (3.33) |

#### m3

| Commit | Key change | Parse ns (×) | Pass ns (×) | Read ns (×) | Alloc Mi (×) |
|---|---|---:|---:|---:|---:|
| `3f0b2bd72` | baseline | 22774389 (1.00) | 11749549 (1.00) | 1791641 (1.00) | 13.746 (1.00) |
| `labels varint` | labels varint | 28791658 (0.79) | n/a | n/a | 5.01953 (2.74) |
| `sample encoding` | sample encoding | 31195012 (0.73) | n/a | n/a | 3.76953 (3.65) |
| `4656f7137` | initial encoding | 28740757 (0.79) | n/a | n/a | 4.14844 (3.31) |
| `0027219f0` | reading + 2-shards bench update | 26255763 (0.87) | 14931157 (0.79) | 4980513 (0.36) | 4.12891 (3.33) |
| `9b62e6ae` | r/w optimizations | 28865275 (0.79) | 16914917 (0.69) | 1287539 (1.39) | 4.12891 (3.33) |
| `aca48508` | sample opt | 28900335 (0.79) | 16259238 (0.72) | 1265548 (1.42) | 4.12891 (3.33) |

### Серия 2 (новый baseline `3f0b2bd72` после `26d0af47`)

#### cehp-runner

| Commit | Key change | Parse ns (×) | Pass ns (×) | Read ns (×) | Alloc Mi (×) |
|---|---|---:|---:|---:|---:|
| `3f0b2bd72` | baseline | 43942332 (1.00) | 32910757 (1.00) | 6080402 (1.00) | 13.746 (1.00) |
| `26d0af47` | 2 shards bench / state after sample opt | 45321131 (0.97) | 34085663 (0.97) | 3535429 (1.72) | 4.129 (3.33) |
| `2036d7ea` | MetricParser → parse_metric method | 44676447 (0.98) | 33235111 (0.99) | 3896916 (1.56) | 4.129 (3.33) |
| `1113d04b` | parse_metric refactoring | 45107031 (0.97) | 33783974 (0.97) | 3791985 (1.60) | 4.129 (3.33) |

#### m3

| Commit | Key change | Parse ns (×) | Pass ns (×) | Read ns (×) | Alloc Mi (×) |
|---|---|---:|---:|---:|---:|
| `3f0b2bd72` | baseline | 22985376 (1.00) | 11277242 (1.00) | 1245206 (1.00) | 13.746 (1.00) |
| `26d0af47` | 2 shards bench / state after sample opt | 29213092 (0.79) | 17359001 (0.65) | 796406 (1.56) | 4.129 (3.33) |
| `2036d7ea` | MetricParser → parse_metric method | 27959700 (0.82) | 16263446 (0.69) | 918037 (1.36) | 4.129 (3.33) |
| `1113d04b` | parse_metric refactoring | 28861682 (0.80) | 17450605 (0.65) | 810836 (1.54) | 4.129 (3.33) |

## 8. Open questions / TODO перед публикацией

- [ ] Воспроизвести замеры на свежей машине, на которой будут писаться остальные графики статьи (одна и та же CPU, одна и та же сборка `-c opt --copt=-march=native`).
- [ ] Создать/добавить в репо два публикуемых дампа фикстур (`cehp.metrics`, `m3.metrics`) либо описать, как их получить (например, `curl http://kube-apiserver:443/metrics`).
- [ ] Снять Tracy-трейсы на 5 ревизиях из таблицы 6.3.
- [ ] Сделать гистограммы распределения `name.length` / `value.length` / `value type` на двух фикстурах.
- [ ] Прогнать `bazel test -c dbg --asan //wal/hashdex/scraper:scraper_test` на каждой ревизии, отметить, начиная с какой проходит.

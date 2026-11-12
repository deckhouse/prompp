We use re2c as lexer generator for parse prometheus text metrics format.

## Installation

```bash
apt install re2c=4.6-1
```

## Prometheus lexer code generation

```bash
re2c prometheus/textparse/prometheus/tokenizer.cxx -o prometheus/textparse/prometheus/tokenizer.cpp --no-debug-info --conditions --no-generation-date --no-version -Werror
clang-format -i prometheus/textparse/prometheus/tokenizer.cpp
```

## Open metrics Lexer code generation

```bash
re2c prometheus/textparse/open_metrics/tokenizer.cxx -o prometheus/textparse/open_metrics/tokenizer.cpp --no-debug-info --conditions --no-generation-date --no-version -Werror
clang-format -i prometheus/textparse/open_metrics/tokenizer.cpp
```

## End of input

Both lexers use the generic API with `re2c:eof = 0`: `YYPEEK` returns `0` past the end of the input and the lexer
checks the real end of the input only when it reads this sentinel. The input does not need a terminator or
padding, and a body that ends in the middle of a token never makes the lexer read past the buffer.

`re2c:eof` cannot be combined with `--bit-vectors` and `--computed-gotos-relative`, so these options are not used.
Dropping them costs about 10% of the tokenizer time; the whole scraper parse stays within noise.

## Links

[re2c home](https://re2c.org/)

[re2c manual C](https://re2c.org/manual/manual_c.html)

## Benchmark

We have tokenized a 82KB prometheus metrics text file using several lexer generators.

| Generator | Speed (microseconds) |
|-----------|----------------------|
| flex      | ~551                 |
| re2c      | ~76                  |

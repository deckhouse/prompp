extern "C" __attribute__((annotate("prompp.entrypoint.fastcgo"))) void prompp_store(void* args, void* res) {
  struct Arguments {
    int series;
  };
  struct Result {
    double value;
  };
}

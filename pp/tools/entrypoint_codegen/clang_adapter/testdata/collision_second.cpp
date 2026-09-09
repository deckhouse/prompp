static int helper() {
  return 2;
}

extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void prompp_second() {
  (void)helper();
}

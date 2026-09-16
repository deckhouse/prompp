static int helper() {
  return 1;
}

extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void prompp_first() {
  (void)helper();
}

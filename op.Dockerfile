FROM golang:1.21.3-bullseye as builder
WORKDIR /prometheus
COPY . /prometheus
RUN CGO_ENABLED=0 GOOS=linux go build -o build/prometheus ./cmd/prometheus
RUN CGO_ENABLED=0 GOOS=linux go build -o build/promtool ./cmd/promtool

FROM prom/prometheus:v2.50.1
COPY --from=builder /prometheus/build/prometheus /bin/prometheus
COPY --from=builder /prometheus/build/promtool /bin/promtool

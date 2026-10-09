module github.com/prometheus/prometheus/web/ui/mantine-ui/src/promql/tools

go 1.26.0

require (
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853
	github.com/prometheus/prometheus v0.0.0-00010101000000-000000000000
	github.com/russross/blackfriday/v2 v2.1.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

replace cloud.google.com/go => cloud.google.com/go v0.123.0

// Generate the signatures and docs from this repository's PromQL parser rather than upstream's.
replace github.com/prometheus/prometheus => ../../../../../..

// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scrape

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

// FuzzTargetsFromGroup fuzzes target construction: the discovered label sets of
// a target group, the scrape config and its relabel rules go in, and the
// [*Target] list the scrape pool would then scrape comes out.
//
// The byte string is not a scrape body here — it drives a small decoder that
// assembles the label sets and the relabel configs (see [structure]). Feeding
// the fuzzer raw YAML would spend all of its budget inside the YAML parser,
// while the interesting behaviour is in relabelling, address handling and the
// interval/timeout rules.
//
// What is asserted is what the rest of the scrape package assumes about a
// target it is handed:
//
//   - every returned target is scrapeable: an address, and a URL with a host;
//   - the interval and timeout labels always parse, and the timeout never
//     exceeds the interval, since the scrape loop derives its timers from them;
//   - the sample limit annotation is never negative — it crosses into C++ as an
//     unsigned value, so a negative one would silently disable the limit;
//   - a target is only returned if it is either active or dropped-but-reportable
//     (non-empty discovered labels), otherwise the scrape pool would show a
//     target it can neither scrape nor explain;
//   - the same group and config produce the same targets, which the map
//     iteration in TargetsFromGroup would break if the result depended on it.
func FuzzTargetsFromGroup(f *testing.F) {
	// Seeds spanning the decoder's choices: an empty group, plain targets, and
	// byte strings dense enough to reach the relabel configs.
	f.Add([]byte(""), false)
	f.Add([]byte("\x01\x00\x00"), false)
	f.Add([]byte("\x02\x01\x02\x03\x04\x05\x06\x07\x08"), true)
	f.Add([]byte("\x03\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15"), false)
	f.Add([]byte("\xff\xfe\xfd\xfc\xfb\xfa\xf9\xf8\xf7\xf6\xf5\xf4\xf3\xf2"), true)
	f.Add([]byte("localhost:9090\x00\x01\x02"), false)

	f.Fuzz(func(t *testing.T, in []byte, noDefaultPort bool) {
		group, cfg, ok := decodeTargetGroup(in)
		if !ok {
			t.Skip("input does not describe a usable scrape config")
		}

		targets, failures := targetsFromGroup(t, group, cfg, noDefaultPort)

		for _, failure := range failures {
			if failure == nil {
				t.Fatal("TargetsFromGroup returned a nil failure")
			}
		}
		if len(targets) > len(group.Targets) {
			t.Fatalf("got %d targets out of %d discovered label sets", len(targets), len(group.Targets))
		}

		for _, target := range targets {
			assertTargetIsUsable(t, target)
		}

		// TargetsFromGroup iterates over the discovered label sets, which are
		// maps, so a result that depends on their iteration order would make
		// the scrape pool churn targets on every sync.
		again, againFailures := targetsFromGroup(t, group, cfg, noDefaultPort)
		if got, want := describeTargets(again), describeTargets(targets); got != want {
			t.Fatalf("targets are not reproducible:\nfirst:  %s\nsecond: %s", want, got)
		}
		if got, want := len(againFailures), len(failures); got != want {
			t.Fatalf("failure count is not reproducible: first %d, second %d", want, got)
		}
	})
}

// targetsFromGroup builds the targets with a label builder of its own, so that
// nothing carries over between the two calls the fuzzer compares.
func targetsFromGroup(
	t *testing.T,
	group *targetgroup.Group,
	cfg *config.ScrapeConfig,
	noDefaultPort bool,
) ([]*Target, []error) {
	t.Helper()

	return TargetsFromGroup(group, cfg, noDefaultPort, nil, labels.NewBuilder(labels.EmptyLabels()))
}

// assertTargetIsUsable checks the properties the scrape pool and the scrape loop
// rely on when they are handed a target.
func assertTargetIsUsable(t *testing.T, target *Target) {
	t.Helper()

	if limit := target.SampleLimit(); limit < 0 {
		t.Fatalf("target %s has a negative sample limit %d (label %q)",
			target, limit, target.GetValue("__sample_limit__"))
	}

	// The internal label set, unlike Labels(), still has the __-prefixed labels
	// that decide whether the target is scraped at all.
	activeLabels := target.labels
	if activeLabels.IsEmpty() {
		// A dropped target: it is only kept around to be reported, so it needs
		// the discovered labels to report and nothing else.
		if target.DiscoveredLabels().IsEmpty() {
			t.Fatal("TargetsFromGroup returned a target with no active and no discovered labels")
		}

		return
	}

	if address := target.GetValue(model.AddressLabel); address == "" {
		t.Fatalf("active target %s has no %s label: %s", target, model.AddressLabel, activeLabels)
	}
	if host := target.URL().Host; host == "" {
		t.Fatalf("active target %s has a URL without a host: %s", target, target.URL())
	}

	// The scrape loop turns these two into its ticker and its request deadline
	// on every reload, so PopulateLabels has to have left them parseable.
	interval, timeout, err := target.intervalAndTimeout(time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("active target %s has unusable interval/timeout labels: %s", target, err)
	}
	if interval <= 0 {
		t.Fatalf("active target %s has a scrape interval of %s", target, interval)
	}
	if timeout > interval {
		t.Fatalf("active target %s has timeout %s greater than interval %s", target, timeout, interval)
	}
}

// describeTargets renders the targets in a form that can be compared between
// two runs on the same input.
func describeTargets(targets []*Target) string {
	described := make([]string, 0, len(targets))
	for _, target := range targets {
		described = append(described, fmt.Sprintf("{active=%s discovered=%s url=%s}",
			target.labels, target.DiscoveredLabels(), target.URL()))
	}

	return strings.Join(described, " ")
}

// Value pools for the decoder. They mix the labels and values that carry
// meaning for target construction with shapes that have broken address or
// duration handling before, since the fuzzer is unlikely to spell out
// "__scrape_interval__" on its own.
var (
	fuzzLabelNames = []string{
		model.AddressLabel,
		model.SchemeLabel,
		model.MetricsPathLabel,
		model.ScrapeIntervalLabel,
		model.ScrapeTimeoutLabel,
		model.JobLabel,
		model.InstanceLabel,
		"__sample_limit__",
		model.ParamLabelPrefix + "target",
		model.MetaLabelPrefix + "kubernetes_pod_name",
		"__tmp_internal",
		"team",
		"pod",
	}

	fuzzLabelValues = []string{
		"",
		"localhost:9090",
		"localhost",
		"127.0.0.1:9090",
		"[::1]:9090",
		"[::1]",
		"example.com:80",
		"example.com:443",
		"example.com:not-a-port",
		"example.com:99999",
		":9090",
		"http://example.com:9090",
		"http://example.com:9090/metrics",
		"http",
		"https",
		"ftp",
		"/metrics",
		"metrics",
		"0",
		"1",
		"-1",
		"0s",
		"1s",
		"15s",
		"1m",
		"100000000d",
		"1y",
		"not-a-duration",
		"9223372036854775807",
		"-9223372036854775808",
		"99999999999999999999",
		"\xff\xfe",
		"a b",
		"a.b",
		"$1",
	}

	fuzzRegexes = []string{
		"",
		".*",
		"(.*)",
		"(.+):(.+)",
		"__meta_(.*)",
		"team",
		"__tmp_.*",
		"[",
	}

	fuzzReplacements = []string{
		"$1",
		"${1}",
		"$2",
		"$0",
		"",
		"fixed",
		"$1-$2",
		"__meta_$1",
		"a.b",
	}

	fuzzActions = []relabel.Action{
		relabel.Replace,
		relabel.Keep,
		relabel.Drop,
		relabel.HashMod,
		relabel.LabelMap,
		relabel.LabelDrop,
		relabel.LabelKeep,
		relabel.Lowercase,
		relabel.Uppercase,
		relabel.KeepEqual,
		relabel.DropEqual,
	}

	fuzzDurations = []model.Duration{
		model.Duration(0),
		model.Duration(time.Second),
		model.Duration(10 * time.Second),
		model.Duration(time.Minute),
		model.Duration(time.Hour),
	}
)

// decodeTargetGroup assembles a target group and a scrape config out of the
// fuzzer's byte string. A config the YAML loader would have rejected is
// reported as unusable rather than fixed up: those configs cannot reach
// TargetsFromGroup in production, and relabelling one panics by design.
func decodeTargetGroup(in []byte) (*targetgroup.Group, *config.ScrapeConfig, bool) {
	s := &structure{data: in}

	cfg := &config.ScrapeConfig{
		JobName:        s.pick([]string{"job", "", "a b"}),
		MetricsPath:    s.pick([]string{"/metrics", "", "metrics", "/a b"}),
		Scheme:         s.pick([]string{"http", "https", "", "ftp"}),
		ScrapeInterval: fuzzDurations[s.index(len(fuzzDurations))],
		ScrapeTimeout:  fuzzDurations[s.index(len(fuzzDurations))],
	}
	if s.flag() {
		cfg.Params = url.Values{"target": []string{s.value()}}
	}

	for range s.index(4) {
		cfg.RelabelConfigs = append(cfg.RelabelConfigs, s.relabelConfig())
	}
	for _, relabelConfig := range cfg.RelabelConfigs {
		if err := relabelConfig.Validate(); err != nil {
			return nil, nil, false
		}
	}

	group := &targetgroup.Group{Source: "fuzz"}
	for range s.index(3) + 1 {
		group.Targets = append(group.Targets, s.labelSet())
	}
	if s.flag() {
		group.Labels = s.labelSet()
	}

	return group, cfg, true
}

// structure decodes a fuzzer byte string into the values a scrape config is
// built from. It never fails: once the bytes run out every choice falls back to
// the first, which keeps the mapping from input to config stable as the fuzzer
// mutates a prefix.
type structure struct {
	data []byte
	pos  int
}

func (s *structure) next() byte {
	if s.pos >= len(s.data) {
		return 0
	}
	s.pos++

	return s.data[s.pos-1]
}

// index returns a value in [0, n).
func (s *structure) index(n int) int {
	if n <= 0 {
		return 0
	}

	return int(s.next()) % n
}

func (s *structure) flag() bool {
	return s.next()%2 == 1
}

// pick returns one of the options.
func (s *structure) pick(options []string) string {
	return options[s.index(len(options))]
}

// value returns either a pooled label value or, so that the fuzzer is not
// confined to the pool, a run of raw bytes from the input.
func (s *structure) value() string {
	if index := s.index(len(fuzzLabelValues) + 1); index < len(fuzzLabelValues) {
		return fuzzLabelValues[index]
	}

	length := s.index(12)
	start := min(s.pos, len(s.data))
	end := min(start+length, len(s.data))
	s.pos = end

	return string(s.data[start:end])
}

func (s *structure) labelSet() model.LabelSet {
	set := model.LabelSet{}
	for range s.index(5) {
		name := fuzzLabelNames[s.index(len(fuzzLabelNames))]
		set[model.LabelName(name)] = model.LabelValue(s.value())
	}

	return set
}

func (s *structure) relabelConfig() *relabel.Config {
	cfg := relabel.DefaultRelabelConfig
	cfg.Action = fuzzActions[s.index(len(fuzzActions))]

	for range s.index(3) {
		cfg.SourceLabels = append(cfg.SourceLabels, model.LabelName(fuzzLabelNames[s.index(len(fuzzLabelNames))]))
	}
	cfg.TargetLabel = fuzzLabelNames[s.index(len(fuzzLabelNames))]
	cfg.Modulus = uint64(s.index(4))

	// DropEqual, KeepEqual, LabelDrop and LabelKeep only accept the defaults for
	// the remaining fields, so leaving them alone keeps those actions reachable.
	switch cfg.Action {
	case relabel.DropEqual, relabel.KeepEqual:
		cfg.SourceLabels = cfg.SourceLabels[:min(1, len(cfg.SourceLabels))]
	case relabel.LabelDrop, relabel.LabelKeep:
		cfg.SourceLabels = nil
		cfg.TargetLabel = relabel.DefaultRelabelConfig.TargetLabel
		cfg.Regex = s.regexp()
	default:
		cfg.Regex = s.regexp()
		cfg.Replacement = fuzzReplacements[s.index(len(fuzzReplacements))]
	}

	return &cfg
}

// regexp compiles a pooled pattern, falling back to the default when the
// pattern does not compile — an invalid regex cannot survive config loading.
func (s *structure) regexp() relabel.Regexp {
	compiled, err := relabel.NewRegexp(fuzzRegexes[s.index(len(fuzzRegexes))])
	if err != nil {
		return relabel.DefaultRelabelConfig.Regex
	}

	return compiled
}

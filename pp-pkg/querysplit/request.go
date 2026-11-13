package querysplit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Header is the HTTP header that turns on the split of one request and sets its settings. The
// value is a list of comma-separated name=value options, for example
// "query_range_split_parallelism=4,query_range_split_min_series=5000". An empty value selects the
// default settings, and a parallelism of 1 turns the split off.
const Header = "X-PromPP-Query-Split"

// DefaultParallelism is the default maximum number of sub-queries of one query that run at the
// same time.
const DefaultParallelism = 4

// Settings are the split settings of one query. A zero field takes the default value.
type Settings struct {
	Parallelism int
	MergeFactor int
	MinBlocks   int
	MinSeries   int
}

// ParseSettings parses the value of Header.
func ParseSettings(value string) (Settings, error) {
	var s Settings
	for _, option := range strings.Split(value, ",") {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}

		name, raw, ok := strings.Cut(option, "=")
		if !ok {
			return Settings{}, fmt.Errorf("option %q has no value", option)
		}

		v, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || v < 1 {
			return Settings{}, fmt.Errorf("option %q must be a positive integer", option)
		}

		switch strings.TrimSpace(name) {
		case "query_range_split_parallelism":
			s.Parallelism = v
		case "query_range_split_merge_factor":
			s.MergeFactor = v
		case "query_range_split_min_blocks":
			s.MinBlocks = v
		case "query_range_split_min_series":
			s.MinSeries = v
		default:
			return Settings{}, fmt.Errorf("unknown option %q", name)
		}
	}

	return s, nil
}

// overlay returns s with the non-zero fields of o.
func (s Settings) overlay(o Settings) Settings {
	if o.Parallelism > 0 {
		s.Parallelism = o.Parallelism
	}
	if o.MergeFactor > 0 {
		s.MergeFactor = o.MergeFactor
	}
	if o.MinBlocks > 0 {
		s.MinBlocks = o.MinBlocks
	}
	if o.MinSeries > 0 {
		s.MinSeries = o.MinSeries
	}

	return s
}

type settingsKey struct{}

// WithSettings returns a copy of ctx with the split settings of one request.
func WithSettings(ctx context.Context, s Settings) context.Context {
	return context.WithValue(ctx, settingsKey{}, s)
}

// settingsFrom returns the split settings of the request, if the request has them.
func settingsFrom(ctx context.Context) (Settings, bool) {
	s, ok := ctx.Value(settingsKey{}).(Settings)
	return s, ok
}

// Instrument is a route instrumentation that puts the settings from Header into the request
// context. It answers 400 if the header value is not correct.
func Instrument(_ string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, ok := r.Header[http.CanonicalHeaderKey(Header)]
		if !ok {
			next(w, r)
			return
		}

		s, err := ParseSettings(strings.Join(value, ","))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":    "error",
				"errorType": "bad_data",
				"error":     fmt.Sprintf("invalid %s header: %s", Header, err),
			})
			return
		}

		next(w, r.WithContext(WithSettings(r.Context(), s)))
	}
}

package cppbridge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/common/model"
)

// NullTimestamp the timestamp that is used as the nil value.
const NullTimestamp = math.MinInt64

// ErrLSSNullPointer - error when lss is null pointer
var ErrLSSNullPointer = errors.New("lss is null pointer")

//
// Config for relabeling.
//

var (
	// RelabelTarget - validate Target label.
	RelabelTarget = regexp.MustCompile(`^(?:(?:[a-zA-Z_]|\$(?:\{\w+\}|\w+))+\w*)+$`)

	defaultRelabelConfig = RelabelConfig{
		Action:      Replace,
		Separator:   ";",
		Regex:       "(.*)",
		Replacement: "$1",
	}

	invalidTargetLabelForAction = "%q is invalid 'target_label' for %s action"
)

// RelabelConfig - is the configuration for relabeling of target label sets.
type RelabelConfig struct {
	// A list of labels from which values are taken and concatenated with the configured separator in order.
	SourceLabels []string `yaml:"source_labels,flow,omitempty"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `yaml:"separator,omitempty"`
	// Regex against which the concatenation is matched.
	Regex string `yaml:"regex,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `yaml:"modulus,omitempty"`
	// TargetLabel is the label to which the resulting string is written in a replacement.
	// Regexp interpolation is allowed for the replace action.
	TargetLabel string `yaml:"target_label,omitempty"`
	// Replacement is the regex replacement pattern to be used.
	Replacement string `yaml:"replacement,omitempty"`
	// Action is the action to be performed for the relabeling.
	Action Action `yaml:"action,omitempty"`
}

// UnmarshalYAML - implements the yaml.Unmarshaler interface.
func (c *RelabelConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = defaultRelabelConfig
	type plain RelabelConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Validate - validate config.
//
//revive:disable-next-line:cyclomatic this is validate.
//revive:disable-next-line:function-length long but readable.
//revive:disable-next-line:cognitive-complexity function is not complicated.
func (c *RelabelConfig) Validate() error {
	if c.Action == NoAction {
		return fmt.Errorf("relabel action cannot be empty")
	}
	if c.Modulus == 0 && c.Action == HashMod {
		return fmt.Errorf("relabel configuration for hashmod requires non-zero modulus")
	}
	if (c.Action == Replace ||
		c.Action == HashMod ||
		c.Action == Lowercase ||
		c.Action == Uppercase ||
		c.Action == KeepEqual ||
		c.Action == DropEqual) && c.TargetLabel == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'target_label' value", c.Action)
	}
	if c.Action == Replace && !strings.Contains(c.TargetLabel, "$") && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}
	if c.Action == Replace && strings.Contains(c.TargetLabel, "$") && !RelabelTarget.MatchString(c.TargetLabel) {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase ||
		c.Action == Uppercase ||
		c.Action == KeepEqual ||
		c.Action == DropEqual) && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase ||
		c.Action == Uppercase ||
		c.Action == KeepEqual ||
		c.Action == DropEqual) && c.Replacement != defaultRelabelConfig.Replacement {
		return fmt.Errorf("'replacement' can not be set for %s action", c.Action)
	}
	if c.Action == LabelMap && !RelabelTarget.MatchString(c.Replacement) {
		return fmt.Errorf("%q is invalid 'replacement' for %s action", c.Replacement, c.Action)
	}
	if c.Action == HashMod && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf(invalidTargetLabelForAction, c.TargetLabel, c.Action)
	}

	if c.Action == DropEqual || c.Action == KeepEqual {
		if c.Regex != defaultRelabelConfig.Regex ||
			c.Modulus != defaultRelabelConfig.Modulus ||
			c.Separator != defaultRelabelConfig.Separator ||
			c.Replacement != defaultRelabelConfig.Replacement {
			return fmt.Errorf(
				"%s action requires only 'source_labels' and `target_label`, and no other fields",
				c.Action,
			)
		}
	}

	if c.Action == LabelDrop || c.Action == LabelKeep {
		if c.SourceLabels != nil ||
			c.TargetLabel != defaultRelabelConfig.TargetLabel ||
			c.Modulus != defaultRelabelConfig.Modulus ||
			c.Separator != defaultRelabelConfig.Separator ||
			c.Replacement != defaultRelabelConfig.Replacement {
			return fmt.Errorf("%s action requires only 'regex', and no other fields", c.Action)
		}
	}

	return nil
}

// Equal check for complete coincidence of values.
func (c *RelabelConfig) Equal(input *RelabelConfig) bool {
	if len(c.SourceLabels) != len(input.SourceLabels) {
		return false
	}

	for j := range c.SourceLabels {
		if c.SourceLabels[j] != input.SourceLabels[j] {
			return false
		}
	}

	if c.Separator != input.Separator {
		return false
	}

	if c.Regex != input.Regex {
		return false
	}

	if c.Modulus != input.Modulus {
		return false
	}

	if c.TargetLabel != input.TargetLabel {
		return false
	}

	if c.Replacement != input.Replacement {
		return false
	}

	if c.Action != input.Action {
		return false
	}

	return true
}

// Copy return copy *RelabelConfig.
func (c *RelabelConfig) Copy() *RelabelConfig {
	newCfg := &RelabelConfig{
		SourceLabels: make([]string, 0, len(c.SourceLabels)),
		Separator:    c.Separator,
		Regex:        c.Regex,
		Modulus:      c.Modulus,
		TargetLabel:  c.TargetLabel,
		Replacement:  c.Replacement,
		Action:       c.Action,
	}
	newCfg.SourceLabels = append(newCfg.SourceLabels, c.SourceLabels...)
	return newCfg
}

// Action - is the action to be performed on relabeling.
type Action uint8

const (
	// NoAction - no action, init state.
	NoAction Action = iota
	// Drop - drops targets for which the input does match the regex.
	Drop
	// Keep - drops targets for which the input does not match the regex.
	Keep
	// DropEqual - drops targets for which the input does match the target.
	DropEqual
	// KeepEqual - drops targets for which the input does not match the target.
	KeepEqual
	// Replace - performs a regex replacement.
	Replace
	// Lowercase - maps input letters to their lower case.
	Lowercase
	// Uppercase - maps input letters to their upper case.
	Uppercase
	// HashMod - sets a label to the modulus of a hash of labels.
	HashMod
	// LabelMap - copies labels to other labelnames based on a regex.
	LabelMap
	// LabelDrop - drops any label matching the regex.
	LabelDrop
	// LabelKeep - drops any label not matching the regex.
	LabelKeep
)

// ActionNameToValueMap - converting Action string name to Action value.
var ActionNameToValueMap = map[string]Action{
	"drop":      Drop,
	"keep":      Keep,
	"dropequal": DropEqual,
	"keepequal": KeepEqual,
	"replace":   Replace,
	"lowercase": Lowercase,
	"uppercase": Uppercase,
	"hashmod":   HashMod,
	"labelmap":  LabelMap,
	"labeldrop": LabelDrop,
	"labelkeep": LabelKeep,
}

// actionValueToNameMap - converting Action value to Action string name.
var actionValueToNameMap = map[Action]string{
	Drop:      "drop",
	Keep:      "keep",
	DropEqual: "dropequal",
	KeepEqual: "keepequal",
	Replace:   "replace",
	Lowercase: "lowercase",
	Uppercase: "uppercase",
	HashMod:   "hashmod",
	LabelMap:  "labelmap",
	LabelDrop: "labeldrop",
	LabelKeep: "labelkeep",
}

// String - serialize to string.
func (a Action) String() string {
	v, ok := actionValueToNameMap[a]
	if !ok {
		return fmt.Sprintf("Action(%d)", a)
	}

	return v
}

// UnmarshalYAML - implements the yaml.Unmarshaler interface.
func (a *Action) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	v, ok := ActionNameToValueMap[strings.ToLower(s)]
	if !ok {
		return fmt.Errorf("unknown relabel action %q", s)
	}
	*a = v

	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (a Action) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

// ToHash make hash from RelabelConfig's.
func ToHash(rCfgs []*RelabelConfig) uint64 {
	h := xxhash.New()
	for _, rcfg := range rCfgs {
		for _, sl := range rcfg.SourceLabels {
			_, _ = h.WriteString(sl)
		}
		_, _ = h.WriteString(rcfg.Separator)
		_, _ = h.WriteString(rcfg.Regex)
		_, _ = h.WriteString(strconv.FormatUint(rcfg.Modulus, 10)) //revive:disable-line:add-constant it's base 10
		_, _ = h.WriteString(rcfg.TargetLabel)
		_, _ = h.WriteString(rcfg.Replacement)
		_, _ = h.WriteString(rcfg.Action.String())
	}

	return h.Sum64()
}

//
// StatelessRelabeler
//

// StatelessRelabeler - go wrapper for C-StatelessRelabeler.
//
//	cptr - pointer to a C++ StatelessRelabeler initiated in C++ memory;
type StatelessRelabeler struct {
	rCfgs      []*RelabelConfig
	cptr       uintptr
	generation uint64
}

// NewStatelessRelabeler - init new StatelessRelabeler.
func NewStatelessRelabeler(rCfgs []*RelabelConfig) (*StatelessRelabeler, error) {
	cptr, exception := prometheusStatelessRelabelerCtor(rCfgs)
	if len(exception) != 0 {
		return nil, handleException(exception)
	}
	sr := &StatelessRelabeler{
		cptr:       cptr,
		rCfgs:      rCfgs,
		generation: ToHash(rCfgs),
	}
	runtime.SetFinalizer(sr, func(cr *StatelessRelabeler) {
		prometheusStatelessRelabelerDtor(cr.cptr)
		cr.rCfgs = nil
	})
	return sr, nil
}

// Pointer - return c-pointer.
func (sr *StatelessRelabeler) Pointer() uintptr {
	return sr.cptr
}

// Generation return StatelessRelabeler's generation hash from configs.
func (sr *StatelessRelabeler) Generation() uint64 {
	return sr.generation
}

// EqualConfigs check for complete matching of configs.
func (sr *StatelessRelabeler) EqualConfigs(relabelingCfgs []*RelabelConfig) bool {
	if len(sr.rCfgs) != len(relabelingCfgs) {
		return false
	}

	for i := range sr.rCfgs {
		if !sr.rCfgs[i].Equal(relabelingCfgs[i]) {
			return false
		}
	}

	return true
}

// ResetTo reset configs and replace on new converting go-config.
func (sr *StatelessRelabeler) ResetTo(relabelingCfgs []*RelabelConfig) error {
	sr.rCfgs = relabelingCfgs
	sr.generation = ToHash(relabelingCfgs)
	exception := prometheusStatelessRelabelerResetTo(sr.cptr, sr.rCfgs)
	return handleException(exception)
}

//
// ShardsInnerSeries
//

// NewShardsInnerSeries - init slice with the results of relabeling per shards.
func NewShardsInnerSeries(numberOfShards uint16) []*InnerSeries {
	srs := make([]*InnerSeries, numberOfShards)
	for i := range srs {
		srs[i] = NewInnerSeries()
	}

	return srs
}

// stdVector implementation cpp std::vector, for allocate 24-byte, used in cpp.
//
//nolint:unused // for cpp-bridge, used in cpp.
type stdVector struct {
	start        uintptr
	finish       uintptr
	endOfStorage uintptr
}

type bareBonesVector struct {
	memory   uintptr
	capacity uint32
	size     uint32
}

type roaringBitset [40]byte

// InnerSeries - go wrapper for C-InnerSeries.
//
//	size - number of timeseries processed;
//	data - pointer for vector with timeseries;
type InnerSeries struct {
	size uint64
	//nolint:unused // for cpp-bridge, used in cpp
	data bareBonesVector
	//nolint:unused // for cpp-bridge, used in cpp
	trackStaleNans roaringBitset
}

// Size - number of Timeseries.
func (iss *InnerSeries) Size() uint64 {
	return iss.size
}

// NewInnerSeries - init new InnerSeries with finalizer for dtor C-InnerSeries.
func NewInnerSeries() *InnerSeries {
	iss := &InnerSeries{size: 0}
	prometheusInnerSeriesCtor(iss)
	runtime.SetFinalizer(iss, func(i *InnerSeries) {
		prometheusInnerSeriesDtor(i)
	})

	return iss
}

//
// ShardsRelabeledSeries
//

// NewShardsRelabeledSeries - init slice with the relabeled results per shards.
func NewShardsRelabeledSeries(numberOfShards uint16) []*RelabeledSeries {
	rrs := make([]*RelabeledSeries, numberOfShards)
	for i := range rrs {
		rrs[i] = NewRelabeledSeries()
	}

	return rrs
}

// RelabeledSeries - go wrapper for C-RelabeledSeries.
//
//	size - number of relabeled elements processed;
//	data - pointer for vector with relabeled elements;
type RelabeledSeries struct {
	size uint64
	//nolint:unused // for cpp-bridge, used in cpp
	data stdVector
	//nolint:unused // for cpp-bridge, used in cpp
	trackStaleNans roaringBitset
}

// NewRelabeledSeries - init new RelabeledSeries with finalizer for dtor C-RelabeledSeries.
func NewRelabeledSeries() *RelabeledSeries {
	rss := &RelabeledSeries{size: 0}
	prometheusRelabeledSeriesCtor(rss)
	runtime.SetFinalizer(rss, func(r *RelabeledSeries) {
		prometheusRelabeledSeriesDtor(r)
	})

	return rss
}

// Size - number of series.
func (rss *RelabeledSeries) Size() uint64 {
	return rss.size
}

// incomingAndRelabeledLsID to update cache data.
type incomingAndRelabeledLsID struct {
	//nolint:unused // for cpp-bridge, used in cpp
	incomingLSID uint32
	//nolint:unused // for cpp-bridge, used in cpp
	relabeledLSID uint32
}

// RelabelerStateUpdate go wrapper for C-RelabelerStateUpdate.
type RelabelerStateUpdate []incomingAndRelabeledLsID

// NewRelabelerStateUpdate init new RelabelerStateUpdate.
func NewRelabelerStateUpdate() *RelabelerStateUpdate {
	rsu := new(RelabelerStateUpdate)
	prometheusRelabelerStateUpdateCtor(rsu)
	runtime.SetFinalizer(rsu, func(r *RelabelerStateUpdate) {
		prometheusRelabelerStateUpdateDtor(r)
	})

	return rsu
}

// IsEmpty returns true if the length of slice is zero.
func (rsu *RelabelerStateUpdate) IsEmpty() bool {
	return len(*rsu) == 0
}

// NewShardsRelabelerStateUpdate init slice with the results of update state per shards.
func NewShardsRelabelerStateUpdate(numberOfShards uint16) []*RelabelerStateUpdate {
	rsu := make([]*RelabelerStateUpdate, numberOfShards)
	for i := range rsu {
		rsu[i] = NewRelabelerStateUpdate()
	}

	return rsu
}

// MetricLimits limits on label set and samples.
type MetricLimits struct {
	LabelLimit            int64
	LabelNameLengthLimit  int64
	LabelValueLengthLimit int64
	SampleLimit           int64
}

// RelabelerOptions relabeling options.
type RelabelerOptions struct {
	TargetLabels             []Label
	MetricLimits             *MetricLimits
	HonorLabels              bool
	TrackTimestampsStaleness bool
	HonorTimestamps          bool
}

// StaleNansState wrap pointer to source state for stale nans .
type StaleNansState struct {
	state uintptr
}

// NewStaleNansState init new SourceStaleNansState.
func NewStaleNansState() *StaleNansState {
	s := &StaleNansState{
		state: prometheusRelabelStaleNansStateCtor(),
	}
	runtime.SetFinalizer(s, func(s *StaleNansState) {
		prometheusRelabelStaleNansStateDtor(s.state)
	})

	return s
}

// RelabelerStats statistics return from relabeler.
type RelabelerStats struct {
	SamplesAdded uint32
	SeriesAdded  uint32
	SeriesDrop   uint32
}

// Add another stats.
func (rs *RelabelerStats) Add(stats ...RelabelerStats) {
	for _, s := range stats {
		rs.SamplesAdded += s.SamplesAdded
		rs.SeriesAdded += s.SeriesAdded
		rs.SeriesDrop += s.SeriesDrop
	}
}

// String serialize to string.
func (rs RelabelerStats) String() string {
	return fmt.Sprintf(
		"{samples_added: %d, series_added: %d, series_drop: %d}",
		rs.SamplesAdded,
		rs.SeriesAdded,
		rs.SeriesDrop,
	)
}

// OutputPerShardRelabeler go wrapper for C-PerShardRelabeler, relabeler for shard.
type OutputPerShardRelabeler struct {
	statelessRelabeler           *StatelessRelabeler // pointer to go StatelessRelabeler, for keep alive
	cache                        *Cache
	externalLabels               []Label
	cptr                         uintptr // pointer to C-InputPerShardRelabeler
	generationStatelessRelabeler uint64
	generationManagerKeeper      uint32
	numberOfShards               uint16
	shardID                      uint16 // current shard id
}

// NewOutputPerShardRelabeler init new OutputPerShardRelabeler.
func NewOutputPerShardRelabeler(
	externalLabels []Label,
	statelessRelabeler *StatelessRelabeler,
	generationManagerKeeper uint32,
	numberOfShards, shardID uint16,
) (*OutputPerShardRelabeler, error) {
	p, exception := prometheusPerShardRelabelerCtor(
		externalLabels,
		statelessRelabeler.Pointer(),
		shardID,
		numberOfShards,
	)
	if len(exception) != 0 {
		return nil, handleException(exception)
	}

	opsr := &OutputPerShardRelabeler{
		statelessRelabeler:           statelessRelabeler,
		cache:                        NewCache(),
		externalLabels:               externalLabels,
		cptr:                         p,
		generationStatelessRelabeler: statelessRelabeler.Generation(),
		generationManagerKeeper:      generationManagerKeeper,
		numberOfShards:               numberOfShards,
		shardID:                      shardID,
	}
	runtime.SetFinalizer(opsr, func(psr *OutputPerShardRelabeler) {
		prometheusPerShardRelabelerDtor(psr.cptr)
		psr.statelessRelabeler = nil
	})
	return opsr, nil
}

// OutputRelabeling relabeling output series(fourth stage).
func (opsr *OutputPerShardRelabeler) OutputRelabeling(
	ctx context.Context,
	lss *LabelSetStorage,
	incomingInnerSeries []*InnerSeries,
	encodersInnerSeries []*InnerSeries,
	relabeledSeries *RelabeledSeries,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exception := prometheusPerShardRelabelerOutputRelabeling(
		opsr.cptr,
		lss.Pointer(),
		opsr.cache.cPointer,
		incomingInnerSeries,
		encodersInnerSeries,
		relabeledSeries,
	)

	return handleException(exception)
}

// ResetCache reset cache if need.
func (opsr *OutputPerShardRelabeler) ResetCache(generationManagerKeeper uint32, numberOfShards uint16) {
	if opsr.generationStatelessRelabeler == opsr.statelessRelabeler.Generation() &&
		opsr.generationManagerKeeper == generationManagerKeeper &&
		opsr.numberOfShards == numberOfShards {
		return
	}
	opsr.cache = NewCache()
}

// ResetTo reset set new number_of_shards and external_labels.
func (opsr *OutputPerShardRelabeler) ResetTo(
	externalLabels []Label,
	generationManagerKeeper uint32,
	numberOfShards uint16,
) {
	opsr.ResetCache(generationManagerKeeper, numberOfShards)
	opsr.numberOfShards = numberOfShards
	opsr.generationManagerKeeper = generationManagerKeeper
	opsr.externalLabels = externalLabels
	prometheusPerShardRelabelerResetTo(opsr.externalLabels, opsr.cptr, opsr.numberOfShards)
}

// StatelessRelabeler return current *StatelessRelabeler.
func (opsr *OutputPerShardRelabeler) StatelessRelabeler() *StatelessRelabeler {
	return opsr.statelessRelabeler
}

// UpdateRelabelerState add to cache relabled data(fifth stage).
func (opsr *OutputPerShardRelabeler) UpdateRelabelerState(
	ctx context.Context,
	relabelerStateUpdate *RelabelerStateUpdate,
	relabeledShardID uint16,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exception := prometheusPerShardSingleRelabelerUpdateRelabelerState(
		relabelerStateUpdate,
		opsr.cptr,
		opsr.cache.cPointer,
		relabeledShardID,
	)

	return handleException(exception)
}

//
// Cache
//

// Cache - go wrapper for C-Cache, cache for relabeler.
//
//	cPointer   - pointer to C-Cache;
type Cache struct {
	cPointer uintptr
	lock     sync.RWMutex
}

// NewCache init new Cache.
func NewCache() *Cache {
	cache := &Cache{
		cPointer: prometheusCacheCtor(),
	}
	runtime.SetFinalizer(cache, func(c *Cache) {
		prometheusCacheDtor(c.cPointer)
	})
	return cache
}

// AllocatedMemory return size of allocated memory for caches.
func (c *Cache) AllocatedMemory() uint64 {
	c.lock.RLock()
	res := prometheusCacheAllocatedMemory(c.cPointer)
	c.lock.RUnlock()
	runtime.KeepAlive(c)
	return res
}

// Update add to cache relabled data(third stage).
func (c *Cache) Update(ctx context.Context, shardsRelabelerStateUpdate []*RelabelerStateUpdate) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	c.lock.Lock()
	exception := prometheusCacheUpdate(shardsRelabelerStateUpdate, c.cPointer)
	c.lock.Unlock()
	runtime.KeepAlive(c)

	return handleException(exception)
}

//
// PerGoroutineRelabeler
//

// PerGoroutineRelabeler go wrapper for C-PerGoroutineRelabeler, relabeler for shard goroutines.
type PerGoroutineRelabeler struct {
	cptr              uintptr
	gcDestroyDetector *uint64
	shardID           uint16
}

// NewPerGoroutineRelabeler init new [PerGoroutineRelabeler].
func NewPerGoroutineRelabeler(
	numberOfShards, shardID uint16,
) *PerGoroutineRelabeler {
	pgr := &PerGoroutineRelabeler{
		cptr:              prometheusPerGoroutineRelabelerCtor(numberOfShards, shardID),
		gcDestroyDetector: &gcDestroyDetector,
		shardID:           shardID,
	}
	runtime.SetFinalizer(pgr, func(r *PerGoroutineRelabeler) {
		prometheusPerGoroutineRelabelerDtor(r.cptr)
	})

	return pgr
}

// AppendRelabelerSeries add relabeled ls to lss, add to result and add to cache update(second stage).
func (pgr *PerGoroutineRelabeler) AppendRelabelerSeries(
	ctx context.Context,
	targetLss *LabelSetStorage,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
	shardsRelabelerStateUpdate []*RelabelerStateUpdate,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	exception, hasReallocations := prometheusPerGoroutineRelabelerAppendRelabelerSeries(
		pgr.cptr,
		targetLss.Pointer(),
		shardsInnerSeries,
		shardsRelabeledSeries,
		shardsRelabelerStateUpdate,
	)

	return hasReallocations, handleException(exception)
}

// Relabeling relabeling incoming hashdex(first stage).
func (pgr *PerGoroutineRelabeler) Relabeling(
	ctx context.Context,
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	state *StateV2,
	shardedData ShardedData,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) (RelabelerStats, bool, error) {
	if ctx.Err() != nil {
		return RelabelerStats{}, false, ctx.Err()
	}

	cptrContainer, ok := shardedData.(cptrable)
	if !ok {
		return RelabelerStats{}, false, ErrMustImplementCptrable
	}

	if state.TrackStaleness() {
		return pgr.inputRelabelingWithStalenans(
			inputLss,
			targetLss,
			state,
			cptrContainer,
			shardsInnerSeries,
			shardsRelabeledSeries,
		)
	}

	if state.IsTransition() {
		return pgr.inputTransitionRelabeling(
			targetLss,
			state,
			cptrContainer,
			shardsInnerSeries,
		)
	}

	return pgr.inputRelabeling(
		inputLss,
		targetLss,
		state,
		cptrContainer,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
}

// RelabelingFromCache relabeling incoming hashdex(first stage) from cache.
func (pgr *PerGoroutineRelabeler) RelabelingFromCache(
	ctx context.Context,
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	state *StateV2,
	shardedData ShardedData,
	shardsInnerSeries []*InnerSeries,
) (RelabelerStats, bool, error) {
	if ctx.Err() != nil {
		return RelabelerStats{}, false, ctx.Err()
	}

	cptrContainer, ok := shardedData.(cptrable)
	if !ok {
		return RelabelerStats{}, false, ErrMustImplementCptrable
	}

	if state.TrackStaleness() {
		return pgr.inputRelabelingWithStalenansFromCache(
			inputLss,
			targetLss,
			state,
			cptrContainer,
			shardsInnerSeries,
		)
	}

	if state.IsTransition() {
		return pgr.inputTransitionRelabelingOnlyRead(
			targetLss,
			state,
			cptrContainer,
			shardsInnerSeries,
		)
	}

	return pgr.inputRelabelingFromCache(
		inputLss,
		targetLss,
		state,
		cptrContainer,
		shardsInnerSeries,
	)
}

// inputRelabeling relabeling incoming hashdex(first stage).
func (pgr *PerGoroutineRelabeler) inputRelabeling(
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	state *StateV2,
	cptrContainer cptrable,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) (RelabelerStats, bool, error) {
	cache := state.CacheByShard(pgr.shardID)
	cache.lock.Lock()
	stats, exception, hasReallocations := prometheusPerGoroutineRelabelerInputRelabeling(
		pgr.cptr,
		state.StatelessRelabeler().Pointer(),
		inputLss.Pointer(),
		targetLss.Pointer(),
		cache.cPointer,
		cptrContainer.cptr(),
		state.RelabelerOptions(),
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	cache.lock.Unlock()

	runtime.KeepAlive(pgr)
	runtime.KeepAlive(inputLss)
	runtime.KeepAlive(targetLss)
	runtime.KeepAlive(state)
	runtime.KeepAlive(cptrContainer)

	return stats, hasReallocations, handleException(exception)
}

// InputRelabelingFromCache relabeling incoming hashdex(first stage) from cache.
func (pgr *PerGoroutineRelabeler) inputRelabelingFromCache(
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	state *StateV2,
	cptrContainer cptrable,
	shardsInnerSeries []*InnerSeries,
) (RelabelerStats, bool, error) {
	cache := state.CacheByShard(pgr.shardID)
	cache.lock.RLock()
	stats, exception, ok := prometheusPerGoroutineRelabelerInputRelabelingFromCache(
		pgr.cptr,
		inputLss.Pointer(),
		targetLss.Pointer(),
		cache.cPointer,
		cptrContainer.cptr(),
		state.RelabelerOptions(),
		shardsInnerSeries,
	)
	cache.lock.RUnlock()

	runtime.KeepAlive(pgr)
	runtime.KeepAlive(inputLss)
	runtime.KeepAlive(targetLss)
	runtime.KeepAlive(state)
	runtime.KeepAlive(cptrContainer)

	return stats, ok, handleException(exception)
}

// inputRelabelingWithStalenans relabeling incoming hashdex(first stage) with state stalenans.
func (pgr *PerGoroutineRelabeler) inputRelabelingWithStalenans(
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	state *StateV2,
	cptrContainer cptrable,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) (RelabelerStats, bool, error) {
	cache := state.CacheByShard(pgr.shardID)
	cache.lock.Lock()
	stats, exception, hasReallocations := prometheusPerGoroutineRelabelerInputRelabelingWithStalenans(
		pgr.cptr,
		state.StatelessRelabeler().Pointer(),
		inputLss.Pointer(),
		targetLss.Pointer(),
		cache.cPointer,
		cptrContainer.cptr(),
		state.DefTimestamp(),
		state.RelabelerOptions(),
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	cache.lock.Unlock()

	runtime.KeepAlive(pgr)
	runtime.KeepAlive(inputLss)
	runtime.KeepAlive(targetLss)
	runtime.KeepAlive(state)
	runtime.KeepAlive(cptrContainer)

	return stats, hasReallocations, handleException(exception)
}

// inputRelabelingWithStalenansFromCache relabeling incoming hashdex(first stage) from cache with state stalenans.
func (pgr *PerGoroutineRelabeler) inputRelabelingWithStalenansFromCache(
	inputLss *LabelSetStorage,
	targetLss *LabelSetStorage,
	state *StateV2,
	cptrContainer cptrable,
	shardsInnerSeries []*InnerSeries,
) (RelabelerStats, bool, error) {
	cache := state.CacheByShard(pgr.shardID)
	cache.lock.RLock()
	stats, exception, ok := prometheusPerGoroutineRelabelerInputRelabelingWithStalenansFromCache(
		pgr.cptr,
		inputLss.Pointer(),
		targetLss.Pointer(),
		cache.cPointer,
		cptrContainer.cptr(),
		state.DefTimestamp(),
		state.RelabelerOptions(),
		shardsInnerSeries,
	)
	cache.lock.RUnlock()

	runtime.KeepAlive(pgr)
	runtime.KeepAlive(inputLss)
	runtime.KeepAlive(targetLss)
	runtime.KeepAlive(state)
	runtime.KeepAlive(cptrContainer)

	return stats, ok, handleException(exception)
}

// inputTransitionRelabeling transparent relabeling incoming hashdex(first stage).
func (pgr *PerGoroutineRelabeler) inputTransitionRelabeling(
	targetLss *LabelSetStorage,
	state *StateV2,
	cptrContainer cptrable,
	shardsInnerSeries []*InnerSeries,
) (RelabelerStats, bool, error) {
	stats, exception, hasReallocations := prometheusPerGoroutineRelabelerInputTransitionRelabeling(
		pgr.cptr,
		targetLss.Pointer(),
		cptrContainer.cptr(),
		shardsInnerSeries,
	)

	runtime.KeepAlive(pgr)
	runtime.KeepAlive(targetLss)
	runtime.KeepAlive(state)
	runtime.KeepAlive(cptrContainer)

	return stats, hasReallocations, handleException(exception)
}

// inputTransitionRelabelingOnlyRead transparent relabeling incoming hashdex(first stage) from cache.
func (pgr *PerGoroutineRelabeler) inputTransitionRelabelingOnlyRead(
	targetLss *LabelSetStorage,
	state *StateV2,
	cptrContainer cptrable,
	shardsInnerSeries []*InnerSeries,
) (RelabelerStats, bool, error) {
	stats, exception, ok := prometheusPerGoroutineRelabelerInputRelabelingOnlyRead(
		pgr.cptr,
		targetLss.Pointer(),
		cptrContainer.cptr(),
		shardsInnerSeries,
	)

	runtime.KeepAlive(pgr)
	runtime.KeepAlive(targetLss)
	runtime.KeepAlive(state)
	runtime.KeepAlive(cptrContainer)

	return stats, ok, handleException(exception)
}

// PerGoroutineRelabelerTrackStaleNans add stale nans samples if needed
func PerGoroutineRelabelerTrackStaleNans(
	innerSeries []*InnerSeries,
	state *StateV2,
	shardID uint16,
) {
	if !state.TrackStaleness() {
		return
	}

	prometheusPerGoroutineRelabelerTrackStaleNans(
		innerSeries,
		state.StaleNansStateByShard(shardID).state,
		state.DefTimestamp(),
	)
	runtime.KeepAlive(innerSeries)
	runtime.KeepAlive(state)
}

//
// TransitionLocker
//

// TransitionLocker is an implementing [sync.Mutex] that, depending on the situation, does not block.
type TransitionLocker struct {
	mx   sync.Mutex
	lock bool
}

// NewTransitionLocker init new [TransitionLocker].
func NewTransitionLocker() TransitionLocker {
	return TransitionLocker{
		mx:   sync.Mutex{},
		lock: true,
	}
}

// NewTransitionLockerWithoutLock init new [TransitionLocker], without locks.
func NewTransitionLockerWithoutLock() TransitionLocker {
	return TransitionLocker{
		mx:   sync.Mutex{},
		lock: false,
	}
}

// Lock locks rw for writing, if need.
func (l *TransitionLocker) Lock() {
	if l.lock {
		l.mx.Lock()
	}
}

// Unlock unlocks rw for writing, if need.
func (l *TransitionLocker) Unlock() {
	if l.lock {
		l.mx.Unlock()
	}
}

//
// StateV2
//

const (
	initStatus       uint8 = 0
	inited           uint8 = 15
	transitionStatus uint8 = 240
)

// StateV2 of relabelers per shard.
type StateV2 struct {
	caches             []*Cache
	staleNansStates    []*StaleNansState
	statelessRelabeler *StatelessRelabeler
	locker             TransitionLocker
	defTimestamp       int64
	generationHead     uint64
	options            RelabelerOptions
	status             uint8
	trackStaleness     bool
}

// NewTransitionStateV2 init empty [StateV2], with locks.
func NewTransitionStateV2() *StateV2 {
	return &StateV2{
		locker:         NewTransitionLocker(),
		generationHead: math.MaxUint64,
		status:         transitionStatus,
		trackStaleness: false,
	}
}

// NewTransitionStateV2WithoutLock init empty [StateV2], without locks.
func NewTransitionStateV2WithoutLock() *StateV2 {
	return &StateV2{
		locker:         NewTransitionLockerWithoutLock(),
		generationHead: math.MaxUint64,
		status:         transitionStatus,
		trackStaleness: false,
	}
}

// NewStateV2 init empty [StateV2], with locks.
func NewStateV2() *StateV2 {
	return &StateV2{
		locker:         NewTransitionLocker(),
		generationHead: math.MaxUint64,
		status:         initStatus,
		trackStaleness: false,
	}
}

// NewStateV2WithoutLock init empty [StateV2], without locks.
func NewStateV2WithoutLock() *StateV2 {
	return &StateV2{
		locker:         NewTransitionLockerWithoutLock(),
		generationHead: math.MaxUint64,
		status:         initStatus,
		trackStaleness: false,
	}
}

// CacheByShard return *Cache for shard.
func (s *StateV2) CacheByShard(shardID uint16) *Cache {
	if s.IsTransition() {
		panic("CacheByShard: state is transition")
	}

	return s.caches[shardID]
}

// DefTimestamp return timestamp for scrape time and stalenan.
func (s *StateV2) DefTimestamp() int64 {
	if s.defTimestamp == 0 {
		return time.Now().UnixMilli()
	}

	return s.defTimestamp
}

// EnableTrackStaleness enable track stalenans.
func (s *StateV2) EnableTrackStaleness() {
	if s.IsTransition() {
		panic("EnableTrackStaleness: state is transition")
	}

	s.trackStaleness = true
}

// Reconfigure recreate caches and stalenans states if need and set new generations.
func (s *StateV2) Reconfigure(
	generationHead uint64,
	numberOfShards uint16,
) {
	if s.status&inited == inited && generationHead == s.generationHead {
		return
	}

	// long way
	s.locker.Lock()

	// we check it a second time, but under lock
	if s.status&inited == inited && generationHead == s.generationHead {
		s.locker.Unlock()
		return
	}

	// the transition state does not require caches and staleNaNs
	if s.IsTransition() {
		s.status |= inited
		s.generationHead = generationHead
		s.locker.Unlock()
		return
	}

	s.resetCaches(numberOfShards)
	s.resetStaleNansStates(numberOfShards)
	s.status |= inited
	s.generationHead = generationHead

	s.locker.Unlock()
}

// IsTransition indicates whether the state is transition.
func (s *StateV2) IsTransition() bool {
	return s.status&transitionStatus == transitionStatus
}

// RelabelerOptions return Options for relabeler.
func (s *StateV2) RelabelerOptions() RelabelerOptions {
	return s.options
}

// SetDefTimestamp set timestamp for scrape time and stalenan.
func (s *StateV2) SetDefTimestamp(ts int64) {
	s.defTimestamp = ts
}

// SetRelabelerOptions set Options for relabeler.
func (s *StateV2) SetRelabelerOptions(options *RelabelerOptions) {
	s.options = *options
}

// SetStatelessRelabeler sets [StatelessRelabeler] for [PerGoroutineRelabeler].
func (s *StateV2) SetStatelessRelabeler(statelessRelabeler *StatelessRelabeler) {
	if s.IsTransition() {
		panic("SetStatelessRelabeler: state is transition")
	}

	s.statelessRelabeler = statelessRelabeler
}

// StaleNansStateByShard return SourceStaleNansState for shard.
func (s *StateV2) StaleNansStateByShard(shardID uint16) *StaleNansState {
	if s.IsTransition() {
		panic("StaleNansStateByShard: state is transition")
	}

	return s.staleNansStates[shardID]
}

// StatelessRelabeler returns [StatelessRelabeler] for [PerGoroutineRelabeler].
func (s *StateV2) StatelessRelabeler() *StatelessRelabeler {
	if s.IsTransition() {
		panic("StatelessRelabeler: state is transition")
	}

	if s.statelessRelabeler == nil {
		panic("statelessRelabeler is nil")
	}

	return s.statelessRelabeler
}

// TrackStaleness return state track stalenans.
func (s *StateV2) TrackStaleness() bool {
	return s.trackStaleness
}

// resetCaches recreate Caches.
func (s *StateV2) resetCaches(numberOfShards uint16) {
	switch {
	case len(s.caches) > int(numberOfShards):
		for shardID := range s.caches[numberOfShards:] {
			s.caches[shardID] = nil
		}

		// cut
		s.caches = s.caches[:numberOfShards]
	case len(s.caches) < int(numberOfShards):
		// grow
		s.caches = make([]*Cache, numberOfShards)
	}

	for shardID := range s.caches {
		s.caches[shardID] = NewCache()
	}
}

// resetStaleNansStates recreate StaleNansStates.
func (s *StateV2) resetStaleNansStates(numberOfShards uint16) {
	if !s.trackStaleness {
		return
	}

	switch {
	case len(s.staleNansStates) > int(numberOfShards):
		for shardID := range s.staleNansStates[numberOfShards:] {
			s.staleNansStates[shardID] = nil
		}

		// cut
		s.staleNansStates = s.staleNansStates[:numberOfShards]
	case len(s.staleNansStates) < int(numberOfShards):
		// grow
		s.staleNansStates = make([]*StaleNansState, numberOfShards)
	}

	for shardID := range s.staleNansStates {
		s.staleNansStates[shardID] = NewStaleNansState()
	}
}

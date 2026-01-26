package cppbridge

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L.
// #cgo asan LDFLAGS: -fsanitize=address
// #cgo asan CFLAGS: -fsanitize=address
// #cgo arm64,!asan,!dbg LDFLAGS: -l:arm64_entrypoint_init_aio_opt.a -l:arm64_armv8_a_entrypoint_aio_prefixed_opt.a -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_opt.a
// #cgo arm64,!asan,dbg LDFLAGS: -l:arm64_entrypoint_init_aio_dbg.a -l:arm64_armv8_a_entrypoint_aio_prefixed_dbg.a -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_dbg.a
// #cgo arm64,asan,!dbg LDFLAGS: -l:arm64_entrypoint_init_aio_opt_asan.a -l:arm64_armv8_a_entrypoint_aio_prefixed_opt_asan.a -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_opt_asan.a
// #cgo arm64,asan,dbg LDFLAGS: -l:arm64_entrypoint_init_aio_dbg_asan.a -l:arm64_armv8_a_entrypoint_aio_prefixed_dbg_asan.a -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_dbg_asan.a
// #cgo amd64,!asan,!dbg LDFLAGS: -l:amd64_entrypoint_init_aio_opt.a -l:amd64_k8_entrypoint_aio_prefixed_opt.a -l:amd64_nehalem_entrypoint_aio_prefixed_opt.a -l:amd64_haswell_entrypoint_aio_prefixed_opt.a
// #cgo amd64,!asan,dbg LDFLAGS: -l:amd64_entrypoint_init_aio_dbg.a -l:amd64_k8_entrypoint_aio_prefixed_dbg.a -l:amd64_nehalem_entrypoint_aio_prefixed_dbg.a -l:amd64_haswell_entrypoint_aio_prefixed_dbg.a
// #cgo amd64,asan,!dbg LDFLAGS: -l:amd64_entrypoint_init_aio_opt_asan.a -l:amd64_k8_entrypoint_aio_prefixed_opt_asan.a -l:amd64_nehalem_entrypoint_aio_prefixed_opt_asan.a -l:amd64_haswell_entrypoint_aio_prefixed_opt_asan.a
// #cgo amd64,asan,dbg LDFLAGS: -l:amd64_entrypoint_init_aio_dbg_asan.a -l:amd64_k8_entrypoint_aio_prefixed_dbg_asan.a -l:amd64_nehalem_entrypoint_aio_prefixed_dbg_asan.a -l:amd64_haswell_entrypoint_aio_prefixed_dbg_asan.a
// #cgo !static LDFLAGS: -lstdc++ -lm -lgcc_eh -l:libunwind.a -llzma -lstdc++_libbacktrace
// #cgo static LDFLAGS: -static -static-libgcc -static-libstdc++ -l:libstdc++.a -l:libm.a -l:libgcc_eh.a -l:libunwind.a -l:liblzma.a -l:libstdc++_libbacktrace.a
// #include "entrypoint.h"
import "C" //nolint:gocritic // because otherwise it won't work
import (
	"runtime"
	"time"
	"unsafe" //nolint:gocritic // because otherwise it won't work

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge/fastcgo"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/util"
)

type (
	CppStdVector              = [C.Sizeof_StdVector]byte
	CppBareBonesVector        = [C.Sizeof_BareBonesVector]byte
	CppRoaringBitset          = [C.Sizeof_RoaringBitset]byte
	CppSerializedDataIterator = [C.Sizeof_SerializedDataIterator]byte
	CppMetricsIterator        = [C.Sizeof_MetricsIterator]byte
)

const (
	GoLabelsSize = C.Sizeof_GoLabels
)

var (

	// per_goroutine_relabeler input_relabeling
	perGoroutineRelabelerInputRelabelingSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "per_goroutine_relabeler", "method": "input_relabeling"},
		},
	)
	perGoroutineRelabelerInputRelabelingCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "per_goroutine_relabeler", "method": "input_relabeling"},
		},
	)

	// per_goroutine_relabeler input_relabeling_from_cache
	perGoroutineRelabelerInputRelabelingFromCacheSum = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "input_relabeling_from_cache",
			},
		},
	)
	perGoroutineRelabelerInputRelabelingFromCacheCount = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "input_relabeling_from_cache",
			},
		},
	)

	// per_goroutine_relabeler relabeling_with_stalenans
	perGoroutineRelabelerInputRelabelingWithStalenansSum = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "relabeling_with_stalenans",
			},
		},
	)
	perGoroutineRelabelerInputRelabelingWithStalenansCount = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "relabeling_with_stalenans",
			},
		},
	)

	// per_goroutine_relabeler relabeling_with_stalenans_from_cache
	perGoroutineRelabelerInputRelabelingWithStalenansFromCacheSum = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "relabeling_with_stalenans_from_cache",
			},
		},
	)
	perGoroutineRelabelerInputRelabelingWithStalenansFromCacheCount = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "relabeling_with_stalenans_from_cache",
			},
		},
	)

	// per_goroutine_relabeler input_transition_relabeling
	perGoroutineRelabelerInputTransitionRelabelingSum = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "input_transition_relabeling",
			},
		},
	)
	perGoroutineRelabelerInputTransitionRelabelingCount = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "input_transition_relabeling",
			},
		},
	)

	// per_goroutine_relabeler input_transition_relabeling_only_read
	perGoroutineRelabelerInputTransitionRelabelingOnlyReadSum = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "input_transition_relabeling_only_read",
			},
		},
	)
	perGoroutineRelabelerInputTransitionRelabelingOnlyReadCount = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help: "The time duration cpp call.",
			ConstLabels: prometheus.Labels{
				"object": "per_goroutine_relabeler",
				"method": "input_transition_relabeling_only_read",
			},
		},
	)

	// per_goroutine_relabeler append_relabeler_series
	perGoroutineRelabelerAppendRelabelerSeriesSum = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "per_goroutine_relabeler", "method": "append_relabeler_series"},
		},
	)
	perGoroutineRelabelerAppendRelabelerSeriesCount = util.NewUnconflictRegisterer(
		prometheus.DefaultRegisterer,
	).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "per_goroutine_relabeler", "method": "append_relabeler_series"},
		},
	)

	// input_relabeler update_relabeler_state
	inputRelabelerUpdateRelabelerStateSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "input_relabeler", "method": "update_relabeler_state"},
		},
	)
	inputRelabelerUpdateRelabelerStateCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "input_relabeler", "method": "update_relabeler_state"},
		},
	)

	// head_data_storage allocated_memory
	headDataStorageAllocatedMemorySum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "allocated_memory"},
		},
	)
	headDataStorageAllocatedMemoryCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "allocated_memory"},
		},
	)

	// head_data_storage query
	headDataStorageQuerySum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "query"},
		},
	)
	headDataStorageQueryCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "query"},
		},
	)

	// head_data_storage query final
	headDataStorageQueryFinalSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "query_final"},
		},
	)
	headDataStorageQueryFinalCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "query_final"},
		},
	)

	// head_data_storage encode_inner_series_slice
	headDataStorageEncodeInnerSeriesSliceSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "encode_inner_series_slice"},
		},
	)
	headDataStorageEncodeInnerSeriesSliceCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "encode_inner_series_slice"},
		},
	)

	// head_data_storage merge_out_of_order_chunks
	headDataStorageMergeOutOfOrderChunksSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "merge_out_of_order_chunks"},
		},
	)
	headDataStorageMergeOutOfOrderChunksCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_data_storage", "method": "merge_out_of_order_chunks"},
		},
	)

	// prometheus_hashdex parse
	prometheusHashdexParseSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "prometheus_hashdex", "method": "parse"},
		},
	)
	prometheusHashdexParseCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "prometheus_hashdex", "method": "parse"},
		},
	)

	// open_metrics_hashdex parse
	openMetricsHashdexParseSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "open_metrics_hashdex", "method": "parse"},
		},
	)
	openMetricsHashdexParseCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "open_metrics_hashdex", "method": "parse"},
		},
	)

	// head_wal_encoder add_inner_series
	headWalEncoderAddInnerSeriesSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_wal_encoder", "method": "add_inner_series"},
		},
	)
	headWalEncoderAddInnerSeriesCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_wal_encoder", "method": "add_inner_series"},
		},
	)

	// head_wal_encoder finalize
	headWalEncoderFinalizeSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_wal_encoder", "method": "finalize"},
		},
	)
	headWalEncoderFinalizeCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "head_wal_encoder", "method": "finalize"},
		},
	)

	// chunk_recoder recode_next_chunk
	chunkRecoderRecodeNextChunkSum = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_sum",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "chunk_recoder", "method": "recode_next_chunk"},
		},
	)
	chunkRecoderRecodeNextChunkCount = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_unsafecall_nanoseconds_count",
			Help:        "The time duration cpp call.",
			ConstLabels: prometheus.Labels{"object": "chunk_recoder", "method": "recode_next_chunk"},
		},
	)
)

func freeBytes(b []byte) {
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_free_bytes,
		uintptr(unsafe.Pointer(&b)),
	)
	runtime.KeepAlive(b)
}

// GetFlavor returns recognized architecture flavor
//
//revive:disable:confusing-naming // wrapper
func getFlavor() string {
	var res struct {
		flavor string
	}
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_get_flavor,
		uintptr(unsafe.Pointer(&res)),
	)
	return res.flavor
}

func memInfo() (res MemInfo) {
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_mem_info,
		uintptr(unsafe.Pointer(&res)),
	)
	return res
}

func dumpMemoryProfile(filename string) int {
	args := struct {
		filename string
	}{filename}

	res := struct {
		error int
	}{0}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_dump_memory_profile,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	return res.error
}

//
// ProtobufHashdex
//

func walProtobufHashdexCtor(limits WALHashdexLimits) uintptr {
	var res struct {
		hashdex uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_hashdex_ctor,
		uintptr(unsafe.Pointer(&limits)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walHashdexDtor(hashdex uintptr) {
	args := struct {
		hashdex uintptr
	}{hashdex}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_hashdex_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func walProtobufHashdexSnappyPresharding(
	hashdex uintptr,
	compressedProtobuf []byte,
) (cluster, replica string, err []byte) {
	args := struct {
		hashdex            uintptr
		compressedProtobuf []byte
	}{hashdex, compressedProtobuf}
	var res struct {
		cluster   string
		replica   string
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_hashdex_snappy_presharding,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cluster, res.replica, res.exception
}

func walProtobufHashdexGetMetadata(hashdex uintptr) []WALScraperHashdexMetadata {
	args := struct {
		hashdex uintptr
	}{hashdex}
	var res struct {
		metadata []WALScraperHashdexMetadata
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_hashdex_get_metadata,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.metadata
}

//
// GoModelHashdex
//

func walGoModelHashdexCtor(limits WALHashdexLimits) uintptr {
	var res struct {
		hashdex uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_go_model_hashdex_ctor,
		uintptr(unsafe.Pointer(&limits)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walGoModelHashdexPresharding(hashdex uintptr, data []model.TimeSeries) (cluster, replica string, err []byte) {
	args := struct {
		hashdex uintptr
		data    []model.TimeSeries
	}{hashdex, data}
	var res struct {
		cluster   string
		replica   string
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_go_model_hashdex_presharding,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cluster, res.replica, res.exception
}

//
// Encoder
//

// walEncodersVersion - return current encoders version.
func walEncodersVersion() uint8 {
	var res struct {
		encoders_version uint8
	}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_encoders_version,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoders_version
}

// walEncoderCtor - wrapper for constructor C-Encoder.
func walEncoderCtor(shardID uint16, logShards uint8) uintptr {
	args := struct {
		shardID   uint16
		logShards uint8
	}{shardID, logShards}
	var res struct {
		encoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

// walEncoderAdd - add to encode incoming data(ShardedData) through C++ encoder.
func walEncoderAdd(encoder, hashdex uintptr) (stats WALEncoderStats, exception []byte) {
	args := struct {
		encoder uintptr
		hashdex uintptr
	}{encoder, hashdex}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

func walEncoderAddInnerSeries(encoder uintptr, innerSeries []InnerSeries) (stats WALEncoderStats, exception []byte) {
	args := struct {
		innerSeries []InnerSeries
		encoder     uintptr
	}{innerSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add_inner_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

func walEncoderAddRelabeledSeries(
	encoder uintptr,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) (stats WALEncoderStats, exception []byte) {
	args := struct {
		relabelerStateUpdate *RelabelerStateUpdate
		relabeledSeries      *RelabeledSeries
		encoder              uintptr
	}{relabelerStateUpdate, relabeledSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add_relabeled_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func walEncoderFinalize(encoder uintptr) (stats WALEncoderStats, segment, exception []byte) {
	args := struct {
		encoder uintptr
	}{encoder}
	var res struct {
		WALEncoderStats
		segment   []byte
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_finalize,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.segment, res.exception
}

// walEncoderAddWithStaleNans - add to encode incoming data(ShardedData)
// to current segment and mark as stale obsolete series through C++ encoder.
func walEncoderAddWithStaleNans(
	encoder, hashdex, sourceState uintptr,
	staleTS int64,
) (stats WALEncoderStats, state uintptr, exception []byte) {
	args := struct {
		encoder     uintptr
		hashdex     uintptr
		staleTS     int64
		sourceState uintptr
	}{encoder, hashdex, staleTS, sourceState}
	var res struct {
		WALEncoderStats
		sourceState uintptr
		exception   []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add_with_stale_nans,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.sourceState, res.exception
}

// walEncoderCollectSource - destroy source state and mark all series as stale.
func walEncoderCollectSource(encoder, sourceState uintptr, staleTS int64) (stats WALEncoderStats, exception []byte) {
	args := struct {
		encoder     uintptr
		staleTS     int64
		sourceState uintptr
	}{encoder, staleTS, sourceState}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_collect_source,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderDtor - wrapper for destructor C-Encoder.
func walEncoderDtor(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// EncoderLightweight
//

// walEncoderLightweightCtor - wrapper for constructor C-EncoderLightweight.
func walEncoderLightweightCtor(shardID uint16, logShards uint8) uintptr {
	args := struct {
		shardID   uint16
		logShards uint8
	}{shardID, logShards}
	var res struct {
		encoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

// walEncoderLightweightAdd - add to encode incoming data(ShardedData) through C++ EncoderLightweight.
func walEncoderLightweightAdd(encoder, hashdex uintptr) (stats WALEncoderStats, exception []byte) {
	args := struct {
		encoder uintptr
		hashdex uintptr
	}{encoder, hashdex}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_add,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderLightweightAddInnerSeries - add inner series to current segment.
func walEncoderLightweightAddInnerSeries(
	encoder uintptr,
	innerSeries []InnerSeries,
) (stats WALEncoderStats, exception []byte) {
	args := struct {
		innerSeries []InnerSeries
		encoder     uintptr
	}{innerSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_add_inner_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderLightweightAddRelabeledSeries - add relabeled series to current segment.
func walEncoderLightweightAddRelabeledSeries(
	encoder uintptr,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) (stats WALEncoderStats, exception []byte) {
	args := struct {
		relabelerStateUpdate *RelabelerStateUpdate
		relabeledSeries      *RelabeledSeries
		encoder              uintptr
	}{relabelerStateUpdate, relabeledSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_add_relabeled_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderLightweightFinalize - finalize the encoded data in the C++ EncoderLightweight to Segment.
func walEncoderLightweightFinalize(encoder uintptr) (stats WALEncoderStats, segment, exception []byte) {
	args := struct {
		encoder uintptr
	}{encoder}
	var res struct {
		WALEncoderStats
		segment   []byte
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_finalize,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.segment, res.exception
}

// walEncoderLightweightDtor - wrapper for destructor C-EncoderLightweight.
func walEncoderLightweightDtor(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_encoder_lightweight_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// Decoder
//

// walDecoderCtor - wrapper for constructor C-Decoder.
func walDecoderCtor(encodersVersion uint8) uintptr {
	args := struct {
		encoder_version uint8
	}{encodersVersion}
	var res struct {
		decoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

// walDecoderDecode - decode WAL-segment into protobuf message through C++ decoder.
func walDecoderDecode(decoder uintptr, segment []byte) (stats DecodedSegmentStats, protobuf, err []byte) {
	args := struct {
		decoder uintptr
		segment []byte
	}{decoder, segment}
	var res struct {
		DecodedSegmentStats
		protobuf []byte
		error    []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.DecodedSegmentStats, res.protobuf, res.error
}

// walDecoderDecodeToHashdex decode WAL-segment into BasicDecoderHashdex through C++ decoder.
func walDecoderDecodeToHashdex(
	decoder uintptr,
	segment []byte,
) (
	stats DecodedSegmentStats,
	hashdex uintptr,
	cluster, replica string,
	err []byte,
) {
	args := struct {
		decoder uintptr
		segment []byte
	}{decoder, segment}
	var res struct {
		DecodedSegmentStats
		hashdex uintptr
		cluster string
		replica string
		error   []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode_to_hashdex,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.DecodedSegmentStats, res.hashdex, res.cluster, res.replica, res.error
}

// walDecoderDecodeToHashdexWithMetricInjection decode WAL-segment into BasicDecoderHashdex through C++ decoder
// with metadata for injection metrics.
func walDecoderDecodeToHashdexWithMetricInjection(
	decoder uintptr,
	meta *MetaInjection,
	segment []byte,
) (
	stats DecodedSegmentStats,
	hashdex uintptr,
	cluster, replica string,
	err []byte,
) {
	args := struct {
		decoder uintptr
		meta    *MetaInjection
		segment []byte
	}{decoder, meta, segment}
	var res struct {
		DecodedSegmentStats
		hashdex uintptr
		cluster string
		replica string
		error   []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode_to_hashdex_with_metric_injection,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.DecodedSegmentStats, res.hashdex, res.cluster, res.replica, res.error
}

// decoderDecode - decode WAL-segment and drop decoded data through C++ decoder.
func walDecoderDecodeDry(decoder uintptr, segment []byte) (segmentID uint32, err []byte) {
	args := struct {
		decoder uintptr
		segment []byte
	}{decoder, segment}
	var res struct {
		segmentID uint32
		error     []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode_dry,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.segmentID, res.error
}

// decoderDecode - decode all segments from given stream dump through C++ decoder.
func walDecoderRestoreFromStream(
	decoder uintptr,
	segment []byte,
	segmentID uint32,
) (offset uint64, rSegmentID uint32, err []byte) {
	args := struct {
		decoder   uintptr
		segment   []byte
		segmentID uint32
	}{decoder, segment, segmentID}
	var res struct {
		offset    uint64
		segmentID uint32
		error     []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_restore_from_stream,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.offset, res.segmentID, res.error
}

// walDecoderDtor - wrapper for destructor C-Decoder.
func walDecoderDtor(decoder uintptr) {
	args := struct {
		decoder uintptr
	}{decoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_decoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// OutputDecoder
//

// walOutputDecoderCtor - wrapper for constructor C-WalOutputDecoder.
func walOutputDecoderCtor(
	externalLabels []Label,
	statelessRelabeler, outputLss uintptr,
	encodersVersion uint8,
) uintptr {
	args := struct {
		externalLabels     []Label
		statelessRelabeler uintptr
		outputLss          uintptr
		encodersVersion    uint8
	}{externalLabels, statelessRelabeler, outputLss, encodersVersion}
	var res struct {
		decoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

// walOutputDecoderDtor - wrapper for destructor C-WalOutputDecoder.
func walOutputDecoderDtor(decoder uintptr) {
	args := struct {
		decoder uintptr
	}{decoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_output_decoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// walOutputDecoderDumpTo dump output decoder state(output_lss and cache) to slice byte.
func walOutputDecoderDumpTo(decoder uintptr) (dump, err []byte) {
	args := struct {
		decoder uintptr
	}{decoder}
	var res struct {
		dump  []byte
		error []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_dump_to,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.dump, res.error
}

// walOutputDecoderLoadFrom load from dump(slice byte) output decoder state(output_lss and cache).
func walOutputDecoderLoadFrom(decoder uintptr, dump []byte) []byte {
	args := struct {
		dump    []byte
		decoder uintptr
	}{dump, decoder}
	var res struct {
		error []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_load_from,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.error
}

// walOutputDecoderDecode decode segment to slice RefSample.
func walOutputDecoderDecode(
	segment []byte,
	decoder uintptr,
	lowerLimitTimestamp int64,
) (stats OutputDecoderStats, dump []RefSample, err []byte) {
	args := struct {
		segment             []byte
		decoder             uintptr
		lowerLimitTimestamp int64
	}{segment, decoder, lowerLimitTimestamp}
	var res struct {
		OutputDecoderStats
		refSamples []RefSample
		error      []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_decode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.OutputDecoderStats, res.refSamples, res.error
}

//
// ProtobufEncoder
//

// walProtobufEncoderCtor - wrapper for constructor C-ProtobufEncoder.
func walProtobufEncoderCtor(outputLsses []uintptr) uintptr {
	args := struct {
		outputLsses []uintptr
	}{outputLsses}
	var res struct {
		decoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

// walProtobufEncoderDtor - wrapper for destructor C-ProtobufEncoder.
func walProtobufEncoderDtor(decoder uintptr) {
	args := struct {
		decoder uintptr
	}{decoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_protobuf_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// walProtobufEncoderEncode encode batch slice ShardRefSamples to snapped protobufs on shards.
func walProtobufEncoderEncode(
	batch []*DecodedRefSamples,
	outSlices [][]byte,
	stats []protobufEncoderStats,
	encoder uintptr,
) []byte {
	args := struct {
		batch     []*DecodedRefSamples
		outSlices [][]byte
		stats     []protobufEncoderStats
		encoder   uintptr
	}{batch, outSlices, stats, encoder}
	var res struct {
		error []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_encoder_encode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.error
}

//
// LabelSetStorage EncodingBimap
//

// primitivesLSSCtor - wrapper for constructor C-Lss.
func primitivesLSSCtor(lss_type uint32) uintptr {
	args := struct {
		lss_type uint32
	}{lss_type}
	var res struct {
		lss uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.lss
}

// primitivesLSSDtor - wrapper for destructor C-EncodingBimap.
func primitivesLSSDtor(lss uintptr) {
	args := struct {
		lss uintptr
	}{lss}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_primitives_lss_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// primitivesLSSAllocatedMemory -  return size of allocated memory for label sets in C++.
func primitivesLSSAllocatedMemory(lss uintptr) uint64 {
	args := struct {
		lss uintptr
	}{lss}
	var res struct {
		allocatedMemory uint64
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_allocated_memory,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.allocatedMemory
}

type FindOrEmplaceResult struct {
	LabelSetID          uint32
	LssHasReallocations bool
}

func primitivesLSSFindOrEmplace(lss uintptr, labelSet model.LabelSet) FindOrEmplaceResult {
	args := struct {
		lss      uintptr
		labelSet model.LabelSet
	}{lss, labelSet}
	var res FindOrEmplaceResult

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_find_or_emplace,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res
}

func primitivesLSSFindOrEmplaceBuilder(lss uintptr, builder CppLabelSetBuilder) FindOrEmplaceResult {
	args := struct {
		lss     uintptr
		builder CppLabelSetBuilder
	}{lss, builder}
	var res FindOrEmplaceResult

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_find_or_emplace_builder,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res
}

func primitivesLSSQuerySelector(lss uintptr, matchers []model.LabelMatcher) (
	selector uintptr,
	status uint32,
) {
	args := struct {
		lss      uintptr
		matchers []model.LabelMatcher
	}{lss, matchers}

	var res struct {
		selector uintptr
		status   uint32
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_query_selector,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.selector, res.status
}

func primitivesLSSQuery(lss uintptr, selector uintptr) (
	matches []uint32,
	labelSetLengths []uint16,
	status uint32,
) {
	args := struct {
		lss      uintptr
		selector uintptr
	}{lss, selector}

	var res struct {
		matches         []uint32
		labelSetLengths []uint16
		status          uint32
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_query,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.matches, res.labelSetLengths, res.status
}

func primitivesLabelSetMatchesFree(result *LSSQueryResult) {
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_primitives_lss_query_result_free,
		uintptr(unsafe.Pointer(result)),
	)
}

func primitivesLSSGetLabelSets(lss uintptr, labelSetIDs []uint32) []Labels {
	args := struct {
		lss         uintptr
		labelSetIDs []uint32
	}{lss, labelSetIDs}
	var res struct {
		labelSets []Labels
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_get_label_sets,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.labelSets
}

func primitivesLSSFreeLabelSets(labelSets []Labels) {
	args := struct {
		labelSets []Labels
	}{labelSets}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_primitives_lss_free_label_sets,
		uintptr(unsafe.Pointer(&args)),
	)
}

func primitivesLSSQueryLabelNames(lss uintptr, matchers []model.LabelMatcher) (uint32, []string) {
	args := struct {
		lss      uintptr
		matchers []model.LabelMatcher
	}{lss, matchers}
	var res struct {
		status uint32
		names  []string
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_query_label_names,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.status, res.names
}

func primitivesLSSQueryLabelValues(lss uintptr, label_name string, matchers []model.LabelMatcher) (uint32, []string) {
	args := struct {
		lss        uintptr
		label_name string
		matchers   []model.LabelMatcher
	}{lss, label_name, matchers}
	var res struct {
		status uint32
		values []string
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_query_label_values,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.status, res.values
}

func primitivesLSSCreateReadonlyLss(lss uintptr) uintptr {
	args := struct {
		lss uintptr
	}{lss}
	var res struct {
		lss uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_create_readonly_lss,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.lss
}

// primitivesLSSBitsetSeries returns a copy of the bitset of added series from the lss.
func primitivesLSSBitsetSeries(lss uintptr) uintptr {
	args := struct {
		lss uintptr
	}{lss}
	var res struct {
		bitset uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_bitset_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.bitset
}

// primitivesLSSBitsetDtor destroy bitset of added series.
func primitivesLSSBitsetDtor(bitset uintptr) {
	args := struct {
		bitset uintptr
	}{bitset}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_primitives_lss_bitset_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// primitivesReadonlyLSSCopyAddedSeries copy the label sets from the source lss to the destination lss
// that were added source lss.
func primitivesReadonlyLSSCopyAddedSeries(source, sourceBitset, destination uintptr) uintptr {
	var dstSrcLsIdsMapping uintptr

	C.prompp_primitives_readonly_lss_copy_added_series(
		C.uint64_t(source),
		C.uint64_t(sourceBitset),
		C.uint64_t(destination),
		C.uint64_t(uintptr(unsafe.Pointer(&dstSrcLsIdsMapping))),
	)

	return dstSrcLsIdsMapping
}

func primitivesFreeLsIdsMapping(lsIdsMapping uintptr) {
	args := struct {
		lsIdsMapping uintptr
	}{lsIdsMapping}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_primitives_free_ls_ids_mapping,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// StatelessRelabeler
//

// prometheusStatelessRelabelerCtor - wrapper for constructor C-StatelessRelabeler.
func prometheusStatelessRelabelerCtor(cfgs []*RelabelConfig) (statelessRelabeler uintptr, exception []byte) {
	args := struct {
		cfgs []*RelabelConfig
	}{cfgs}
	var res struct {
		statelessRelabeler uintptr
		exception          []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_stateless_relabeler_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.statelessRelabeler, res.exception
}

// prometheusStatelessRelabelerDtor - wrapper for destructor C-StatelessRelabeler.
func prometheusStatelessRelabelerDtor(statelessRelabeler uintptr) {
	args := struct {
		statelessRelabeler uintptr
	}{statelessRelabeler}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_stateless_relabeler_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusStatelessRelabelerResetTo reset configs and replace on new converting go-config..
func prometheusStatelessRelabelerResetTo(statelessRelabeler uintptr, cfgs []*RelabelConfig) (exception []byte) {
	args := struct {
		statelessRelabeler uintptr
		cfgs               []*RelabelConfig
	}{statelessRelabeler, cfgs}
	var res struct {
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_stateless_relabeler_reset_to,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.exception
}

//
// InnerSeries
//

// prometheusInnerSeriesCtor - wrapper for constructor C-InnerSeries(vector).
func prometheusInnerSeriesCtor(innerSeries []InnerSeries) {
	args := struct {
		innerSeries []InnerSeries
	}{innerSeries}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_inner_series_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusInnerSeriesDtor - wrapper for destructor C-InnerSeries(vector).
func prometheusInnerSeriesDtor(innerSeries []InnerSeries) {
	args := struct {
		innerSeries []InnerSeries
	}{innerSeries}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_inner_series_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusInnerSeriesReset - wrapper for reset C-InnerSeries(vector).
func prometheusInnerSeriesReset(innerSeries []InnerSeries) {
	args := struct {
		innerSeries []InnerSeries
	}{innerSeries}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_inner_series_reset,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// RelabeledSeries
//

// prometheusRelabeledSeriesCtor - wrapper for constructor C-RelabeledSeries(vector).
func prometheusRelabeledSeriesCtor(relabeledSeries []RelabeledSeries) {
	args := struct {
		relabeledSeries []RelabeledSeries
	}{relabeledSeries}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeled_series_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabeledSeriesDtor - wrapper for destructor C-RelabeledSeries(vector).
func prometheusRelabeledSeriesDtor(relabeledSeries []RelabeledSeries) {
	args := struct {
		relabeledSeries []RelabeledSeries
	}{relabeledSeries}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeled_series_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabeledSeriesReset - wrapper for reset C-RelabeledSeries(vector).
func prometheusRelabeledSeriesReset(relabeledSeries []RelabeledSeries) {
	args := struct {
		relabeledSeries []RelabeledSeries
	}{relabeledSeries}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeled_series_reset,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// RelabelerStateUpdate
//

// prometheusRelabelerStateUpdateCtor - wrapper for constructor C-RelabelerStateUpdate(vector), filling in c++.
func prometheusRelabelerStateUpdateCtor(relabelerStateUpdate []RelabelerStateUpdate) {
	args := struct {
		relabelerStateUpdate []RelabelerStateUpdate
	}{relabelerStateUpdate}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeler_state_update_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabelerStateUpdateDtor - wrapper for destructor C-RelabelerStateUpdate(vector).
func prometheusRelabelerStateUpdateDtor(relabelerStateUpdate []RelabelerStateUpdate) {
	args := struct {
		relabelerStateUpdate []RelabelerStateUpdate
	}{relabelerStateUpdate}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeler_state_update_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabelerStateUpdateReset - wrapper for reset C-RelabelerStateUpdate(vector).
func prometheusRelabelerStateUpdateReset(relabelerStateUpdate []RelabelerStateUpdate) {
	args := struct {
		relabelerStateUpdate []RelabelerStateUpdate
	}{relabelerStateUpdate}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeler_state_update_reset,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// StaleNansState
//

func prometheusRelabelStaleNansStateCtor() uintptr {
	var res struct {
		state uintptr
	}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabel_stale_nans_state_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.state
}

func prometheusRelabelStaleNansStateDtor(state uintptr) {
	args := struct {
		state uintptr
	}{state}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabel_stale_nans_state_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// PerShardRelabeler
//

// prometheusPerShardRelabelerCtor - wrapper for constructor C-PerShardRelabeler.
func prometheusPerShardRelabelerCtor(
	externalLabels []Label,
	statelessRelabeler uintptr,
	numberOfShards, shardID uint16,
) (perShardRelabeler uintptr, exception []byte) {
	args := struct {
		externalLabels     []Label
		statelessRelabeler uintptr
		numberOfShards     uint16
		shardID            uint16
	}{externalLabels, statelessRelabeler, numberOfShards, shardID}
	var res struct {
		perShardRelabeler uintptr
		exception         []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.perShardRelabeler, res.exception
}

// prometheusPerShardRelabelerDtor - wrapper for destructor C-PerShardRelabeler.
func prometheusPerShardRelabelerDtor(perShardRelabeler uintptr) {
	args := struct {
		perShardRelabeler uintptr
	}{perShardRelabeler}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_shard_relabeler_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusPerShardSingleRelabelerUpdateRelabelerState - wrapper for add to cache relabled data(third stage).
func prometheusPerShardSingleRelabelerUpdateRelabelerState(
	relabelerStateUpdate *RelabelerStateUpdate,
	perShardRelabeler, cache uintptr,
	relabeledShardID uint16,
) []byte {
	args := struct {
		relabelerStateUpdate *RelabelerStateUpdate
		perShardRelabeler    uintptr
		cache                uintptr
		relabeledShardID     uint16
	}{relabelerStateUpdate, perShardRelabeler, cache, relabeledShardID}
	var res struct {
		exception []byte
	}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_single_relabeler_update_relabeler_state,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	inputRelabelerUpdateRelabelerStateSum.Add(float64(time.Now().UnixNano() - start))
	inputRelabelerUpdateRelabelerStateCount.Inc()

	return res.exception
}

// prometheusPerShardRelabelerOutputRelabeling - wrapper for relabeling output series(fourth stage).
func prometheusPerShardRelabelerOutputRelabeling(
	perShardRelabeler, lss, cache uintptr,
	incomingInnerSeries, encodersInnerSeries []InnerSeries,
	relabeledSeries *RelabeledSeries,
) []byte {
	args := struct {
		relabeledSeries     *RelabeledSeries
		incomingInnerSeries []InnerSeries
		encodersInnerSeries []InnerSeries
		perShardRelabeler   uintptr
		lss                 uintptr
		cache               uintptr
	}{relabeledSeries, incomingInnerSeries, encodersInnerSeries, perShardRelabeler, lss, cache}
	var res struct {
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_output_relabeling,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.exception
}

// prometheusPerShardRelabelerResetTo - reset set new number_of_shards and external_labels.
func prometheusPerShardRelabelerResetTo(
	externalLabels []Label,
	perShardRelabeler uintptr,
	numberOfShards uint16,
) {
	args := struct {
		externalLabels    []Label
		perShardRelabeler uintptr
		numberOfShards    uint16
	}{externalLabels, perShardRelabeler, numberOfShards}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_shard_relabeler_reset_to,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataDataStorageCtor() uintptr {
	var res struct {
		dataStorage uintptr
	}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.dataStorage
}

func seriesDataDataStorageReset(dataStorage uintptr) {
	args := struct {
		dataStorage uintptr
	}{dataStorage}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_reset,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataDataStorageAllocatedMemory(dataStorage uintptr) uint64 {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	var res struct {
		allocatedMemory uint64
	}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_allocated_memory,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	headDataStorageAllocatedMemorySum.Add(float64(time.Now().UnixNano() - start))
	headDataStorageAllocatedMemoryCount.Inc()

	return res.allocatedMemory
}

type DataStorageQueryResult struct {
	Querier        uintptr
	Status         uint8
	SerializedData *DataStorageSerializedData
}

func seriesDataDataStorageQueryV2(dataStorage uintptr, query DataStorageQuery, serializedData *DataStorageSerializedData, downsamplingMs int64) (querier uintptr, status uint8) {
	args := struct {
		dataStorage    uintptr
		query          DataStorageQuery
		downsamplingMs int64
	}{dataStorage, query, downsamplingMs}

	res := struct {
		Querier        uintptr
		Status         uint8
		SerializedData *uintptr
	}{
		SerializedData: &serializedData.serializedData,
	}

	testGC()
	start := time.Now().UnixNano()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_query_v2,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	headDataStorageQuerySum.Add(float64(time.Now().UnixNano() - start))
	headDataStorageQueryCount.Inc()

	return res.Querier, res.Status
}

func seriesDataDataStorageInstantQuery(dataStorage uintptr, labelSetIDs []uint32, timestamp int64, samples uintptr) DataStorageQueryResult {
	args := struct {
		dataStorage uintptr
		labelSetIDs []uint32
		timestamp   int64
		samples     uintptr
	}{dataStorage, labelSetIDs, timestamp, samples}
	var res DataStorageQueryResult

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_instant_query,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res
}

func seriesDataDataStorageQueryFinal(queriers []uintptr) {
	args := struct {
		queriers []uintptr
	}{queriers}

	testGC()
	start := time.Now().UnixNano()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_query_final,
		uintptr(unsafe.Pointer(&args)),
	)
	headDataStorageQueryFinalSum.Add(float64(time.Now().UnixNano() - start))
	headDataStorageQueryFinalCount.Inc()
}

func seriesDataSerializedDataDtor(serializedData uintptr) {
	args := struct {
		serializedData uintptr
	}{serializedData}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_serialization_serialized_data_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataSerializedDataNext(serializedData uintptr) (uint32, uint32) {
	args := struct {
		serializedData uintptr
	}{serializedData}
	res := struct {
		seriesID uint32
		chunkRef uint32
	}{}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_serialization_serialized_data_next,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.seriesID, res.chunkRef
}

func seriesDataSerializedDataIteratorCtor(iterator *DataStorageSerializedDataIterator, serializedData uintptr, chunkRef uint32) {
	args := struct {
		iterator       uintptr
		serializedData uintptr
		chunkRef       uint32
	}{uintptr(unsafe.Pointer(iterator)), serializedData, chunkRef}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_serialization_serialized_data_iterator_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataSerializedDataIteratorNext(iterator *DataStorageSerializedDataIterator) {
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_serialization_serialized_data_iterator_next,
		uintptr(unsafe.Pointer(iterator)),
	)
}

func seriesDataSerializedDataIteratorSeek(iterator *DataStorageSerializedDataIterator, targetTimestamp int64) {
	args := struct {
		iterator        uintptr
		targetTimestamp int64
	}{uintptr(unsafe.Pointer(iterator)), targetTimestamp}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_serialization_serialized_data_iterator_seek,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataSerializedDataIteratorReset(iterator *DataStorageSerializedDataIterator, serializedData uintptr, chunkRef uint32) {
	args := struct {
		iterator       uintptr
		serializedData uintptr
		chunkRef       uint32
	}{uintptr(unsafe.Pointer(iterator)), serializedData, chunkRef}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_serialization_serialized_data_iterator_reset,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataDataStorageTimeInterval(dataStorage uintptr) TimeInterval {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	res := struct {
		interval TimeInterval
	}{}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_time_interval,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.interval
}

func seriesDataDataStorageQueriedSeriesBitsetSize(dataStorage uintptr) uint32 {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	res := struct {
		size uint32
	}{}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_queried_series_bitset_size,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.size
}

func seriesDataDataStorageQueriedSeriesBitset(dataStorage uintptr, queriedSeriesBitset []byte) []byte {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	res := struct {
		queriedSeriesBitset []byte
	}{queriedSeriesBitset}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_queried_series_bitset,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.queriedSeriesBitset
}

func seriesDataDataStorageQueriedSeriesSetBitset(dataStorage uintptr, queriedSeriesBitset []byte) bool {
	args := struct {
		dataStorage         uintptr
		queriedSeriesBitset []byte
	}{dataStorage, queriedSeriesBitset}
	res := struct {
		result bool
	}{}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_queried_series_set_bitset,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.result
}

func seriesDataDataStorageDtor(dataStorage uintptr) {
	args := struct {
		dataStorage uintptr
	}{dataStorage}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataEncoderCtor(dataStorage uintptr) uintptr {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	var res struct {
		encoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

func seriesDataEncoderEncode(encoder uintptr, seriesID uint32, timestamp int64, value float64) {
	args := struct {
		encoder   uintptr
		seriesID  uint32
		timestamp int64
		value     float64
	}{encoder, seriesID, timestamp, value}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_encode,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataEncoderEncodeInnerSeriesSlice(encoder uintptr, innerSeriesSlice []InnerSeries) {
	args := struct {
		encoder          uintptr
		innerSeriesSlice []InnerSeries
	}{encoder, innerSeriesSlice}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_encode_inner_series_slice,
		uintptr(unsafe.Pointer(&args)),
	)
	headDataStorageEncodeInnerSeriesSliceSum.Add(float64(time.Now().UnixNano() - start))
	headDataStorageEncodeInnerSeriesSliceCount.Inc()
}

func seriesDataEncoderMergeOutOfOrderChunks(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_merge_out_of_order_chunks,
		uintptr(unsafe.Pointer(&args)),
	)
	headDataStorageMergeOutOfOrderChunksSum.Add(float64(time.Now().UnixNano() - start))
	headDataStorageMergeOutOfOrderChunksCount.Inc()
}

func seriesDataEncoderDtor(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataChunkRecoderCtor(lss uintptr, lsIdBatchSize uint32, dataStorage uintptr, timeInterval TimeInterval, downsamplingMs int64) uintptr {
	args := struct {
		lss           uintptr
		lsIdBatchSize uint32
		dataStorage   uintptr
		TimeInterval
		downsamplingMs int64
	}{lss, lsIdBatchSize, dataStorage, timeInterval, downsamplingMs}
	var res struct {
		chunkRecoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_chunk_recoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.chunkRecoder
}

func seriesDataSerializedChunkRecoderCtor(serializedData *DataStorageSerializedData, timeInterval TimeInterval) uintptr {
	args := struct {
		serializedData *uintptr
		TimeInterval
	}{&serializedData.serializedData, timeInterval}
	var res struct {
		chunkRecoder uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_serialized_chunk_recoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.chunkRecoder
}

func seriesDataChunkRecoderRecodeNextChunk(chunkRecoder uintptr, recodedChunk *RecodedChunk) {
	args := struct {
		chunkRecoder uintptr
	}{chunkRecoder}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_chunk_recoder_recode_next_chunk,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(recodedChunk)),
	)
	chunkRecoderRecodeNextChunkSum.Add(float64(time.Now().UnixNano() - start))
	chunkRecoderRecodeNextChunkCount.Inc()
}

func seriesDataChunkRecoderNextBatch(chunkRecoder uintptr) bool {
	args := struct {
		chunkRecoder uintptr
	}{chunkRecoder}
	var res struct {
		hasMoreData bool
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_chunk_recoder_next_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hasMoreData
}

func seriesDataChunkRecoderDtor(chunkRecoder uintptr) {
	args := struct {
		chunkRecoder uintptr
	}{chunkRecoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_chunk_recoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataUnusedSeriesDataUnloaderCtor(dataStorage uintptr) uintptr {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	var res struct {
		unloader uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_unloader_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.unloader
}

func seriesDataUnusedSeriesDataUnloaderDtor(unloader uintptr) {
	args := struct {
		unloader uintptr
	}{unloader}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_unloader_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataUnusedSeriesDataUnloaderCreateSnapshot(unloader uintptr) []byte {
	args := struct {
		unloader uintptr
	}{unloader}
	var res struct {
		snapshot []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_unloader_create_snapshot,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.snapshot
}

func seriesDataUnusedSeriesDataUnloaderUnload(unloader uintptr) {
	args := struct {
		unloader uintptr
	}{unloader}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_unloader_unload,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataUnloadedDataLoaderCtor(dataStorage uintptr, queriers []uintptr) uintptr {
	args := struct {
		dataStorage uintptr
		queriers    []uintptr
	}{dataStorage, queriers}
	var res struct {
		loader uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_loader_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.loader
}

func seriesDataUnloadedDataRevertableLoaderCtor(lss uintptr, lsIdBatchSize uint32, dataStorage uintptr) uintptr {
	args := struct {
		lss           uintptr
		lsIdBatchSize uint32
		dataStorage   uintptr
	}{lss, lsIdBatchSize, dataStorage}
	var res struct {
		loader uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_revertable_loader_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.loader
}

func seriesDataUnloadedDataLoaderLoad(loader uintptr, snapshot []byte, isLast bool) {
	args := struct {
		loader   uintptr
		snapshot []byte
		isLast   bool
	}{loader, snapshot, isLast}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_loader_load_next,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataUnloadedDataRevertableLoaderNextBatch(loader uintptr) bool {
	args := struct {
		loader uintptr
	}{loader}
	var res struct {
		hasMoreData bool
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_revertable_loader_next_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hasMoreData
}

func seriesDataUnloadedDataLoaderDtor(loader uintptr) {
	args := struct {
		loader uintptr
	}{loader}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_loader_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func indexWriterCtor(lss uintptr) uintptr {
	args := struct {
		lss uintptr
	}{lss}

	var res struct {
		writer uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.writer
}

func indexWriterDtor(writer uintptr) {
	args := struct {
		writer uintptr
	}{writer}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_index_writer_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func indexWriterWriteHeader(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_header,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteSymbols(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_symbols,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteNextSeriesBatch(writer uintptr, ls_id uint32, chunks_meta []ChunkMetadata, data []byte) []byte {
	args := struct {
		writer      uintptr
		chunks_meta []ChunkMetadata
		ls_id       uint32
	}{writer, chunks_meta, ls_id}

	res := struct {
		data []byte
	}{data}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_next_series_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteLabelIndices(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_label_indices,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteNextPostingsBatch(writer uintptr, max_batch_size uint32, data []byte) ([]byte, bool) {
	args := struct {
		writer         uintptr
		max_batch_size uint32
	}{writer, max_batch_size}

	res := struct {
		data          []byte
		has_more_data bool
	}{data, false}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_next_postings_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data, res.has_more_data
}

func indexWriterWriteLabelIndicesTable(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_label_indices_table,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWritePostingsTableOffsets(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_postings_table_offsets,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteTableOfContents(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_table_of_contents,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func freeHeadStatus(status *HeadStatus) {
	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_free_head_status,
		uintptr(unsafe.Pointer(status)),
	)
}

func getHeadStatusLSS(lss uintptr, status *HeadStatus, limit int) {
	args := struct {
		lss   uintptr
		limit int
	}{lss, limit}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_get_head_status_lss,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(status)),
	)
}

func getHeadStatusDataStorage(dataStorage uintptr, status *HeadStatus) {
	args := struct {
		dataStorage uintptr
	}{dataStorage}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_get_head_status_data_storage,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(status)),
	)
}

//
// Prometheus Scraper
//

func walPrometheusScraperHashdexCtor() uintptr {
	var res struct {
		hashdex uintptr
	}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_prometheus_scraper_hashdex_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walPrometheusScraperHashdexParse(hashdex uintptr, buffer []byte, default_timestamp int64) (uint32, uint32) {
	args := struct {
		hashdex           uintptr
		buffer            []byte
		default_timestamp int64
	}{hashdex, buffer, default_timestamp}
	var res struct {
		error   uint32
		scraped uint32
	}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_prometheus_scraper_hashdex_parse,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	prometheusHashdexParseSum.Add(float64(time.Now().UnixNano() - start))
	prometheusHashdexParseCount.Inc()

	return res.scraped, res.error
}

func walPrometheusScraperHashdexGetMetadata(hashdex uintptr) []WALScraperHashdexMetadata {
	args := struct {
		hashdex uintptr
	}{hashdex}
	var res struct {
		metadata []WALScraperHashdexMetadata
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_prometheus_scraper_hashdex_get_metadata,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.metadata
}

//
// OpenMetrics scraper
//

func walOpenMetricsScraperHashdexCtor() uintptr {
	var res struct {
		hashdex uintptr
	}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_wal_open_metrics_scraper_hashdex_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walOpenMetricsScraperHashdexParse(hashdex uintptr, buffer []byte, default_timestamp int64) (uint32, uint32) {
	args := struct {
		hashdex           uintptr
		buffer            []byte
		default_timestamp int64
	}{hashdex, buffer, default_timestamp}
	var res struct {
		error   uint32
		scraped uint32
	}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_open_metrics_scraper_hashdex_parse,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	openMetricsHashdexParseSum.Add(float64(time.Now().UnixNano() - start))
	openMetricsHashdexParseCount.Inc()

	return res.scraped, res.error
}

func walOpenMetricsScraperHashdexGetMetadata(hashdex uintptr) []WALScraperHashdexMetadata {
	args := struct {
		hashdex uintptr
	}{hashdex}
	var res struct {
		metadata []WALScraperHashdexMetadata
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_wal_open_metrics_scraper_hashdex_get_metadata,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.metadata
}

//
// Relabeler cache
//

// prometheusCacheCtor wrapper for constructor C-Cache.
func prometheusCacheCtor() uintptr {
	var res struct {
		cache uintptr
	}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_cache_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cache
}

// prometheusCacheDtor wrapper for destructor C-Cache.
func prometheusCacheDtor(cache uintptr) {
	args := struct {
		cache uintptr
	}{cache}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_cache_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusCacheAllocatedMemory return size of allocated memory for caches.
func prometheusCacheAllocatedMemory(cache uintptr) uint64 {
	args := struct {
		cache uintptr
	}{cache}
	var res struct {
		cacheAllocatedMemory uint64
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_cache_allocated_memory,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cacheAllocatedMemory
}

// prometheusCacheUpdate add to cache relabled data(third stage).
func prometheusCacheUpdate(
	shardsRelabelerStateUpdate []RelabelerStateUpdate,
	cache uintptr,
) []byte {
	args := struct {
		relabelerStateUpdates []RelabelerStateUpdate
		cache                 uintptr
	}{shardsRelabelerStateUpdate, cache}
	var res struct {
		exception []byte
	}
	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_cache_update,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	inputRelabelerUpdateRelabelerStateSum.Add(float64(time.Now().UnixNano() - start))
	inputRelabelerUpdateRelabelerStateCount.Inc()

	return res.exception
}

func headWalEncoderCtor(shardID uint16, logShards uint8, lss uintptr) uintptr {
	args := struct {
		shardID   uint16
		logShards uint8
		lss       uintptr
	}{shardID, logShards, lss}

	res := struct {
		encoder uintptr
	}{}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

func headWalEncoderAddInnerSeries(encoder uintptr, innerSeries []InnerSeries) (samples uint32, err error) {
	args := struct {
		innerSeries []InnerSeries
		encoder     uintptr
	}{innerSeries, encoder}
	var res struct {
		exception []byte
		samples   uint32
	}

	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_add_inner_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	headWalEncoderAddInnerSeriesSum.Add(float64(time.Now().UnixNano() - start))
	headWalEncoderAddInnerSeriesCount.Inc()

	return res.samples, handleException(res.exception)
}

// headWalEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func headWalEncoderFinalize(encoder uintptr) (samples uint32, segment []byte, err error) {
	args := struct {
		encoder uintptr
	}{encoder}
	var res struct {
		segment   []byte
		exception []byte
		samples   uint32
	}

	start := time.Now().UnixNano()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_finalize,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	headWalEncoderFinalizeSum.Add(float64(time.Now().UnixNano() - start))
	headWalEncoderFinalizeCount.Inc()

	return res.samples, res.segment, handleException(res.exception)
}

func headWalEncoderDtor(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_head_wal_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func headWalDecoderCtor(lss uintptr, encoderVersion uint8) uintptr {
	args := struct {
		lss            uintptr
		encoderVersion uint8
	}{lss, encoderVersion}

	res := struct {
		decoder uintptr
	}{}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_head_wal_decoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

func headWalDecoderDecode(decoder uintptr, segment []byte, innerSeries *InnerSeries) error {
	args := struct {
		decoder     uintptr
		segment     []byte
		innerSeries *InnerSeries
	}{decoder, segment, innerSeries}
	var res struct {
		DecodedSegmentStats
		exception []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_head_wal_decoder_decode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return handleException(res.exception)
}

func headWalDecoderDecodeToDataStorage(decoder uintptr, segment []byte, encoder uintptr) (int64, int64, error) {
	args := struct {
		decoder uintptr
		segment []byte
		encoder uintptr
	}{decoder, segment, encoder}
	var res struct {
		createTimestamp int64
		encodeTimestamp int64
		exception       []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_head_wal_decoder_decode_to_data_storage,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.createTimestamp, res.encodeTimestamp, handleException(res.exception)
}

func headWalDecoderCreateEncoder(decoder uintptr) (uintptr, error) {
	args := struct {
		decoder uintptr
	}{decoder}
	var res struct {
		encoder uintptr
		error   []byte
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_ctor_from_decoder,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder, handleException(res.error)
}

func headWalDecoderDtor(decoder uintptr) {
	args := struct {
		decoder uintptr
	}{decoder}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_head_wal_decoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// label_sets
//

func labelSetLength(lss uintptr, labelSetID uint32) uint64 {
	args := struct {
		lss        uintptr
		labelSetID uint32
	}{lss, labelSetID}
	var res struct {
		length uint64
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_label_set_length,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.length
}

func labelSetSerialize(lss uintptr, labelSetID uint32) []Label {
	args := struct {
		lss        uintptr
		labelSetID uint32
	}{lss, labelSetID}
	var res struct {
		labelSet []Label
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_label_set_serialize,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.labelSet
}

func labelSetFree(labelSet []Label) {
	if labelSet == nil {
		return
	}

	args := struct {
		labelSet []Label
	}{labelSet}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_label_set_free,
		uintptr(unsafe.Pointer(&args)),
	)
}

func allocateSliceForLabelBytes(lss uintptr, labelSetID uint32, bytes []byte) []byte {
	args := struct {
		lss        uintptr
		labelSetID uint32
	}{lss, labelSetID}
	var sizeResult struct {
		size uint32
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_label_set_bytes_size,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&sizeResult)),
	)

	if int(sizeResult.size) > cap(bytes) {
		return make([]byte, sizeResult.size)
	} else {
		return bytes[:sizeResult.size]
	}
}

func LabelSetBytes(lss uintptr, labelSetID uint32, bytes []byte) []byte {
	result := struct {
		bytes []byte
	}{allocateSliceForLabelBytes(lss, labelSetID, bytes)}

	args := struct {
		lss        uintptr
		labelSetID uint32
	}{lss, labelSetID}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_label_set_bytes,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&result)),
	)

	return result.bytes
}

func labelSetBytesWithFilteredNames(cFunction unsafe.Pointer, lss uintptr, labelSetID uint32, bytes []byte, names ...string) []byte {
	result := struct {
		bytes []byte
	}{allocateSliceForLabelBytes(lss, labelSetID, bytes)}

	args := struct {
		lss        uintptr
		labelSetID uint32
		names      []string
	}{lss, labelSetID, names}

	testGC()
	fastcgo.UnsafeCall2(
		cFunction,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&result)),
	)

	return result.bytes
}

func LabelSetBytesWithLabels(lss uintptr, labelSetID uint32, bytes []byte, names ...string) []byte {
	return labelSetBytesWithFilteredNames(C.prompp_label_set_bytes_with_labels, lss, labelSetID, bytes, names...)
}

func LabelSetBytesWithoutLabels(lss uintptr, labelSetID uint32, bytes []byte, names ...string) []byte {
	return labelSetBytesWithFilteredNames(C.prompp_label_set_bytes_without_labels, lss, labelSetID, bytes, names...)
}

//
// PerGoroutineRelabeler
//

// prometheusPerGoroutineRelabelerCtor wrapper for constructor C-PerGoroutineRelabeler.
func prometheusPerGoroutineRelabelerCtor(
	numberOfShards, shardID uint16,
) uintptr {
	args := struct {
		numberOfShards uint16
		shardID        uint16
	}{numberOfShards, shardID}
	var res struct {
		perGoroutineRelabeler uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.perGoroutineRelabeler
}

// prometheusPerGoroutineRelabelerDtor wrapper for destructor C-PerGoroutineRelabeler.
func prometheusPerGoroutineRelabelerDtor(perGoroutineRelabeler uintptr) {
	args := struct {
		perGoroutineRelabeler uintptr
	}{perGoroutineRelabeler}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_goroutine_relabeler_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusPerGoroutineRelabelerInputRelabeling wrapper for relabeling incoming hashdex(first stage).
func prometheusPerGoroutineRelabelerInputRelabeling(
	perGoroutineRelabeler, statelessRelabeler, inputLss, targetLss, cache, hashdex uintptr,
	options RelabelerOptions,
	shardsInnerSeries []InnerSeries,
	shardsRelabeledSeries []RelabeledSeries,
) (stats RelabelerStats, exception []byte, targetLssHasReallocations bool) {
	args := struct {
		shardsInnerSeries     []InnerSeries
		shardsRelabeledSeries []RelabeledSeries
		options               RelabelerOptions
		perGoroutineRelabeler uintptr
		statelessRelabeler    uintptr
		hashdex               uintptr
		cache                 uintptr
		inputLss              uintptr
		targetLss             uintptr
	}{
		shardsInnerSeries,
		shardsRelabeledSeries,
		options,
		perGoroutineRelabeler,
		statelessRelabeler,
		hashdex,
		cache,
		inputLss,
		targetLss,
	}
	var res struct {
		RelabelerStats
		exception                 []byte
		targetLssHasReallocations bool
	}
	start := time.Now()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_input_relabeling,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	perGoroutineRelabelerInputRelabelingSum.Add(float64(time.Since(start).Nanoseconds()))
	perGoroutineRelabelerInputRelabelingCount.Inc()

	return res.RelabelerStats, res.exception, res.targetLssHasReallocations
}

// prometheusPerGoroutineRelabelerInputRelabelingFromCache wrapper for relabeling
// incoming hashdex(first stage) from cache.
func prometheusPerGoroutineRelabelerInputRelabelingFromCache(
	perGoroutineRelabeler, inputLss, targetLss, cache, hashdex uintptr,
	options RelabelerOptions,
	shardsInnerSeries []InnerSeries,
) (stats RelabelerStats, exception []byte, ok bool) {
	args := struct {
		shardsInnerSeries     []InnerSeries
		options               RelabelerOptions
		perGoroutineRelabeler uintptr
		hashdex               uintptr
		cache                 uintptr
		inputLss              uintptr
		targetLss             uintptr
	}{shardsInnerSeries, options, perGoroutineRelabeler, hashdex, cache, inputLss, targetLss}
	var res struct {
		RelabelerStats
		ok        bool
		exception []byte
	}
	start := time.Now()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_input_relabeling_from_cache,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	perGoroutineRelabelerInputRelabelingFromCacheSum.Add(float64(time.Since(start).Nanoseconds()))
	perGoroutineRelabelerInputRelabelingFromCacheCount.Inc()

	return res.RelabelerStats, res.exception, res.ok
}

// prometheusPerGoroutineRelabelerInputRelabelingWithStalenans wrapper for relabeling incoming
// hashdex(first stage) with state stalenans.
func prometheusPerGoroutineRelabelerInputRelabelingWithStalenans(
	perGoroutineRelabeler, statelessRelabeler, inputLss, targetLss, cache, hashdex uintptr,
	defTimestamp int64,
	options RelabelerOptions,
	shardsInnerSeries []InnerSeries,
	shardsRelabeledSeries []RelabeledSeries,
) (stats RelabelerStats, exception []byte, targetLssHasReallocations bool) {
	args := struct {
		shardsInnerSeries     []InnerSeries
		shardsRelabeledSeries []RelabeledSeries
		options               RelabelerOptions
		perGoroutineRelabeler uintptr
		statelessRelabeler    uintptr
		hashdex               uintptr
		cache                 uintptr
		inputLss              uintptr
		targetLss             uintptr
		defTimestamp          int64
	}{
		shardsInnerSeries,
		shardsRelabeledSeries,
		options,
		perGoroutineRelabeler,
		statelessRelabeler,
		hashdex,
		cache,
		inputLss,
		targetLss,
		defTimestamp,
	}
	var res struct {
		RelabelerStats
		exception                 []byte
		targetLssHasReallocations bool
	}
	start := time.Now()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	perGoroutineRelabelerInputRelabelingWithStalenansSum.Add(float64(time.Since(start).Nanoseconds()))
	perGoroutineRelabelerInputRelabelingWithStalenansCount.Inc()

	return res.RelabelerStats, res.exception, res.targetLssHasReallocations
}

// prometheusPerGoroutineRelabelerInputRelabelingWithStalenansFromCache wrapper for relabeling incoming from cache
// hashdex(first stage) with state stalenans.
func prometheusPerGoroutineRelabelerInputRelabelingWithStalenansFromCache(
	perGoroutineRelabeler, inputLss, targetLss, cache, hashdex uintptr,
	defTimestamp int64,
	options RelabelerOptions,
	shardsInnerSeries []InnerSeries,
) (stats RelabelerStats, exception []byte, targetLssHasReallocations bool) {
	args := struct {
		shardsInnerSeries     []InnerSeries
		options               RelabelerOptions
		perGoroutineRelabeler uintptr
		hashdex               uintptr
		cache                 uintptr
		inputLss              uintptr
		targetLss             uintptr
		defTimestamp          int64
	}{
		shardsInnerSeries,
		options,
		perGoroutineRelabeler,
		hashdex,
		cache,
		inputLss,
		targetLss,
		defTimestamp,
	}
	var res struct {
		RelabelerStats
		ok        bool
		exception []byte
	}
	start := time.Now()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans_from_cache,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	perGoroutineRelabelerInputRelabelingWithStalenansFromCacheSum.Add(float64(time.Since(start).Nanoseconds()))
	perGoroutineRelabelerInputRelabelingWithStalenansFromCacheCount.Inc()

	return res.RelabelerStats, res.exception, res.ok
}

// prometheusPerGoroutineRelabelerInputTransitionRelabeling wrapper for
// transparent relabeling incoming hashdex(first stage).
func prometheusPerGoroutineRelabelerInputTransitionRelabeling(
	perGoroutineRelabeler, targetLss, hashdex uintptr,
	shardsInnerSeries []InnerSeries,
) (stats RelabelerStats, exception []byte, targetLssHasReallocations bool) {
	args := struct {
		shardsInnerSeries     []InnerSeries
		perGoroutineRelabeler uintptr
		hashdex               uintptr
		targetLss             uintptr
	}{
		shardsInnerSeries,
		perGoroutineRelabeler,
		hashdex,
		targetLss,
	}
	var res struct {
		RelabelerStats
		exception                 []byte
		targetLssHasReallocations bool
	}
	start := time.Now()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	perGoroutineRelabelerInputTransitionRelabelingSum.Add(float64(time.Since(start).Nanoseconds()))
	perGoroutineRelabelerInputTransitionRelabelingCount.Inc()

	return res.RelabelerStats, res.exception, res.targetLssHasReallocations
}

// prometheusPerGoroutineRelabelerInputRelabelingOnlyRead wrapper for transparent relabeling
// incoming hashdex(first stage) from cache.
func prometheusPerGoroutineRelabelerInputRelabelingOnlyRead(
	perGoroutineRelabeler, targetLss, hashdex uintptr,
	shardsInnerSeries []InnerSeries,
) (stats RelabelerStats, exception []byte, ok bool) {
	args := struct {
		shardsInnerSeries     []InnerSeries
		perGoroutineRelabeler uintptr
		hashdex               uintptr
		targetLss             uintptr
	}{shardsInnerSeries, perGoroutineRelabeler, hashdex, targetLss}
	var res struct {
		RelabelerStats
		ok        bool
		exception []byte
	}
	start := time.Now()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling_only_read,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	perGoroutineRelabelerInputTransitionRelabelingOnlyReadSum.Add(float64(time.Since(start).Nanoseconds()))
	perGoroutineRelabelerInputTransitionRelabelingOnlyReadCount.Inc()

	return res.RelabelerStats, res.exception, res.ok
}

// prometheusPerGoroutineRelabelerAppendRelabelerSeries wrapper for add relabeled ls to lss,
// add to result and add to cache update(second stage).
func prometheusPerGoroutineRelabelerAppendRelabelerSeries(
	perGoroutineRelabeler, targetLss uintptr,
	shardsInnerSeries []InnerSeries,
	shardsRelabeledSeries []RelabeledSeries,
	shardsRelabelerStateUpdate []RelabelerStateUpdate,
) (exception []byte, targetLssHasReallocations bool) {
	args := struct {
		shardsInnerSeries          []InnerSeries
		shardsRelabeledSeries      []RelabeledSeries
		shardsRelabelerStateUpdate []RelabelerStateUpdate
		perGoroutineRelabeler      uintptr
		targetLss                  uintptr
	}{shardsInnerSeries, shardsRelabeledSeries, shardsRelabelerStateUpdate, perGoroutineRelabeler, targetLss}
	var res struct {
		exception                 []byte
		targetLssHasReallocations bool
	}
	start := time.Now()
	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_goroutine_relabeler_append_relabeler_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	perGoroutineRelabelerAppendRelabelerSeriesSum.Add(float64(time.Since(start).Nanoseconds()))
	perGoroutineRelabelerAppendRelabelerSeriesCount.Inc()

	return res.exception, res.targetLssHasReallocations
}

func prometheusPerGoroutineRelabelerTrackStaleNans(
	innerSeries []InnerSeries,
	staleNansState uintptr,
	defaultTimestamp int64,
) {
	args := struct {
		innerSeries      []InnerSeries
		staleNansState   uintptr
		defaultTimestamp int64
	}{innerSeries, staleNansState, defaultTimestamp}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_goroutine_relabeler_track_stale_nans,
		uintptr(unsafe.Pointer(&args)),
	)
}

func prometheusRemapStaleNansState(staleNansState, lsIdsMapping uintptr) {
	args := struct {
		staleNansState uintptr
		lsIdsMapping   uintptr
	}{staleNansState, lsIdsMapping}

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_remap_stale_nans_state,
		uintptr(unsafe.Pointer(&args)),
	)
}

func prometheusMetricsIteratorCtor() CppMetricsIterator {
	var iterator CppMetricsIterator

	testGC()
	fastcgo.UnsafeCall1(
		C.prompp_metrics_iterator_ctor,
		uintptr(unsafe.Pointer(&iterator)),
	)

	return iterator
}

func prometheusMetricsIteratorNext(iterator *CppMetricsIterator) *CppMetric {
	args := struct {
		iterator uintptr
	}{uintptr(unsafe.Pointer(iterator))}

	var res struct {
		metric *CppMetric
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_metrics_iterator_next,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.metric
}

func prometheusMetricsPageForTestCtor(labels Labels, counterName string, counterValue uint64) uintptr {
	args := struct {
		labels       Labels
		counterName  string
		counterValue uint64
	}{labels, counterName, counterValue}

	var res struct {
		page uintptr
	}

	testGC()
	fastcgo.UnsafeCall2(
		C.prompp_metrics_page_for_test_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.page
}

func prometheusMetricsPageForTestDetach(page uintptr) {
	args := struct {
		page uintptr
	}{page}

	fastcgo.UnsafeCall1(
		C.prompp_metrics_page_for_test_detach,
		uintptr(unsafe.Pointer(&args)),
	)
}

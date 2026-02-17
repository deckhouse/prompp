#define Sizeof_SizeT sizeof(size_t)
#define Sizeof_StdVector 24
#define Sizeof_BareBonesVector 16
#define Sizeof_RoaringBitset 40
#define Sizeof_InnerSeries (Sizeof_SizeT + Sizeof_BareBonesVector + Sizeof_RoaringBitset)
#define Sizeof_GoLabels 16

#define Sizeof_SerializedDataIterator 192

#define Sizeof_MetricsIterator 24

#define Sizeof_SegmentSamplesStorage 80
#define Sizeof_RemoteWriteMessageEncoder 32

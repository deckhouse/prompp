package cppbridge

const (
	PromqlCppThinningFunction                = 1
	PromqlCppSynthesizingFunction            = 2
	PromqlCppCrossSeriesThinningFunction     = 3
	PromqlCppCrossSeriesSynthesizingFunction = 4
)

// GetFlavor returns recognized architecture flavor
//
//revive:disable:confusing-naming // wrapper
func GetFlavor() string {
	return getFlavor()
}

// MemInfo stats from C++ allocator
type MemInfo struct {
	InUse     uint64
	Allocated uint64
	Resident  uint64
}

// GetMemInfo returns current C++ allocator stats
func GetMemInfo() MemInfo {
	return memInfo()
}

// DumpMemoryProfile Dump C++ allocated memory profile to file
func DumpMemoryProfile(filename string) bool {
	return dumpMemoryProfile(filename) == 0
}

func GetPromqlCppFunctions() []PromqlCppFunction {
	return getPromqlCppFunctions()
}

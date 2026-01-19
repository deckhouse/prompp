package model

import (
	"sync"

	"github.com/RoaringBitmap/roaring/v2"
)

// CacheValue value for ls cache.
type CacheValue struct {
	lsID   uint32
	length uint16
}

// CacheWithBitset cache for labels by hash.
type CacheWithBitset struct {
	cache      map[uint64]CacheValue
	bitset     *roaring.Bitmap
	lockCache  sync.RWMutex
	lockBitset sync.RWMutex
}

// NewCacheWithBitset init new *CacheWithBitset.
func NewCacheWithBitset() *CacheWithBitset {
	return &CacheWithBitset{
		cache:      map[uint64]CacheValue{},
		bitset:     roaring.New(),
		lockCache:  sync.RWMutex{},
		lockBitset: sync.RWMutex{},
	}
}

// Load returns the lsID, ls length stored in the map for a hash, if found.
//
//nolint:gocritic // unnamedResult not need
func (c *CacheWithBitset) Load(hash uint64) (uint32, uint16, bool) {
	c.lockCache.RLock()
	v, ok := c.cache[hash]
	c.lockCache.RUnlock()
	if !ok {
		return 0, 0, false
	}

	c.lockBitset.Lock()
	c.bitset.Add(v.lsID)
	c.lockBitset.Unlock()

	return v.lsID, v.length, ok
}

// Reset cache.
func (c *CacheWithBitset) Reset() {
	c.lockCache.Lock()
	c.cache = map[uint64]CacheValue{}
	c.lockCache.Unlock()

	c.lockBitset.Lock()
	c.bitset = roaring.New()
	c.lockBitset.Unlock()
}

// Stats return bitset count and cache size.
func (c *CacheWithBitset) Stats() (cacheSize uint64, bitsetCount uint32) {
	c.lockCache.RLock()
	cacheSize = uint64(len(c.cache))
	c.lockCache.RUnlock()

	c.lockBitset.RLock()
	bitsetCount = uint32(c.bitset.GetCardinality()) // #nosec G115 // no overflow
	c.lockBitset.RUnlock()

	return cacheSize, bitsetCount
}

// StatsWithClearBitset return bitset count and cache size and clear bitset.
func (c *CacheWithBitset) StatsWithClearBitset() (cacheSize uint64, bitsetCount uint32) {
	c.lockCache.RLock()
	cacheSize = uint64(len(c.cache))
	c.lockCache.RUnlock()

	c.lockBitset.Lock()
	bitsetCount = uint32(c.bitset.GetCardinality()) // #nosec G115 // no overflow
	c.bitset.Clear()
	c.lockBitset.Unlock()

	return cacheSize, bitsetCount
}

// Store sets the lsID and length ls for a hash.
func (c *CacheWithBitset) Store(hash uint64, lsID uint32, length uint16) {
	c.lockCache.Lock()

	c.cache[hash] = CacheValue{lsID, length}

	c.lockCache.Unlock()
}

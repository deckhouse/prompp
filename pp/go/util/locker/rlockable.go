package locker

//
// RLockable
//

// RLockable is an interface that can be used to lock and unlock a resource.
type RLockable interface {
	// RLock locks the resource for reading.
	RLock()

	// RUnlock unlocks the resource for reading.
	RUnlock()
}

//
// NoopLocker
//

// NoopLocker is a no-op locker.
type NoopLocker struct{}

// RLock implementation of [RLockable], do nothing.
func (NoopLocker) RLock() {}

// RUnlock implementation of [RLockable], do nothing.
func (NoopLocker) RUnlock() {}

package example

import (
	"sync"
)

// MemoryClaim represents a memory reservation for a unique name
type MemoryClaim struct {
	ClaimName  string `json:"name"`
	MemoryInMb int    `json:"memory_in_mb"`
}

// Name implements Nameable, required for RestApi interface (Payload argument)
func (claim MemoryClaim) Name() string {
	return claim.ClaimName
}

// =========== Fake ugly DB code below ===========
var fakeDbLock = sync.Mutex{}
var fakeDb = map[string]MemoryClaim{}
var AvailableMemoryMb = 2048 // One counter for many datacenters and zones to ensure we get some errors

func FakeDbRead(name string) (MemoryClaim, bool) {
	fakeDbLock.Lock()
	defer fakeDbLock.Unlock()
	claim, ok := fakeDb[name]
	return claim, ok
}

func FakeDbWrite(claim MemoryClaim) {
	fakeDbLock.Lock()
	defer fakeDbLock.Unlock()
	fakeDb[claim.ClaimName] = claim
}

func FakeDbDelete(name string) {
	fakeDbLock.Lock()
	defer fakeDbLock.Unlock()
	delete(fakeDb, name)
}

func FakeDbList() []string {
	fakeDbLock.Lock()
	defer fakeDbLock.Unlock()
	var items []string
	for _, item := range fakeDb {
		items = append(items, item.ClaimName)
	}
	return items
}

func FakeDbGetAvailableMemory() int {
	fakeDbLock.Lock()
	defer fakeDbLock.Unlock()
	return AvailableMemoryMb
}

func FakeDbUpdateAvailableMemory(update int) {
	fakeDbLock.Lock()
	defer fakeDbLock.Unlock()
	AvailableMemoryMb += update
}

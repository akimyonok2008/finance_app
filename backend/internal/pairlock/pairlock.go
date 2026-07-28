// Package pairlock provides an in-process mutex keyed by an unordered pair of
// user IDs. It is used to serialize operations that must never interleave
// for the same pair of users across service boundaries — specifically,
// social.Service.Follow and safety.Service.Block, so that a concurrent
// follow request can never race a concurrent block request and leave both
// an active block and an active follow relationship for the same pair.
//
// This is an in-process guarantee only. The PostgreSQL paths additionally
// take a `pg_advisory_xact_lock` keyed the same way so the invariant holds
// across processes/replicas too.
package pairlock

import "sync"

var (
	mu    sync.Mutex
	locks = map[string]*sync.Mutex{}
)

func key(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// Lock acquires the mutex for the unordered pair (a, b) and returns an
// unlock function. Callers must defer the returned function.
func Lock(a, b string) func() {
	k := key(a, b)
	mu.Lock()
	l, ok := locks[k]
	if !ok {
		l = &sync.Mutex{}
		locks[k] = l
	}
	mu.Unlock()

	l.Lock()
	return l.Unlock
}

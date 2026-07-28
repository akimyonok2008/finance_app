// Package leaderlock provides single-instance leadership for periodic
// background jobs that are not safe to run concurrently across replicas
// (they do a full unpaginated pass over all users with no per-row claiming,
// unlike the outbox and ranked-snapshot workers which already use
// SELECT ... FOR UPDATE SKIP LOCKED / evaluation claims and need no gate).
package leaderlock

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey is an arbitrary, fixed advisory-lock key distinct from the
// migration lock (internal/db/postgres.go) and the per-pair keys hashed by
// internal/pairlock and the block/follow coordinators (those hash two user
// IDs together; this is a single fixed key with no per-entity meaning).
const lockKey int64 = 0x4C454144 // "LEAD"

// Elector holds a session-scoped Postgres advisory lock for as long as this
// process is the leader, so exactly one replica runs the gated jobs at a
// time. It is deliberately simple: try-acquire on a dedicated pooled
// connection, retry on a fixed interval while not leader, and treat a lost
// connection as lost leadership so another replica can take over.
type Elector struct {
	pool   *pgxpool.Pool
	leader atomic.Bool
	conn   *pgxpool.Conn
}

// New wires an Elector around pool. A nil pool is not valid — callers that
// have no Postgres pool (in-memory storage mode, effectively single-instance
// already) should simply not construct an Elector and pass a nil *Elector to
// SetLeaderElector, which IsLeader treats as "always leader".
func New(pool *pgxpool.Pool) *Elector {
	return &Elector{pool: pool}
}

// IsLeader reports whether this process currently holds leadership. Nil-safe:
// a nil *Elector always reports true, so callers that never wire one up (dev,
// memory-mode, or intentionally single-instance deployments) keep running
// every gated job locally without any special-casing.
func (e *Elector) IsLeader() bool {
	if e == nil {
		return true
	}
	return e.leader.Load()
}

// Run attempts to acquire, and once acquired keep holding, leadership until
// ctx is cancelled. Call it in its own goroutine; it never blocks the caller
// and releases the lock on shutdown so another replica can take over
// immediately rather than waiting for this process's connection to time out.
func (e *Elector) Run(ctx context.Context) {
	const retryInterval = 10 * time.Second
	e.tryAcquire(ctx)

	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			e.release()
			return
		case <-ticker.C:
			if !e.leader.Load() {
				e.tryAcquire(ctx)
				continue
			}
			if e.conn == nil || e.conn.Conn().IsClosed() {
				// The held connection died without an explicit release; drop
				// leadership so another replica can win the lock, and try to
				// reacquire it ourselves on the next tick too.
				e.leader.Store(false)
				e.conn = nil
				slog.Warn("leader_election_connection_lost")
			}
		}
	}
}

func (e *Elector) tryAcquire(ctx context.Context) {
	conn, err := e.pool.Acquire(ctx)
	if err != nil {
		slog.Warn("leader_election_acquire_conn_failed", "error", err)
		return
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired); err != nil {
		slog.Warn("leader_election_lock_query_failed", "error", err)
		conn.Release()
		return
	}
	if !acquired {
		conn.Release()
		return
	}
	e.conn = conn
	e.leader.Store(true)
	slog.Info("leader_election_acquired")
}

func (e *Elector) release() {
	if e.conn == nil {
		return
	}
	unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := e.conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, lockKey); err != nil {
		slog.Warn("leader_election_unlock_failed", "error", err)
	}
	e.conn.Release()
	e.conn = nil
	e.leader.Store(false)
	slog.Info("leader_election_released")
}

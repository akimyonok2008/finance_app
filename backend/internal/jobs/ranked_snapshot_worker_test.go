package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ardakimyonok/finance_app/internal/performancehistory"
)

type fakeRankedSnapshotJob struct {
	snapshotCalls   int
	evaluationCalls int
	compactCalls    int
	compactErr      error
}

func (f *fakeRankedSnapshotJob) SnapshotAll(context.Context) performancehistory.BatchResult {
	f.snapshotCalls++
	return performancehistory.BatchResult{UsersProcessed: 3, SnapshotsCreated: 2, Failures: 1}
}
func (f *fakeRankedSnapshotJob) ProcessEvaluations(context.Context) (int, int) {
	f.evaluationCalls++
	return 1, 1
}
func (f *fakeRankedSnapshotJob) Compact(context.Context) (int64, error) {
	f.compactCalls++
	return 0, f.compactErr
}

func TestRankedSnapshotWorkerRunsIndependentStages(t *testing.T) {
	job := &fakeRankedSnapshotJob{compactErr: errors.New("temporary")}
	worker := NewRankedSnapshotWorker(job, time.Hour)
	worker.RunOnce(context.Background())
	assert.Equal(t, 1, job.snapshotCalls)
	assert.Equal(t, 1, job.evaluationCalls)
	assert.Equal(t, 1, job.compactCalls)
}

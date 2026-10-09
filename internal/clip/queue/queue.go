// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package queue provides a simple job queue for processing media jobs.
package queue

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/logging"
)

// JobHandler is a function that handles jobs.
type JobHandler func(ctx context.Context, job *clip.Job) error

// Queue represents a job queue.
//
// The queue owns a copy of every job it is handed. Waiting jobs are held as
// ids in submission order, so queueing never blocks, and a worker renders a
// snapshot of the entry while the entry itself only changes under the lock.
type Queue struct {
	workers  int
	line     []string
	wake     chan struct{}
	jobs     map[string]*clip.Job
	cancels  map[string]context.CancelFunc
	dropped  map[string]struct{}
	waiting  map[string]struct{}
	deleted  map[string]struct{}
	stop     sync.Once
	stopped  bool
	mu       sync.RWMutex
	notifyMu sync.Mutex
	reported *sync.Cond
	inFlight string
	wg       sync.WaitGroup
	handler  JobHandler
	done     chan struct{}
	cancel   context.CancelFunc
	statusFn StatusFunc
}

// StatusFunc is called with a copy of a job whenever its status or progress
// changes.
type StatusFunc func(job *clip.Job)

// outcome is how a job's processing ended.
type outcome int

// stagedEdit is an edit written to a job's entry and waiting on its save.
type stagedEdit struct {
	// entry is the entry the edit wrote to.
	entry *clip.Job
	// before is the entry as it was before the edit.
	before clip.Job
	// queued reports whether the edit holds the job as waiting to render.
	queued bool
}

// editMode is what an edit is for, which decides when a rendering job
// refuses it and when the job has to be queued again.
type editMode int

const (
	// outcomeRecorded means the job's final status was written.
	outcomeRecorded outcome = iota
	// outcomeCanceled means the job was canceled while running.
	outcomeCanceled
	// outcomeDeleted means the job was deleted while running, so nothing is written back.
	outcomeDeleted
)

const (
	// editInPlace changes a job's record, rendering it again only when its
	// type changed.
	editInPlace editMode = iota
	// editForRender changes a job that the caller is about to render again.
	editForRender
)

const (

	// progressDone is the progress value that means a job is finished.
	progressDone = 100
	// progressReset is the progress a re-queued job starts from.
	progressReset = 0

	// canceledMessage is the failure message a canceled job carries. It is a
	// sentence rather than the status literal, because clip.Error is rendered
	// verbatim to the user alongside the reason a render failed.
	canceledMessage = "canceled by the user"
)

// ErrJobActive is returned when a job is submitted while one with the same id is already queued or running.
var ErrJobActive = errors.New("a job with this id is already active")

// ErrQueueStopped is returned when a job is submitted to a stopped queue.
var ErrQueueStopped = errors.New("queue is stopped")

// ErrJobNotFound is returned when an edit names a job the queue does not have.
var ErrJobNotFound = errors.New("job not found")

// ErrJobPanicked is recorded against a job whose handler panicked.
var ErrJobPanicked = errors.New("job handler panicked")

// NewQueue creates a new job queue.
//
// Parameters:
//   - workers: How many jobs may run at once.
//   - handler: Invoked for each job.
//
// Returns:
//   - queue: A queue ready to Start.
func NewQueue(workers int, handler JobHandler) *Queue {
	_, cancel := context.WithCancel(context.Background())
	queue := &Queue{
		stop:     sync.Once{},
		workers:  workers,
		line:     nil,
		wake:     make(chan struct{}, 1),
		jobs:     make(map[string]*clip.Job),
		cancels:  make(map[string]context.CancelFunc),
		dropped:  make(map[string]struct{}),
		waiting:  make(map[string]struct{}),
		deleted:  make(map[string]struct{}),
		mu:       sync.RWMutex{},
		notifyMu: sync.Mutex{},
		reported: nil,
		inFlight: "",
		wg:       sync.WaitGroup{},
		handler:  handler,
		done:     make(chan struct{}),
		cancel:   cancel,
		statusFn: nil,
	}

	queue.reported = sync.NewCond(&queue.mu)

	return queue
}

// Adopt registers a copy of a job unless the queue already holds one, so a
// stored record never replaces an entry another caller has just changed.
//
// Parameters:
//   - job: The clip to track without running.
func (q *Queue) Adopt(job *clip.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, held := q.jobs[job.ID]; held {
		return
	}

	q.jobs[job.ID] = job.Clone()
}

// Cancel stops a pending or processing job.
//
// Parameters:
//   - id: The clip to cancel.
//
// Returns:
//   - canceled: True when a pending or processing job was canceled.
func (q *Queue) Cancel(id string) bool {
	q.mu.Lock()

	job, ok := q.jobs[id]
	if !ok || (job.Status != clip.StatusPending && job.Status != clip.StatusProcessing) {
		q.mu.Unlock()

		return false
	}

	if cancel, running := q.cancels[id]; running {
		// The worker unwinds on its own, and the marker tells it to record a
		// cancellation rather than the error the abort produces.
		cancel()

		q.dropped[id] = struct{}{}
	}

	// A job still waiting simply leaves the line, so nothing is left holding
	// its id and it can be queued again at once.
	q.leaveLine(id)

	job.Status = clip.StatusCancelled
	job.Error = canceledMessage
	job.UpdatedAt = time.Now()
	q.mu.Unlock()

	q.notify(id)

	return true
}

// Delete removes a job from the in-memory map. A waiting job leaves the line,
// and a running one is canceled and tombstoned, so its worker writes nothing
// back. A report of this job already under way finishes first, so it cannot
// write the job back after the caller deletes its row.
//
// Parameters:
//   - id: The clip to remove.
func (q *Queue) Delete(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.inFlight == id {
		q.reported.Wait()
	}

	if _, exists := q.jobs[id]; !exists {
		return
	}

	if cancel, running := q.cancels[id]; running {
		cancel()

		// Marked as a cancellation too, so a worker settling a reinstated job
		// records what the user asked for rather than the abort's error.
		q.dropped[id] = struct{}{}
		q.deleted[id] = struct{}{}
	}

	q.leaveLine(id)
	delete(q.jobs, id)
}

// Done returns a channel that is closed when the queue is stopped.
//
// Returns:
//   - done: Closed on teardown.
func (q *Queue) Done() <-chan struct{} {
	return q.done
}

// Edit applies a change to a job and saves it. A job being rendered only
// accepts a change that leaves its file the same, such as a new name. A
// waiting job is edited in place, so the worker that takes it renders the new
// values. A job whose type changed is queued again in the same step, and only
// once the save succeeds. The save is ordered with the queue's status
// reports, and a failed save leaves the job as it was.
//
// Parameters:
//   - id: The job to change.
//   - edit: The change to apply.
//   - output: Where the job renders to once its type is edit.Type.
//   - save: Writes the edited job. It runs without the queue lock.
//
// Returns:
//   - job: A copy of the edited job.
//   - queued: True when the edit queued the job again, because its type
//     changed and it was not already waiting.
//   - err: ErrJobNotFound, ErrJobActive when a rendering job would render
//     differently, ErrQueueStopped when a job that has to be queued cannot
//     be, or the save's error.
func (q *Queue) Edit(
	id string,
	edit clip.Edit,
	output string,
	save func(*clip.Job) error,
) (*clip.Job, bool, error) {
	job, queued, err := q.applyEdit(id, edit, output, save, editInPlace)
	if err != nil {
		return nil, false, fmt.Errorf("edit %s: %w", id, err)
	}

	return job, queued, nil
}

// EditAndRegenerate applies a change to a job, saves it, and queues the job
// to render again. It refuses a job being rendered, whatever the change, and
// otherwise behaves as Edit.
//
// Parameters:
//   - id: The job to change.
//   - edit: The change to apply.
//   - output: Where the job renders to once its type is edit.Type.
//   - save: Writes the edited job. It runs without the queue lock.
//
// Returns:
//   - job: A copy of the edited job.
//   - queued: True unless the job was already waiting, where the worker that
//     takes it renders the new values anyway.
//   - err: ErrJobNotFound, ErrJobActive when the job is rendering,
//     ErrQueueStopped when the queue has stopped, or the save's error.
func (q *Queue) EditAndRegenerate(
	id string,
	edit clip.Edit,
	output string,
	save func(*clip.Job) error,
) (*clip.Job, bool, error) {
	job, queued, err := q.applyEdit(id, edit, output, save, editForRender)
	if err != nil {
		return nil, false, fmt.Errorf("edit %s for a render: %w", id, err)
	}

	return job, queued, nil
}

// GetAllJobs returns every job, newest first.
//
// Returns:
//   - jobs: Copies of every registered job.
func (q *Queue) GetAllJobs() []*clip.Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*clip.Job, 0, len(q.jobs))
	for _, job := range q.jobs {
		result = append(result, job.Clone())
	}

	slices.SortFunc(result, compareJobsNewestFirst)

	return result
}

// compareJobsNewestFirst orders jobs by CreatedAt descending, then ID descending.
//
// Parameters:
//   - left: The first job to order.
//   - right: The second job to order.
//
// Returns:
//   - order: Negative when left sorts first, positive when right does.
func compareJobsNewestFirst(left, right *clip.Job) int {
	if order := right.CreatedAt.Compare(left.CreatedAt); order != 0 {
		return order
	}

	return cmp.Compare(right.ID, left.ID)
}

// GetJob gets a job by ID, as a copy.
//
// Parameters:
//   - id: The clip to read.
//
// Returns:
//   - job: A copy of the job, nil when the queue does not have it.
func (q *Queue) GetJob(id string) *clip.Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.jobs[id].Clone()
}

// Reinstate puts a job back after a delete that could not be completed. The
// job comes back canceled, so a later start does not resume a render the
// delete stopped. A job a worker still holds is settled by that worker, which
// records the same cancellation.
//
// Parameters:
//   - job: The job as it was before the delete.
func (q *Queue) Reinstate(job *clip.Job) {
	q.mu.Lock()

	entry := job.Clone()

	delete(q.deleted, entry.ID)

	_, running := q.cancels[entry.ID]

	switch {
	case running, entry.Status == clip.StatusPending || entry.Status == clip.StatusProcessing:
		entry.Status = clip.StatusCancelled
		entry.Error = canceledMessage
	default:
		// A settled job keeps the status it had.
	}

	entry.UpdatedAt = time.Now()
	q.jobs[entry.ID] = entry
	q.mu.Unlock()
}

// Requeue puts an idle job back on the queue for another attempt. The job is
// reset to pending in place, so the caller's copy matches the queued one.
//
// Parameters:
//   - job: The clip to run again.
//
// Returns:
//   - err: ErrJobActive when a worker holds the job or it is already waiting,
//     ErrQueueStopped when the queue has stopped.
func (q *Queue) Requeue(job *clip.Job) error {
	q.mu.Lock()

	detail, refused := q.admit(job.ID)
	if refused != nil {
		q.mu.Unlock()

		return fmt.Errorf("%w: %s", refused, detail)
	}

	job.Status = clip.StatusPending
	job.Progress = progressReset
	job.Error = ""
	job.UpdatedAt = time.Now()

	q.enqueue(job)
	q.mu.Unlock()

	q.notify(job.ID)
	q.signal()

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("job requeued")

	return nil
}

// Restore registers a copy of a job without enqueueing it.
//
// Parameters:
//   - job: The clip to track without running.
func (q *Queue) Restore(job *clip.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.jobs[job.ID] = job.Clone()
}

// SetProgress records how far a render has got and reports it.
//
// Parameters:
//   - id: The clip being rendered.
//   - percent: Progress so far.
//
// Returns:
//   - job: The updated job, nil when the queue no longer has it.
func (q *Queue) SetProgress(id string, percent int) *clip.Job {
	q.mu.Lock()

	job, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()

		return nil
	}

	job.Progress = percent
	job.UpdatedAt = time.Now()

	updated := job.Clone()
	q.mu.Unlock()

	q.notify(id)

	return updated
}

// SetStage records that a render moved on to another encode, whose progress
// starts again from zero, and reports it.
//
// Parameters:
//   - id: The clip being rendered.
//   - stage: The encode now running.
//
// Returns:
//   - job: The updated job, nil when the queue no longer has it.
func (q *Queue) SetStage(id string, stage clip.Stage) *clip.Job {
	q.mu.Lock()

	job, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()

		return nil
	}

	job.Stage = stage
	job.Progress = 0
	job.UpdatedAt = time.Now()

	updated := job.Clone()
	q.mu.Unlock()

	q.notify(id)

	return updated
}

// SetStatusFunc registers a callback invoked on job status changes.
//
// Parameters:
//   - fn: The callback, or nil to stop reporting.
func (q *Queue) SetStatusFunc(fn StatusFunc) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.statusFn = fn
}

// Start starts the job queue workers.
//
// Parameters:
//   - ctx: Lifetime for the queue and for every job.
func (q *Queue) Start(ctx context.Context) {
	// Stop cancels this rather than the context it was built from, because this is
	// the one every job's context descends from. Canceling the caller's context
	// instead would leave a running ffmpeg unwinding on nothing.
	ctx, cancel := context.WithCancel(ctx)

	q.mu.Lock()

	q.cancel = cancel
	q.mu.Unlock()

	// Canceling the caller's context stops the queue through the same guarded
	// path Stop uses. Without it a later Submit would be accepted by a queue
	// that can never run it.
	context.AfterFunc(ctx, q.Stop)

	for range q.workers {
		q.wg.Go(func() {
			q.worker(ctx)
		})
	}

	logging.Logger.Info().Int("workers", q.workers).Msg("job queue started")
}

// Stop stops the job queue and waits for its workers.
func (q *Queue) Stop() {
	q.stop.Do(func() {
		q.mu.Lock()

		q.stopped = true
		q.mu.Unlock()

		q.cancel()
		close(q.done)
	})

	q.wg.Wait()
}

// Submit submits a copy of a job to the queue. It never waits on a worker.
//
// Parameters:
//   - job: The clip to queue.
//
// Returns:
//   - err: ErrJobActive when the id is already queued or running,
//     ErrQueueStopped when the queue has stopped.
func (q *Queue) Submit(job *clip.Job) error {
	q.mu.Lock()

	detail, refused := q.admit(job.ID)
	if refused != nil {
		q.mu.Unlock()

		return fmt.Errorf("%w: %s", refused, detail)
	}

	q.enqueue(job)
	q.mu.Unlock()

	q.notify(job.ID)
	q.signal()

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("job submitted")

	return nil
}

// admit reports whether an id may be queued. The caller holds the lock.
//
// Parameters:
//   - id: The clip id to queue.
//
// Returns:
//   - detail: What the refusal is about, for the caller's error.
//   - refused: ErrQueueStopped or ErrJobActive, nil when the id may be queued.
func (q *Queue) admit(id string) (string, error) {
	if q.stopped {
		return id, ErrQueueStopped
	}

	if reason := q.heldReason(id); reason != "" {
		return id + " is " + reason, ErrJobActive
	}

	return "", nil
}

// applyEdit applies a change to a job and saves it, under the rules of mode.
// A job that has to render again is marked pending and held as waiting before
// the save, and joins the line only after the save succeeds, so a worker never
// renders an edit the store refused.
//
// Parameters:
//   - id: The job to change.
//   - change: The change to apply.
//   - output: Where the job renders to once its type is change.Type.
//   - save: Writes the edited job. It runs without the queue lock.
//   - mode: What the edit is for.
//
// Returns:
//   - job: A copy of the edited job.
//   - queued: True when the edit queued the job again.
//   - err: ErrJobNotFound, ErrJobActive, ErrQueueStopped, or the wrapped save
//     error.
func (q *Queue) applyEdit(
	id string,
	change clip.Edit,
	output string,
	save func(*clip.Job) error,
	mode editMode,
) (*clip.Job, bool, error) {
	q.notifyMu.Lock()
	defer q.notifyMu.Unlock()

	q.mu.Lock()

	staged, err := q.stageEdit(id, change, output, mode)
	if err != nil {
		q.mu.Unlock()

		//nolint:wrapcheck // A refusal is this package's own sentinel, which Edit wraps with the id.
		return nil, false, err
	}

	snapshot := staged.entry.Clone()

	var saveErr error

	q.deliver(id, snapshot, func(job *clip.Job) {
		saveErr = save(job)
	})

	if saveErr != nil {
		q.revertEdit(id, &staged)

		return nil, false, fmt.Errorf("save: %w", saveErr)
	}

	if staged.queued {
		q.joinLine(id, staged.entry)
	}

	return snapshot, staged.queued, nil
}

// deliver hands a snapshot of a job to a write with the queue lock released.
// The job is marked in flight meanwhile, so a delete of it waits for the
// write. The caller holds notifyMu and the queue lock, and deliver returns
// with the queue lock released.
//
// Parameters:
//   - id: The job being written.
//   - snapshot: The copy to write.
//   - write: Stores or reports the copy.
func (q *Queue) deliver(id string, snapshot *clip.Job, write func(*clip.Job)) {
	q.inFlight = id
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()

		q.inFlight = ""
		q.reported.Broadcast()
		q.mu.Unlock()
	}()

	write(snapshot)
}

// editPlan decides whether a job may take an edit and whether the edit queues
// it to render. The caller holds the queue lock.
//
// Parameters:
//   - id: The job to change.
//   - edited: The job's record after the edit.
//   - previous: The job's record before it.
//   - mode: What the edit is for.
//
// Returns:
//   - queued: True when the edit has to queue the job.
//   - err: ErrJobActive or ErrQueueStopped when the edit is refused.
func (q *Queue) editPlan(id string, edited, previous *clip.Clip, mode editMode) (bool, error) {
	_, running := q.cancels[id]
	_, waiting := q.waiting[id]

	if running && !mode.allowsWhileRendering(edited, previous) {
		return false, ErrJobActive
	}

	queued := !waiting && (mode == editForRender || edited.Type != previous.Type)
	if queued && q.stopped {
		return false, ErrQueueStopped
	}

	return queued, nil
}

// enqueue stores a copy of a job and puts its id at the back of the line. The
// caller holds the lock.
//
// Parameters:
//   - job: The clip to queue.
func (q *Queue) enqueue(job *clip.Job) {
	q.jobs[job.ID] = job.Clone()
	q.waiting[job.ID] = struct{}{}
	q.line = append(q.line, job.ID)
}

// heldReason reports why an id is already taken, and whether it is.
//
// Parameters:
//   - id: The clip id to check.
//
// Returns:
//   - reason: What has the id, empty when nothing does.
func (q *Queue) heldReason(id string) string {
	if _, running := q.cancels[id]; running {
		return "rendering"
	}

	if _, waiting := q.waiting[id]; waiting {
		return "already queued"
	}

	return ""
}

// joinLine puts an edited job held as waiting at the back of the line, unless
// a cancel or a delete took it out while its edit was being saved.
//
// Parameters:
//   - id: The job to queue.
//   - entry: The entry the edit wrote to.
func (q *Queue) joinLine(id string, entry *clip.Job) {
	q.mu.Lock()

	_, waiting := q.waiting[id]
	held := waiting && q.jobs[id] == entry

	if held {
		q.line = append(q.line, id)
	}

	q.mu.Unlock()

	if held {
		q.signal()

		logging.Logger.Info().Str("job_id", id).Msg("job requeued by an edit")
	}
}

// leaveLine takes a waiting id out of the line. The caller holds the lock.
//
// Parameters:
//   - id: The clip id to remove.
func (q *Queue) leaveLine(id string) {
	if _, waiting := q.waiting[id]; !waiting {
		return
	}

	delete(q.waiting, id)

	q.line = slices.DeleteFunc(q.line, func(queued string) bool {
		return queued == id
	})
}

// notify reports a job's current state to the status callback. Reports are
// made one at a time and each reads the entry as it is then, so the last one
// to land always carries the latest state. The callback runs without the
// queue lock, and the job it reports is marked in flight, so only a delete of
// that job waits for it.
//
// Parameters:
//   - id: The job whose status or progress changed.
func (q *Queue) notify(id string) {
	q.notifyMu.Lock()
	defer q.notifyMu.Unlock()

	q.mu.Lock()

	job, ok := q.jobs[id]
	if !ok || q.statusFn == nil {
		q.mu.Unlock()

		return
	}

	q.deliver(id, job.Clone(), q.statusFn)
}

// processJob runs the next waiting job, if there is one.
//
// Parameters:
//   - ctx: The worker's context, which a single job can be canceled from.
//
// Returns:
//   - ran: False when nothing was waiting.
func (q *Queue) processJob(ctx context.Context) bool {
	// The job's own context, so a single job can be canceled without
	// disturbing the rest of the queue.
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	job, ok := q.take(cancel)
	if !ok {
		return false
	}

	q.notify(job.ID)

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("processing job")

	err := q.runHandler(jobCtx, job)

	result := q.settle(job.ID, err)

	switch {
	case result == outcomeDeleted:
		logging.Logger.Info().Str("job_id", job.ID).Msg("job deleted while processing")
	case result == outcomeCanceled:
		logging.Logger.Info().Str("job_id", job.ID).Msg("job canceled")
	case err != nil:
		logging.Logger.Error().
			Str("job_id", job.ID).
			Err(err).
			Msg("job failed")
	default:
		logging.Logger.Info().
			Str("job_id", job.ID).
			Msg("job completed")
	}

	if result != outcomeDeleted {
		q.notify(job.ID)
	}

	return true
}

// revertEdit undoes an edit whose save failed. A job the edit queued is put
// back whole, unless a cancel took it out of the line meanwhile, in which case
// only the record goes back and the cancel stands. An entry a delete or a new
// submit replaced is left alone.
//
// Parameters:
//   - id: The job that was edited.
//   - staged: The edit to undo.
func (q *Queue) revertEdit(id string, staged *stagedEdit) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.jobs[id] != staged.entry {
		return
	}

	if _, waiting := q.waiting[id]; staged.queued && waiting {
		delete(q.waiting, id)

		*staged.entry = staged.before

		return
	}

	staged.entry.Clip = staged.before.Clip
	staged.entry.OutputPath = staged.before.OutputPath
}

// runHandler invokes the job handler, turning a panic into an error.
//
// Parameters:
//   - ctx: Cancellation for the job.
//   - job: The job being handled.
//
// Returns:
//   - err: The handler's error, or ErrJobPanicked if it panicked.
//
//nolint:nonamedreturns // a deferred recover writes the result through the name.
func (q *Queue) runHandler(ctx context.Context, job *clip.Job) (err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		// The stack is captured here rather than through zerolog's Stack, which
		// only renders when an error is attached and this event carries none.
		logging.Logger.Error().
			Str("job_id", job.ID).
			Str("type", string(job.Type)).
			Interface("panic", recovered).
			Str("stack", string(debug.Stack())).
			Msg("job handler panicked")

		err = ErrJobPanicked
	}()

	// The handler is the queue's own contract with its caller, so its error is
	// passed through as it is.
	//nolint:wrapcheck // the handler's error belongs to the caller that supplied it.
	return q.handler(ctx, job)
}

// settle records how a job ended on the queue's entry and releases its
// bookkeeping.
//
// Parameters:
//   - id: The job that was processed.
//   - err: What the handler returned.
//
// Returns:
//   - outcome: How the job ended.
func (q *Queue) settle(id string, err error) outcome {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, canceled := q.dropped[id]
	_, gone := q.deleted[id]
	delete(q.dropped, id)
	delete(q.deleted, id)
	delete(q.cancels, id)

	live, tracked := q.jobs[id]
	if gone || !tracked {
		// The row was deleted while this was running, so there is no job left
		// to record the result against.
		return outcomeDeleted
	}

	switch {
	case q.stopped && !canceled && err != nil:
		// The queue is shutting down, so the job was interrupted rather than
		// broken. It goes back to pending with no error, which is what restoreJobs
		// looks for, so the render is picked up again on the next start.
		live.Status = clip.StatusPending
		live.Error = ""
	case canceled:
		live.Status = clip.StatusCancelled
		live.Error = canceledMessage
	case err != nil:
		live.Status = clip.StatusFailed
		live.Error = err.Error()
	default:
		live.Status = clip.StatusCompleted
		// Cleared, so a completed job never carries an earlier failure.
		live.Error = ""
		live.Progress = progressDone
	}

	live.Stage = clip.StageClip
	live.UpdatedAt = time.Now()

	if canceled {
		return outcomeCanceled
	}

	return outcomeRecorded
}

// signal wakes an idle worker, if one is waiting for work.
func (q *Queue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// stageEdit applies an edit to a job's entry, holding the job as waiting when
// the edit queues it. The caller holds the queue lock.
//
// Parameters:
//   - id: The job to change.
//   - change: The change to apply.
//   - output: Where the job renders to once its type is change.Type.
//   - mode: What the edit is for.
//
// Returns:
//   - staged: The entry, how it was before, and whether it was queued.
//   - err: ErrJobNotFound, ErrJobActive, or ErrQueueStopped.
func (q *Queue) stageEdit(
	id string,
	change clip.Edit,
	output string,
	mode editMode,
) (stagedEdit, error) {
	entry, ok := q.jobs[id]
	if !ok {
		return stagedEdit{}, ErrJobNotFound
	}

	edited := entry.Clip
	edited.Apply(change)

	queued, err := q.editPlan(id, &edited, &entry.Clip, mode)
	if err != nil {
		//nolint:wrapcheck // A refusal is this package's own sentinel, which Edit wraps with the id.
		return stagedEdit{}, err
	}

	staged := stagedEdit{entry: entry, before: *entry, queued: queued}
	staged.write(edited, output)

	if queued {
		q.waiting[id] = struct{}{}
	}

	return staged, nil
}

// take hands the next waiting job to a worker. The entry is marked processing
// and registered as running under one lock, so its id is never unaccounted
// for.
//
// Parameters:
//   - cancel: Stops the job's context, called when the job is canceled.
//
// Returns:
//   - job: A snapshot of the entry for the handler to render.
//   - ok: False when the queue is stopped or nothing is waiting.
func (q *Queue) take(cancel context.CancelFunc) (*clip.Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.stopped || len(q.line) == 0 {
		return nil, false
	}

	id := q.line[0]

	q.line = q.line[1:]
	delete(q.waiting, id)

	// Another worker may be idle while more jobs wait, and the single wake
	// slot only reached this one.
	if len(q.line) > 0 {
		q.signal()
	}

	live := q.jobs[id]

	live.Status = clip.StatusProcessing
	live.Stage = clip.StageClip
	live.UpdatedAt = time.Now()
	q.cancels[id] = cancel

	return live.Clone(), true
}

// worker runs waiting jobs until the queue stops.
//
// Parameters:
//   - ctx: The worker lifetime, canceled when the queue stops.
func (q *Queue) worker(ctx context.Context) {
	for {
		if q.processJob(ctx) {
			continue
		}

		select {
		case <-q.done:
			return
		case <-q.wake:
		}
	}
}

// allowsWhileRendering reports whether a job being rendered may take an edit.
// Only a change that leaves the file the same is allowed, and never when the
// caller is about to render the job again.
//
// Parameters:
//   - edited: The job's record after the edit.
//   - previous: The job's record before it.
//
// Returns:
//   - allowed: True when the edit may be applied.
func (mode editMode) allowsWhileRendering(edited, previous *clip.Clip) bool {
	return mode == editInPlace && edited.RendersLike(previous)
}

// write applies the edited record to the entry, and marks the entry pending
// when the edit queues it.
//
// Parameters:
//   - edited: The job's record after the edit.
//   - output: Where the job renders to once its type is edited.Type.
func (staged *stagedEdit) write(edited clip.Clip, output string) {
	entry := staged.entry
	typeChanged := edited.Type != entry.Type

	entry.Clip = edited

	if typeChanged || staged.queued {
		entry.OutputPath = output
	}

	if staged.queued {
		entry.Status = clip.StatusPending
		entry.Progress = progressReset
		entry.Error = ""
	}
}

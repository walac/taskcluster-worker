package worker

import (
	"fmt"
	"strconv"

	"github.com/Sirupsen/logrus"
	tcqueue "github.com/taskcluster/taskcluster-client-go/queue"
	"github.com/taskcluster/taskcluster-client-go/tcclient"
	"github.com/taskcluster/taskcluster-worker/runtime"
)

// NOTE (garndt): This is still up in the air as I'm not sure if I like calling the
// update methods like this.  It is nice to just pass in an update object and the method
// takes care of calling it in a goroutine.

type TaskStatusUpdate struct {
	Task          *TaskRun
	Status        runtime.TaskStatus
	IfStatusIn    map[runtime.TaskStatus]bool
	Reason        string
	WorkerId      string
	ProvisionerId string
	WorkerGroup   string
}

type updateError struct {
	statusCode int
	err        string
}

func (e updateError) Error() string {
	return e.err
}

func UpdateTaskStatus(ts TaskStatusUpdate, queue queueClient, log *logrus.Entry) (err <-chan *updateError) {
	e := make(chan *updateError)

	logger := log.WithFields(logrus.Fields{
		"taskId": ts.Task.TaskId,
		"runId":  ts.Task.RunId,
	})

	// we'll make all these functions internal to TaskStatusHandler so that
	// they can only be called inside here, so that reading/writing to the
	// appropriate channels is the only way to trigger them, to ensure
	// proper concurrency handling

	reportException := func(task *TaskRun, reason string, log *logrus.Entry) *updateError {
		ter := tcqueue.TaskExceptionRequest{Reason: reason}
		tsr, _, err := queue.ReportException(task.TaskId, strconv.FormatInt(int64(task.RunId), 10), &ter)
		if err != nil {
			log.WithField("error", err).Warn("Not able to report exception for task")
			return &updateError{err: err.Error()}
		}
		task.TaskClaim.Status = tsr.Status
		return nil
	}

	reportFailed := func(task *TaskRun, log *logrus.Entry) *updateError {
		tsr, _, err := queue.ReportFailed(task.TaskId, strconv.FormatInt(int64(task.RunId), 10))
		if err != nil {
			log.WithField("error", err).Warn("Not able to report failed completion for task.")
			return &updateError{err: err.Error()}
		}
		task.TaskClaim.Status = tsr.Status
		return nil
	}

	reportCompleted := func(task *TaskRun, log *logrus.Entry) *updateError {
		tsr, _, err := queue.ReportCompleted(task.TaskId, strconv.FormatInt(int64(task.RunId), 10))
		if err != nil {
			log.WithField("error", err).Warn("Not able to report successful completion for task.")
			return &updateError{err: err.Error()}
		}
		task.TaskClaim.Status = tsr.Status
		return nil
	}

	claim := func(task *TaskRun, log *logrus.Entry) *updateError {
		log.Info("Claiming task")
		cr := tcqueue.TaskClaimRequest{
			WorkerGroup: ts.WorkerGroup,
			WorkerId:    ts.WorkerId,
		}
		// Using the taskId and runId from the <MessageText> tag, the worker
		// must call queue.claimTask().
		tcrsp, callSummary, err := queue.ClaimTask(task.TaskId, strconv.FormatInt(int64(task.RunId), 10), &cr)
		// check if an error occurred...
		if err != nil {
			e := &updateError{
				err:        err.Error(),
				statusCode: callSummary.HttpResponse.StatusCode,
			}
			var errorMessage string
			switch {
			case callSummary.HttpResponse.StatusCode == 401 || callSummary.HttpResponse.StatusCode == 403:
				errorMessage = "Not authorized to claim task."
			case callSummary.HttpResponse.StatusCode >= 500:
				errorMessage = "Server error when attempting to claim task."
			default:
				errorMessage = "Received an error with a status code other than 401/403/500."
			}
			log.WithFields(logrus.Fields{
				"error":      err,
				"statusCode": callSummary.HttpResponse.StatusCode,
			}).Error(errorMessage)
			return e
		}

		queue := tcqueue.New(
			&tcclient.Credentials{
				ClientId:    tcrsp.Credentials.ClientId,
				AccessToken: tcrsp.Credentials.AccessToken,
				Certificate: tcrsp.Credentials.Certificate,
			},
		)

		task.TaskClaim = *tcrsp
		task.QueueClient = queue

		return nil
	}

	reclaim := func(task *TaskRun, log *logrus.Entry) *updateError {
		log.Info("Reclaiming task")
		tcrsp, _, err := queue.ReclaimTask(task.TaskId, fmt.Sprintf("%d", task.RunId))

		// check if an error occurred...
		if err != nil {
			return &updateError{err: err.Error()}
		}

		task.TaskReclaim = *tcrsp
		log.Info("Reclaimed task successfully")
		return nil
	}

	go func() {
		// only update if either IfStatusIn is nil
		// or it is non-nil but it has "true" value
		// for key of current status
		if ts.IfStatusIn == nil || ts.IfStatusIn[ts.Status] {
			task := ts.Task
			switch ts.Status {
			// Aborting is when you stop running a job you already claimed
			case runtime.Succeeded:
				e <- reportCompleted(task, logger)
			case runtime.Failed:
				e <- reportFailed(task, logger)
			case runtime.Errored:
				e <- reportException(task, ts.Reason, logger)
			case runtime.Claimed:
				e <- claim(task, logger)
			case runtime.Reclaimed:
				e <- reclaim(task, logger)
			default:
				err := &updateError{err: fmt.Sprintf("Internal error: unknown task status: %v", ts.Status)}
				logger.Error(err)
				e <- err
			}
		} else {
			// current status is such that we shouldn't update to new
			// status, so just report that no error occurred...
			e <- nil
		}
	}()
	return e
}

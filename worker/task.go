package worker

import (
	"github.com/taskcluster/taskcluster-client-go/queue"
	"github.com/taskcluster/taskcluster-worker/runtime"
)

type TaskRun struct {
	TaskId          string                       `json:"taskId"`
	RunId           uint                         `json:"runId"`
	SignedDeleteUrl string                       `json:"-"`
	TaskClaim       queue.TaskClaimResponse      `json:"-"`
	TaskReclaim     queue.TaskReclaimResponse    `json:"-"`
	Definition      queue.TaskDefinitionResponse `json:"-"`
	controller      *runtime.TaskContextController
	context         *runtime.TaskContext
}

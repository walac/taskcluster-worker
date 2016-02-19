package worker

import "github.com/taskcluster/taskcluster-client-go/queue"

type TaskRun struct {
	TaskId          string                       `json:"taskId"`
	RunId           uint                         `json:"runId"`
	SignedDeleteUrl string                       `json:"-"`
	TaskClaim       queue.TaskClaimResponse      `json:"-"`
	TaskReclaim     queue.TaskReclaimResponse    `json:"-"`
	Definition      queue.TaskDefinitionResponse `json:"-"`
}

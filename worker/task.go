package worker

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/taskcluster/taskcluster-client-go/queue"
	"github.com/taskcluster/taskcluster-worker/engines"
	"github.com/taskcluster/taskcluster-worker/plugins"
	"github.com/taskcluster/taskcluster-worker/runtime"
)

type TaskRun struct {
	TaskId          string                       `json:"taskId"`
	RunId           uint                         `json:"runId"`
	SignedDeleteUrl string                       `json:"-"`
	TaskClaim       queue.TaskClaimResponse      `json:"-"`
	TaskReclaim     queue.TaskReclaimResponse    `json:"-"`
	Definition      queue.TaskDefinitionResponse `json:"-"`
	QueueClient     queueClient
	plugin          plugins.TaskPlugin
	log             *logrus.Entry
	payload         interface{}
	sync.RWMutex
	context        *runtime.TaskContext
	controller     *runtime.TaskContextController
	sandboxBuilder engines.SandboxBuilder
	sandbox        engines.Sandbox
	resultSet      engines.ResultSet
}

// Abort will set the status of the task to aborted and abort the task execution
// environment if one has been created.
func (t *TaskRun) Abort() {
	t.Lock()
	defer t.Unlock()
	if t.context != nil {
		t.context.Abort()
	}
	if t.sandbox != nil {
		t.sandbox.Abort()
	}
}

// Cancel will set the status of the task to cancelled and abort the task execution
// environment if one has been created.
func (t *TaskRun) Cancel() {
	t.log.Info("Cancelling task")
	t.Lock()
	defer t.Unlock()
	if t.context != nil {
		t.context.Cancel()
	}
	if t.sandbox != nil {
		t.sandbox.Abort()
	}
}

// ExceptionStage will report a task run as an exception with an appropriate reason.
// Tasks that have been cancelled will not be reported as an exception as the run
// has already been resolved.
func (t *TaskRun) ExceptionStage(status runtime.TaskStatus, taskError error) {
	var reason runtime.ExceptionReason
	switch taskError.(type) {
	case engines.MalformedPayloadError:
		reason = runtime.MalformedPayload
	case engines.InternalError:
		reason = runtime.InternalError
	default:
		reason = runtime.WorkerShutdown
	}

	err := t.plugin.Exception(reason)
	if err != nil {
		t.log.WithField("error", err.Error()).Warn("Could not finalize task plugins as exception.")
	}

	if t.context.IsCancelled() {
		return
	}

	e := reportException(t.QueueClient, t, reason, t.log)
	if e != nil {
		t.log.WithField("error", e.Error()).Warn("Could not resolve task as exception.")
	}

	return
}

// Run is the entrypoint to executing a task run.  The task payload will be parsed,
// plugins created, and the task will run through each of the stages of the task
// life cycle.
//
// Tasks that do not complete successfully will be reported as an exception during
// the ExceptionStage.
func (t *TaskRun) Run(pluginManager plugins.Plugin, engine engines.Engine, context *runtime.TaskContext, contextController *runtime.TaskContextController) {
	t.Lock()
	t.context = context
	t.controller = contextController
	t.Unlock()

	defer t.DisposeStage()

	jsonPayload := map[string]json.RawMessage{}
	if err := json.Unmarshal(t.Definition.Payload, &jsonPayload); err != nil {
		t.context.LogError(fmt.Sprintf("Invalid task payload. %s", err))
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	p, err := engine.PayloadSchema().Parse(jsonPayload)
	if err != nil {
		t.context.LogError(fmt.Sprintf("Invalid task payload. %s", err))
		t.ExceptionStage(runtime.Errored, err)
		return
	}
	t.payload = p

	ps, err := pluginManager.PayloadSchema()
	if err != nil {
		t.context.LogError(fmt.Sprintf("Invalid task payload. %s", err))
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	pluginPayload, err := ps.Parse(jsonPayload)
	if err != nil {
		t.context.LogError(fmt.Sprintf("Invalid task payload. %s", err))
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	popts := plugins.TaskPluginOptions{TaskInfo: &runtime.TaskInfo{}, Payload: pluginPayload}
	t.plugin, err = pluginManager.NewTaskPlugin(popts)
	if err != nil {
		t.context.LogError(fmt.Sprintf("Invalid task payload. %s", err))
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	err = t.PrepareStage(engine)
	// TODO (garndt): Add IsAborted() to task context and check here for that
	if err != nil || t.context.IsAborted() || t.context.IsCancelled() {
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	err = t.BuildStage()
	if err != nil || t.context.IsAborted() || t.context.IsCancelled() {
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	err = t.StartStage()
	if err != nil || t.context.IsAborted() || t.context.IsCancelled() {
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	err = t.StopStage()
	if err != nil || t.context.IsAborted() || t.context.IsCancelled() {
		t.ExceptionStage(runtime.Errored, err)
		return
	}

	err = t.FinishStage()
	if err != nil || t.context.IsAborted() || t.context.IsCancelled() {
		t.ExceptionStage(runtime.Errored, err)
		return
	}
}

// DisposeStage is responsible for cleaning up resources allocated for the task execution.
// This will involve closing all necessary files and disposing of contexts, plugins, and sandboxes.
func (t *TaskRun) DisposeStage() {
	err := t.controller.CloseLog()
	if err != nil {
		t.log.WithField("error", err.Error()).Warn("Could not properly close task log")
	}
	err = t.controller.Dispose()
	if err != nil {
		t.log.WithField("error", err.Error()).Warn("Could not dispose of task context")
	}
	return
}

// PrepareStage is the first stage of the task life cycle where task plugins are prepared
// and a sandboxbuilder is created.
func (t *TaskRun) PrepareStage(engine engines.Engine) error {
	t.log.Debug("Preparing task run")

	err := t.plugin.Prepare(t.context)
	if err != nil {
		t.context.LogError(fmt.Sprintf("Could not prepare task plugins. %s", err))
		return err
	}

	t.Lock()
	t.sandboxBuilder, err = engine.NewSandboxBuilder(engines.SandboxOptions{
		TaskContext: t.context,
		Payload:     t.payload,
	})
	t.Unlock()
	if err != nil {
		t.context.LogError(fmt.Sprintf("Could not create task execution environment. %s", err))
		return err
	}

	return nil
}

// BuildStage is the second stage of the task life cycle.  This stage is responsible for
// configuring the environment for building a sandbox (task execution environment).
func (t *TaskRun) BuildStage() error {
	t.log.Debug("Building task run")

	err := t.plugin.BuildSandbox(t.sandboxBuilder)
	if err != nil {
		t.context.LogError(fmt.Sprintf("Could not create task execution environment. %s", err))
		return err
	}

	return nil
}

// StartStage is the third stage of the task life cycle.  This stage is responsible for
// starting the execution environment and waiting for a result.
func (t *TaskRun) StartStage() error {
	t.log.Debug("Running task")

	sandbox, err := t.sandboxBuilder.StartSandbox()
	if err != nil {
		t.context.LogError(fmt.Sprintf("Could not start task execution environment. %s", err))
		return err
	}
	t.Lock()
	t.sandbox = sandbox
	t.Unlock()

	err = t.plugin.Started(t.sandbox)
	if err != nil {
		t.context.LogError(fmt.Sprintf("Could not start task execution environment. %s", err))
		return err
	}

	result, err := t.sandbox.WaitForResult()
	if err != nil {
		t.context.LogError(fmt.Sprintf("Task execution did not complete successfully. %s", err))
		return err
	}

	t.resultSet = result

	return nil
}

func (t *TaskRun) StopStage() error {
	t.log.Debug("Stopping task execution")
	success, err := t.plugin.Stopped(t.resultSet)
	// if stopping the plugins was unsuccessful, the task execution should be a failure
	if success == false {
		t.resultSet.SetResult(false)
	}
	if err != nil {
		t.context.LogError(fmt.Sprintf("Stopping execution environment failed. %s", err))
		return err
	}

	return nil
}

func (t *TaskRun) FinishStage() error {
	t.log.Debug("Finishing task run")

	err := t.plugin.Finished(t.resultSet.Success())
	if err != nil {
		t.context.LogError(fmt.Sprintf("Could not finish cleaning up task execution. %s", err))
		return err
	}

	return nil
}

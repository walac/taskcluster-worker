package worker

import (
	"errors"
	"strings"

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
	controller      *runtime.TaskContextController
	context         *runtime.TaskContext
	plugin          plugins.TaskPlugin
	log             *logrus.Entry
	payload         interface{}
}

func (t *TaskRun) Prepare(engine engines.Engine) (<-chan engines.SandboxBuilder, <-chan error) {
	out := make(chan engines.SandboxBuilder, 1)
	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		t.log.Debug("Preparing task run")
		err := t.plugin.Prepare(t.context)

		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not prepare task plugins")
			errc <- err
			close(out)
			return
		}

		sandboxBuilder, err := engine.NewSandboxBuilder(engines.SandboxOptions{
			TaskContext: t.context,
			Payload:     t.payload,
		})
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not create sandbox builder")
			errc <- err
			close(out)
			return
		}

		out <- sandboxBuilder

	}()
	return out, errc
}

func (t *TaskRun) Build(in <-chan engines.SandboxBuilder, done <-chan struct{}, inerrc <-chan error) (<-chan engines.SandboxBuilder, <-chan error) {
	out := make(chan engines.SandboxBuilder)
	errc := make(chan error, 1)

	go func() {
		defer close(errc)
		sb := <-in
		if sb == nil {
			errc <- <-inerrc
			close(out)
			return
		}
		t.log.Debug("Building task run")
		err := t.plugin.BuildSandbox(sb)
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not build build sandbox")
			errc <- err
			close(out)
			return
		}
		out <- sb
	}()
	return out, errc
}

func (t *TaskRun) Run(in <-chan engines.SandboxBuilder, done <-chan struct{}, inerrc <-chan error) (<-chan engines.ResultSet, <-chan error) {
	out := make(chan engines.ResultSet)
	errc := make(chan error, 1)

	go func() {
		defer func() {
			close(errc)
			close(out)
		}()
		sb := <-in
		if sb == nil {
			errc <- <-inerrc
			close(out)
			return
		}
		t.log.Debug("Running task")
		sandbox, err := sb.StartSandbox()
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not start sandbox")
			errc <- err
			close(out)
			return
		}

		err = t.plugin.Started(sandbox)
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not properly start sandbox")
			errc <- err
			close(out)
			return
		}

		finished := make(chan struct{})
		go func() {
			select {
			case <-done:
				sandbox.Abort()
			case <-finished:
				return
			}
		}()

		result, err := sandbox.WaitForResult()
		close(finished)
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Error when waiting for result set from sandbox")
			errc <- err
		}
		out <- result
	}()
	return out, errc
}

func (t *TaskRun) Stop(in <-chan engines.ResultSet, done <-chan struct{}, inerrc <-chan error) (<-chan engines.ResultSet, <-chan error) {
	out := make(chan engines.ResultSet, 1)
	errc := make(chan error, 1)

	go func() {
		defer func() {
			close(errc)
			close(out)
		}()
		r := <-in
		t.log.Debug("Stopping task run")
		success, err := t.plugin.Stopped(r)
		// if stopping the plugins was unsuccessful, the task execution should be a failure
		if success == false {
			r.SetResult(false)
		}
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not properly stop sandbox")
			errc <- err
		}
		out <- r
	}()

	return out, errc
}

func (t *TaskRun) Finish(in <-chan engines.ResultSet, done <-chan struct{}, inerrc <-chan error) (<-chan engines.ResultSet, <-chan error) {
	out := make(chan engines.ResultSet, 1)
	errc := make(chan error, 1)

	go func() {
		defer func() {
			close(out)
			close(errc)
		}()
		errs := []string{}
		r := <-in
		if r.Success() == false {
			e := <-inerrc
			if e != nil {
				errs = append(errs, e.Error())
			}
		}
		t.log.Debug("Finishing task run")
		err := t.plugin.Finished(r.Success())
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not finish plugin cleanup")
			errs = append(errs, err.Error())
		}

		err = t.plugin.Dispose()
		if err != nil {
			t.log.WithField("error", err.Error()).Warn("Could not dispose plugins")
			errs = append(errs, err.Error())
		}

		if len(errs) > 0 {
			r.SetResult(false)
			errc <- errors.New(strings.Join(errs, ", "))
		}

		out <- r
	}()

	return out, errc
}

package worker

import (
	"github.com/Sirupsen/logrus"
	"github.com/taskcluster/taskcluster-worker/config"
	"github.com/taskcluster/taskcluster-worker/engines"
	"github.com/taskcluster/taskcluster-worker/runtime"
)

// Worker is the center of taskcluster-worker and is responsible for managing resources, tasks,
// and host level events.
type Worker struct {
	logger *logrus.Entry
	tm     *Manager
	stop   chan struct{}
	done   chan struct{}
}

// New will create a worker and task manager.
func New(config *config.Config, engine engines.Engine, environment *runtime.Environment, log *logrus.Entry) (*Worker, error) {
	tm, err := newTaskManager(config, engine, environment, log)
	if err != nil {
		return nil, err
	}

	return &Worker{
		logger: log,
		tm:     tm,
	}, nil
}

// Start will begin the worker cycle of claiming and executing tasks.  The worker
// will also being to respond to host level events such as shutdown notifications and
// resource depletion events.
func (w *Worker) Start() (<-chan struct{}, error) {
	w.logger.Info("worker starting up")
	w.done = make(chan struct{})
	w.stop = make(chan struct{})
	go w.run()
	return w.done, nil
}

func (w *Worker) run() {
	// TODO (garndt): Create a shutdown manager that will send a notification
	// to this channel when either the billing cycle is up, or shutdown notification received
	sc := make(chan struct{})

	go w.tm.Start(w.stop, w.done)

	for {
		select {
		case <-sc:
			w.logger.Info("Shutdown notification received")
			w.Stop()
		case <-w.done:
			w.logger.Info("Worker shutting down.")
			return
		}
	}
}

// Stop will attempt to perform a graceful shutdown of the worker.  Tasks that are running
// will receive a signal to abort and cleaned up properly.
func (w *Worker) Stop() {
	w.stop <- struct{}{}
}

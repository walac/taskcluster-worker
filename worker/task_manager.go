package worker

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	tcqueue "github.com/taskcluster/taskcluster-client-go/queue"
	"github.com/taskcluster/taskcluster-client-go/tcclient"
	"github.com/taskcluster/taskcluster-worker/config"
	"github.com/taskcluster/taskcluster-worker/engines"
	"github.com/taskcluster/taskcluster-worker/runtime"
)

type task struct {
	taskId  string
	runId   uint
	claim   tcqueue.TaskClaimResponse
	context *runtime.TaskContextController
}

// Manager is resonsible for managing the entire task lifecyle from claiming the
// task, creating a sandbox environment, and reporting the results fo the execution.
// The manager will also be responsible for ensuring tasks do not run past their max run
// time and are aborted if a cancellation message is received.
type Manager struct {
	sync.Mutex
	Tasks         []*runtime.TaskContextController
	stop          chan bool
	maxCapacity   int
	engine        *engines.Engine
	environment   *runtime.Environment
	log           *logrus.Entry
	queue         *queueService
	provisionerId string
	workerGroup   string
	workerId      string
}

// Start the task manager and begin executing tasks.
func (m *Manager) Start() {
	m.stop = make(chan bool)
	var wg sync.WaitGroup
	wg.Add(1)
	go m.poll(m.stop, wg)
	wg.Wait()
}

func (m *Manager) Stop() {
	close(m.stop)
}

func (m *Manager) poll(stop <-chan bool, wg sync.WaitGroup) {
	// TODO (garndt): make interval configurable
	doWork := time.NewTicker(time.Second * 5).C
	for {
		select {
		case <-doWork:
			n := math.Max(float64(m.maxCapacity-len(m.Tasks)), 0)
			m.claimWork(int(n))
		case <-stop:
			wg.Done()
			break
		}
	}
}

func (m *Manager) claimWork(ntasks int) {
	if ntasks == 0 {
		return
	}

	claims := m.queue.ClaimWork(ntasks)
	for _, c := range claims {
		ctx, ctrlr, error := runtime.NewTaskContext(m.environment.TemporaryStorage.NewFilePath())
	}

	fmt.Println(claims)
}

// Create a new instance of the task manager that will be responsible for claiming,
// executing, and resolving units of work (tasks).
func New(config *config.Config, engine *engines.Engine, environment *runtime.Environment, log *logrus.Entry) *Manager {
	queue := tcqueue.New(
		&tcclient.Credentials{
			ClientId:    config.Credentials.ClientId,
			AccessToken: config.Credentials.AccessToken,
			Certificate: config.Credentials.Certificate,
		},
	)
	service := &queueService{
		client:           queue,
		ProvisionerId:    config.ProvisionerId,
		WorkerGroup:      config.WorkerGroup,
		Log:              log.WithField("component", "Queue Service"),
		ExpirationOffset: config.QueueService.ExpirationOffset,
	}

	return &Manager{
		engine:        engine,
		environment:   environment,
		log:           log,
		maxCapacity:   config.Capacity,
		queue:         service,
		provisionerId: config.ProvisionerId,
		workerGroup:   config.WorkerGroup,
		workerId:      config.WorkerId,
	}
}

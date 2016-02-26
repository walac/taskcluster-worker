package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/taskcluster/slugid-go/slugid"
	"github.com/taskcluster/taskcluster-client-go/queue"
	"github.com/taskcluster/taskcluster-worker/config"
	"github.com/taskcluster/taskcluster-worker/engines/extpoints"
	_ "github.com/taskcluster/taskcluster-worker/plugins/success"
	"github.com/taskcluster/taskcluster-worker/runtime"
)

type MockQueueService struct {
	tasks []*TaskRun
}

func (q *MockQueueService) ClaimWork(ntasks int) []*TaskRun {
	return q.tasks
}

func TestRunTask(t *testing.T) {
	logger, _ := runtime.CreateLogger(os.Getenv("LOGGING_LEVEL"))
	tempPath := filepath.Join(os.TempDir(), slugid.V4())
	tempStorage, err := runtime.NewTemporaryStorage(tempPath)
	if err != nil {
		t.Fatal(err)
	}

	environment := &runtime.Environment{
		TemporaryStorage: tempStorage,
	}
	engineProvider := extpoints.EngineProviders.Lookup("mock")
	engine, err := engineProvider.NewEngine(extpoints.EngineOptions{
		Environment: environment,
		Log:         logger.WithField("engine", "mock"),
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	tm, err := newTaskManager(&config.Config{}, engine, environment, logger.WithField("test", "TestRunTask"))
	if err != nil {
		t.Fatal(err)
	}

	tr := &TaskRun{
		TaskId: "abc",
		RunId:  1,
		Definition: queue.TaskDefinitionResponse{
			Payload: []byte(`{"start": {"delay": 10,"function": "write-log","argument": "Hello World"}}`),
		},
	}
	err = tm.runTask(tr)
	if err != nil {
		t.Fatal(err)
	}
}

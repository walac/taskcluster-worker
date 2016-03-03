package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/taskcluster/slugid-go/slugid"
	"github.com/taskcluster/taskcluster-client-go/queue"
	"github.com/taskcluster/taskcluster-worker/config"
	"github.com/taskcluster/taskcluster-worker/engines"
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
	result := tm.runTask(tr)
	assert.True(t, result.Success(), "Task should have been successfully completed")
}

func TestCancelTask(t *testing.T) {
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
			Payload: []byte(`{"start": {"delay": 30000,"function": "write-log","argument": "Hello World"}}`),
		},
	}
	result := make(chan engines.ResultSet)
	go func() {
		result <- tm.runTask(tr)
	}()

	time.Sleep(2 * time.Second)
	tm.Stop()

	r := <-result
	assert.False(t, r.Success(), "Task should not have been succesful.")
}

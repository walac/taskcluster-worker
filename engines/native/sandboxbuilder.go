package nativeengine

import (
	"regexp"

	"github.com/Sirupsen/logrus"
	"github.com/walac/taskcluster-worker/engines"
	"github.com/walac/taskcluster-worker/runtime"
)

type sandboxBuilder struct {
	engines.SandboxBuilderBase
	engine  *engine
	log     *logrus.Entry
	payload payload
	context *runtime.TaskContext
	env     map[string]string
}

var envVarPattern = regexp.MustCompile("^[a-zA-Z0-9_-]+$")

func (b *sandboxBuilder) SetEnvironmentVariable(name string, value string) error {
	if !envVarPattern.MatchString(name) {
		return engines.NewMalformedPayloadError(
			"Environment variables name: '", name, "' doesn't match: ",
			envVarPattern.String(),
		)
	}
	if _, ok := b.env[name]; ok {
		return engines.ErrNamingConflict
	}
	b.env[name] = value
	return nil
}

func (b *sandboxBuilder) StartSandbox() (engines.Sandbox, error) {
	return newSandbox(b)
}

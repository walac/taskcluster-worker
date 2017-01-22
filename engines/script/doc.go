// Package scriptengine provides an engine that can be configured with a script
// and a JSON schema, such that the worker executes declarative tasks.
package scriptengine

import "github.com/walac/taskcluster-worker/runtime/util"

var debug = util.Debug("scriptengine")

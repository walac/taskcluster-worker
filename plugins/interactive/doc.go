// Package interactive implements the plugin that serves the interactive
// display and shell sessions over websockets.
//
// The package can also be used as library that provides functionality to host
// display and shell sessions over websockets. This is useful for reusing the
// code in small utilities.
package interactive

import (
	"github.com/walac/taskcluster-worker/plugins"
	"github.com/walac/taskcluster-worker/runtime/util"
)

var debug = util.Debug("interactive")

func init() {
	plugins.Register("interactive", provider{})
}

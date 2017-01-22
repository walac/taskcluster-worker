package osxnative

import "github.com/walac/taskcluster-worker/engines"

func init() {
	// Register the mac engine as an import side-effect
	engines.Register("macosx", engineProvider{})
}

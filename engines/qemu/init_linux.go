package qemuengine

import "github.com/walac/taskcluster-worker/engines"

func init() {
	engines.Register("qemu", engineProvider{})
}

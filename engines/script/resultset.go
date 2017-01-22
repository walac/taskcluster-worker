package scriptengine

import "github.com/walac/taskcluster-worker/engines"

type resultSet struct {
	engines.ResultSetBase
	success bool
}

func (r *resultSet) Success() bool {
	return r.success
}

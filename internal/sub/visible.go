package sub

// IncludeNode is whether a node belongs in a subscription body.
// Dead agents (alive=false) are omitted even if the last Apply was ready.
func IncludeNode(status string, applied []string, alive bool) bool {
	if !alive || len(applied) == 0 {
		return false
	}
	switch status {
	case "ready", "degraded", "offline":
		return true
	default:
		return false
	}
}

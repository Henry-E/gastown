package daemon

import "os/exec"

// PrepareDetachedCommand places a background daemon child in its own process
// group so parent session shutdown does not signal it as collateral damage.
func PrepareDetachedCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	setSysProcAttr(cmd)
}

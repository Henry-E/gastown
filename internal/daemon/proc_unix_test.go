//go:build unix

package daemon

import (
	"os/exec"
	"testing"
)

func TestPrepareDetachedCommand_SetsProcessGroup(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "1")
	PrepareDetachedCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be set")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Fatal("expected Setpgid to be true")
	}
}

package pilot

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/apenella/go-ansible/pkg/adhoc"
	"github.com/gh-efforts/lotus-pilot/build"
)

const RunCmdTimeout = time.Second * 30

func disableAPCmd(ctx context.Context, hostname, miner string) error {
	if build.SkipAnsible {
		log.Debug("disableAPCmd test")
		return nil
	}
	arg := fmt.Sprintf("lotus-worker --worker-repo=%s tasks disable AP", workerRepo(miner))

	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       arg,
	}

	adhoc := &adhoc.AnsibleAdhocCmd{
		Pattern: hostname,
		Options: ansibleAdhocOptions,
	}

	log.Debugw("disableAPCmd", "Command: ", adhoc.String())

	tctx, cancel := context.WithTimeout(ctx, RunCmdTimeout)
	defer cancel()
	err := adhoc.Run(tctx)
	if err != nil {
		return err
	}

	return nil
}

func copyScriptCmd(ctx context.Context, hostname, to, scriptsPath string) error {
	if build.SkipAnsible {
		log.Debug("copyScriptCmd test")
		return nil
	}
	src := fmt.Sprintf("%s/%s.sh", scriptsPath, to)
	dest := fmt.Sprintf("/root/%s.sh", to)

	if _, err := os.Stat(src); err != nil {
		return err
	}

	arg := fmt.Sprintf("src=%s dest=%s mode=777", src, dest)
	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "copy",
		Args:       arg,
	}

	adhoc := &adhoc.AnsibleAdhocCmd{
		Pattern: hostname,
		Options: ansibleAdhocOptions,
	}

	log.Debugw("copyScriptCmd", "Command: ", adhoc.String())

	tctx, cancel := context.WithTimeout(ctx, RunCmdTimeout)
	defer cancel()
	err := adhoc.Run(tctx)
	if err != nil {
		return err
	}

	return nil
}

func workerRunCmd(ctx context.Context, hostname, to, scriptsPath string) error {
	if build.SkipAnsible {
		log.Debug("workerRunCmd test")
		return nil
	}
	err := copyScriptCmd(ctx, hostname, to, scriptsPath)
	if err != nil {
		return err
	}

	arg := fmt.Sprintf("bash /root/%s.sh", to)
	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       arg,
	}

	adhoc := &adhoc.AnsibleAdhocCmd{
		Pattern: hostname,
		Options: ansibleAdhocOptions,
	}

	log.Debugw("workerRunCmd", "Command: ", adhoc.String())

	tctx, cancel := context.WithTimeout(ctx, RunCmdTimeout)
	defer cancel()
	err = adhoc.Run(tctx)
	if err != nil {
		return err
	}
	return nil
}

func workerRepo(miner string) string {
	return fmt.Sprintf("/media/nvme/%s/.lotusworker", miner)
}

func workerStopCmd(ctx context.Context, hostname, from string) error {
	if build.SkipAnsible {
		log.Debug("workerStopCmd test")
		return nil
	}
	arg := fmt.Sprintf("lotus-worker --worker-repo=%s stop", workerRepo(from))

	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       arg,
	}

	adhoc := &adhoc.AnsibleAdhocCmd{
		Pattern: hostname,
		Options: ansibleAdhocOptions,
	}

	log.Debugw("workerStopCmd", "Command: ", adhoc.String())

	tctx, cancel := context.WithTimeout(ctx, RunCmdTimeout)
	defer cancel()
	err := adhoc.Run(tctx)
	if err != nil {
		return err
	}

	return nil
}

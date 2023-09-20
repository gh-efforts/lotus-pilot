package miner

import (
	"context"
	"fmt"
	"os"
	"text/template"

	"github.com/apenella/go-ansible/pkg/adhoc"
	"github.com/filecoin-project/go-state-types/abi"
)

const SCRIPTS_PATH = "./scripts"

type MinerPase struct {
	MinerID      string
	MinerAPIInfo string
}

func createScript(miner, token string, size abi.SectorSize) error {
	var t *template.Template
	var err error

	mp := MinerPase{
		MinerID:      miner,
		MinerAPIInfo: token,
	}

	if size == 68719476736 {
		t, err = template.ParseFiles("./template/worker64G.tmpl")
		if err != nil {
			return err
		}
	} else {
		t, err = template.ParseFiles("./template/worker32G.tmpl")
		if err != nil {
			return err
		}
	}

	f, err := os.Create(fmt.Sprintf("%s/%s.sh", SCRIPTS_PATH, miner))
	if err != nil {
		return err
	}
	defer f.Close()

	err = t.Execute(f, mp)
	if err != nil {
		return err
	}

	log.Infof("create script: %s", fmt.Sprintf("%s/%s.sh", SCRIPTS_PATH, miner))

	return nil
}

func disableAPCmd(ctx context.Context, hostname, miner string) error {
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

	err := adhoc.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}

func enableAPCmd(ctx context.Context, hostname, from string) error {
	arg := fmt.Sprintf("lotus-worker --worker-repo=%s tasks enable AP", workerRepo(from))

	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       arg,
	}

	adhoc := &adhoc.AnsibleAdhocCmd{
		Pattern: hostname,
		Options: ansibleAdhocOptions,
	}

	log.Debugw("enableAPCmd", "Command: ", adhoc.String())

	err := adhoc.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}

func copyScriptCmd(ctx context.Context, hostname, to string, token string, size abi.SectorSize) error {
	src := fmt.Sprintf("%s/%s.sh", SCRIPTS_PATH, to)
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

	err := adhoc.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}

func workerRunCmd(ctx context.Context, hostname, to string, token string, size abi.SectorSize) error {
	err := copyScriptCmd(ctx, hostname, to, token, size)
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

	err = adhoc.Run(ctx)
	if err != nil {
		return err
	}
	//TODO: 可能执行失败, 需要检查执行输出是否包含以下信息
	//[WARNING]: Could not match supplied host pattern, ignoring: DCZ-2007FD101U42-L01-W29
	//[WARNING]: No hosts matched, nothing to do
	return nil
}

func workerRepo(miner string) string {
	return fmt.Sprintf("/media/nvme/%s/.lotusworker", miner)
}

func workerStopCmd(ctx context.Context, hostname, from string) error {
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

	err := adhoc.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}

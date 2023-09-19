package miner

import (
	"context"
	"fmt"

	"github.com/apenella/go-ansible/pkg/adhoc"
	"github.com/filecoin-project/go-state-types/abi"
)

// TODO: support update
var env32G = map[string]interface{}{
	"MINER_API_INFO":                     "",
	"TRUST_PARAMS":                       "1",
	"FIL_PROOFS_USE_GPU_COLUMN_BUILDER":  "1",
	"FIL_PROOFS_USE_GPU_TREE_BUILDER":    "1",
	"FIL_PROOFS_USE_MULTICORE_SDR":       "1",
	"FIL_PROOFS_MULTICORE_SDR_PRODUCERS": "1",
	"FIL_PROOFS_MAXIMIZE_CACHING":        "1",
	"FIL_PROOFS_LAYER_CACHE_SIZE":        "32",
	"FIL_PROOFS_PARENT_CACHE":            "/media/nvme/parent_cache",
	"P1_SEPARATE_PROCESS":                "1",
	"P2_SEPARATE_PROCESS":                "1",
	"PC1_32G_MAX_CONCURRENT":             "24",
	"PC1_32G_MAX_PARALLELISM":            "1",
	"PC2_32G_MAX_CONCURRENT":             "2",
	"PC2_32G_MAX_PARALLELISM_GPU":        "2",
	"PC2_32G_GPU_UTILIZATION":            "1.0",
	"AP_32G_MAX_CONCURRENT":              "1",
	"GET_32G_MAX_CONCURRENT":             "1",
	"READ_STORAGE_FROM_MINER":            "1",
	"RUST_LOG":                           "info",
	"P1_SEPARATE_LOG_PATH":               "/media/nvme/logs",
	"P2_SEPARATE_LOG_PATH":               "/media/nvme/logs",
}

var env64G = map[string]interface{}{
	"MINER_API_INFO":                     "",
	"TRUST_PARAMS":                       "1",
	"FIL_PROOFS_USE_GPU_COLUMN_BUILDER":  "1",
	"FIL_PROOFS_USE_GPU_TREE_BUILDER":    "1",
	"FIL_PROOFS_USE_MULTICORE_SDR":       "1",
	"FIL_PROOFS_MULTICORE_SDR_PRODUCERS": "3",
	"FIL_PROOFS_MAXIMIZE_CACHING":        "1",
	"FIL_PROOFS_LAYER_CACHE_SIZE":        "64",
	"FIL_PROOFS_PARENT_CACHE":            "/media/nvme/parent_cache",
	"P1_SEPARATE_PROCESS":                "1",
	"P2_SEPARATE_PROCESS":                "1",
	"PC1_64G_MAX_CONCURRENT":             "14",
	"PC1_64G_MAX_PARALLELISM":            "2",
	"PC2_64G_MAX_CONCURRENT":             "1",
	"PC2_64G_MAX_PARALLELISM_GPU":        "4",
	"PC2_64G_GPU_UTILIZATION":            "2.0",
	"AP_64G_MAX_CONCURRENT":              "1",
	"GET_64G_MAX_CONCURRENT":             "1",
	"READ_STORAGE_FROM_MINER":            "1",
	"RUST_LOG":                           "info",
	"P1_SEPARATE_LOG_PATH":               "/media/nvme/logs",
	"P2_SEPARATE_LOG_PATH":               "/media/nvme/logs",
}

func disableAPCmd(ctx context.Context, hostname, miner string) error {
	cmd := fmt.Sprintf("lotus-worker --worker-repo=%s tasks disable AP", workerRepo(miner))

	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       cmd,
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
	cmd := fmt.Sprintf("lotus-worker --worker-repo=%s tasks enable AP", workerRepo(from))

	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       cmd,
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

func workerRunCmd(ctx context.Context, hostname, to string, token string, size abi.SectorSize) error {
	env := env32G
	if size == 68719476736 {
		env = env64G
	}
	env["MINER_API_INFO"] = token

	path := workerRepo(to)
	cmd := fmt.Sprintf("mkdir -p %s && nohup lotus-worker --worker-repo=%s run --commit=false --listen 0.0.0.0:3457 >> %s/daemon.log 2>&1 &", path, path, path)
	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       cmd,
		ExtraVars:  env,
	}

	adhoc := &adhoc.AnsibleAdhocCmd{
		Pattern: hostname,
		Options: ansibleAdhocOptions,
	}

	log.Debugw("workerRunCmd", "Command: ", adhoc.String())

	err := adhoc.Run(ctx)
	if err != nil {
		return err
	}
	//TODO: 可能执行失败, 需要检查执行输出是否包含以下信息
	//[WARNING]: Could not match supplied host pattern, ignoring: DCZ-2007FD101U42-L01-W29
	//[WARNING]: No hosts matched, nothing to do
	return nil
}

func workerRepo(miner string) string {
	return fmt.Sprintf("/media/nvme/%s", miner)
}

func workerStopCmd(ctx context.Context, hostname, from string) error {
	cmd := fmt.Sprintf("lotus-worker --worker-repo=%s stop", workerRepo(from))

	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       cmd,
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

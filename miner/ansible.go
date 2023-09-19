package miner

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/apenella/go-ansible/pkg/adhoc"
	"github.com/filecoin-project/go-state-types/abi"
)

const worker32G = `
#!/bin/bash
ulimit -n 655350

base_dir=/media/nvme/{{.MinerID}}
mkdir -p $base_dir

repo_dir=$base_dir/.lotusworker
log_dir=$base_dir/log
tmp_dir=$base_dir/tmp
parent_cache_dir=$base_dir/parent_cache

mkdir -p $repo_dir $log_dir $tmp_dir $parent_cache_dir

# miner 的api信息，参考miner部署环节
export MINER_API_INFO={{.MinerAPIInfo}}


## 选择可以用的显卡
if [[ -f /etc/gpu.conf ]];then
        gpu_x=cat /etc/gpu.conf
        if [[ $(echo $gpu_x|grep unset) ]];then
                        unset CUDA_VISIBLE_DEVICES
			gpu_number=$(nvidia-smi -L|wc -l)
        else
                gpu=$(echo $gpu_x|awk -F '=' '{print $2}')
                export CUDA_VISIBLE_DEVICES=$gpu
                gpu_number=1
        fi
else
        echo "no find /etc/gpu.conf" 
        exit
fi


export TRUST_PARAMS=1
export TMPDIR=$tmp_dir

# 封存参数配置， 一般不需要改动
export FIL_PROOFS_USE_GPU_COLUMN_BUILDER=1
export FIL_PROOFS_USE_GPU_TREE_BUILDER=1
export FIL_PROOFS_USE_MULTICORE_SDR=1
export FIL_PROOFS_MULTICORE_SDR_PRODUCERS=1
export FIL_PROOFS_MAXIMIZE_CACHING=1
export FIL_PROOFS_LAYER_CACHE_SIZE=32
export FIL_PROOFS_PARENT_CACHE=$parent_cache_dir
# 使用独立的程序做P1,P2任务， gh的代码需要开启，不开启可能封存有异常
export P1_SEPARATE_PROCESS=1
export P2_SEPARATE_PROCESS=1

# 设置 PC1 的并行数量， 必须设置，根据配置情况提前计算好
export PC1_32G_MAX_CONCURRENT=24

# 设置 PC1 的CPU使用数量，必须设置，默认设置为是1, 一般不需要修改
export PC1_32G_MAX_PARALLELISM=1

# 设置 PC2 的并行数量， 必须设置，根据配置情况提前计算好
if [[ $gpu_number == 1 ]];then
        export PC2_32G_MAX_CONCURRENT=1
elif [[ $gpu_number > 1 ]];then
        export PC2_32G_MAX_CONCURRENT=2
        export CUDA_VISIBLE_DEVICES=0,1
fi

# 设置 PC2 的CPU使用数量，必须设置，默认设置为是2, 一般不需要修改
export PC2_32G_MAX_PARALLELISM_GPU=2

# 设置 PC2 的GPU使用数量，必须设置，默认设置为是1, 一般不需要修改
export PC2_32G_GPU_UTILIZATION=1

# 设置 AP 任务并行数， 设置为1， 一般不需要修改。
export AP_32G_MAX_CONCURRENT=1
export FAST_ADD_PIECE=1

# 设置 GET 任务并行数， 设置为1， 一般不需修改。
export GET_32G_MAX_CONCURRENT=1

# 如果Miner使用了七牛，那么Worker也必须使用，否则数据存储会有问题，生产中都使用七牛。
export MUST_USE_QINIU=1
export QINIU_READER_CONFIG_PATH=/media/nvme/ap-read.json
# Rust 日志等级
export RUST_LOG=info

# 使用独立进程做P1, P2, 也会生成独立的日志文件，建议配置到一个单独的目录， 需要提前创建好
# 如果不配置，P1, P2的日志会和Worker日志混在一起，不便排查问题。
export P1_SEPARATE_LOG_PATH=$log_dir
export P2_SEPARATE_LOG_PATH=$log_dir


nohup lotus-worker --worker-repo=$repo_dir run --commit=false --precommit2=false --precommit1=false --addpiece=false --prove-replica-update2=false --replica-update=false>> $base_dir/worker.log 2>&1 &
`

const worker64G = `
#!/bin/bash
ulimit -n 655350

base_dir=/media/nvme/{{.MinerID}}
mkdir -p $base_dir

repo_dir=$base_dir/.lotusworker
log_dir=$base_dir/log
tmp_dir=$base_dir/tmp
parent_cache_dir=$base_dir/parent_cache

mkdir -p $repo_dir $log_dir $tmp_dir $parent_cache_dir

# miner 的api信息，参考miner部署环节
export MINER_API_INFO={{.MinerAPIInfo}}

## 选择可以用的显卡
if [[ -f /etc/gpu.conf ]];then
        gpu_x=$(cat /etc/gpu.conf)
        if [[ $(echo $gpu_x|grep unset) ]];then
		gpu_number=$(nvidia-smi -L|wc -l)
                        unset CUDA_VISIBLE_DEVICES
        else
                gpu=$(echo $gpu_x|awk -F '=' '{print $2}')
                export CUDA_VISIBLE_DEVICES=$gpu
                gpu_number=1
        fi
else
        echo "no find /etc/gpu.conf" 
        exit
fi
export TRUST_PARAMS=1
export TMPDIR=$tmp_dir
# 封存参数配置， 一般不需要改动
export FIL_PROOFS_USE_GPU_COLUMN_BUILDER=1
export FIL_PROOFS_USE_GPU_TREE_BUILDER=1
export FIL_PROOFS_USE_MULTICORE_SDR=1
export FIL_PROOFS_MULTICORE_SDR_PRODUCERS=3
export FIL_PROOFS_MAXIMIZE_CACHING=1
export FIL_PROOFS_LAYER_CACHE_SIZE=64
export FIL_PROOFS_PARENT_CACHE=$parent_cache_dir
# 使用独立的程序做P1,P2任务， gh的代码需要开启，不开启可能封存有异常
export P1_SEPARATE_PROCESS=1
export P2_SEPARATE_PROCESS=1

# 设置 PC1 的并行数量， 必须设置，根据配置情况提前计算好
export PC1_64G_MAX_CONCURRENT=14

# 设置 PC1 的CPU使用数量，必须设置，默认设置为是1, 一般不需要修改
export PC1_64G_MAX_PARALLELISM=2

# 设置 PC2 的并行数量， 必须设置，根据配置情况提前计算好
if [[ $gpu_number > 1 ]];then
        export PC2_64G_MAX_CONCURRENT=1
        export CUDA_VISIBLE_DEVICES=0,1
fi

# 设置 PC2 的CPU使用数量，必须设置，默认设置为是2, 一般不需要修改
export PC2_64G_MAX_PARALLELISM_GPU=2

# 设置 PC2 的GPU使用数量，必须设置，默认设置为是1, 一般不需要修改
export PC2_64G_GPU_UTILIZATION=1

# 设置 AP 任务并行数， 设置为1， 一般不需要修改。
export AP_64G_MAX_CONCURRENT=1
export FAST_ADD_PIECE=1

# 设置 GET 任务并行数， 设置为1， 一般不需修改。
export GET_64G_MAX_CONCURRENT=1

# 如果Miner使用了七牛，那么Worker也必须使用，否则数据存储会有问题，生产中都使用七牛。
export MUST_USE_QINIU=1
export QINIU_READER_CONFIG_PATH=/media/nvme/ap-read.json
# Rust 日志等级
export RUST_LOG=info

# 使用独立进程做P1, P2, 也会生成独立的日志文件，建议配置到一个单独的目录， 需要提前创建好
# 如果不配置，P1, P2的日志会和Worker日志混在一起，不便排查问题。
export P1_SEPARATE_LOG_PATH=$log_dir
export P2_SEPARATE_LOG_PATH=$log_dir

nohup lotus-worker --worker-repo=$repo_dir run --commit=false --precommit2=false --precommit1=false --addpiece=false --prove-replica-update2=false --replica-update=false>> $base_dir/worker.log 2>&1 &
`

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

type MinerPase struct {
	MinerID      string
	MinerAPIInfo string
}

func copyScriptCmd(ctx context.Context, hostname, to string, token string, size abi.SectorSize) error {
	mp := MinerPase{
		MinerID:      to,
		MinerAPIInfo: token,
	}
	var t *template.Template
	var err error
	if size == 68719476736 {
		t, err = template.New("worker").Parse(worker64G)
		if err != nil {
			return err
		}
	} else {
		t, err = template.New("worker").Parse(worker32G)
		if err != nil {
			return err
		}
	}
	buf := new(bytes.Buffer)
	err = t.Execute(buf, mp)
	if err != nil {
		return err
	}

	path := workerRepo(to)
	arg := fmt.Sprintf("content=\"%s\" src=%s/worker.sh mode=777", buf.String(), path)
	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "copy",
		Args:       arg,
	}

	adhoc := &adhoc.AnsibleAdhocCmd{
		Pattern: hostname,
		Options: ansibleAdhocOptions,
	}

	log.Debugw("copyWorkerCmd", "Command: ", adhoc.String())

	err = adhoc.Run(ctx)
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

	path := workerRepo(to)
	cmd := fmt.Sprintf("bash %s/worker.sh", path)
	ansibleAdhocOptions := &adhoc.AnsibleAdhocOptions{
		ModuleName: "shell",
		Args:       cmd,
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

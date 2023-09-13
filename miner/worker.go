package miner

import (
	"fmt"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/google/uuid"
)

type wst = map[uuid.UUID]storiface.WorkerStats
type jobs = map[uuid.UUID][]storiface.WorkerJob

type Worker struct {
	WorkerID  uuid.UUID
	Hostname  string
	Tasks     map[string]int
	LastStart map[string]time.Time
}

type workerState struct {
	workerID  uuid.UUID
	hostname  string
	disableAP bool
	isSwitch  bool
}

func (m *Miner) statsAndJobs(ma address.Address) (wst, jobs, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	mi, ok := m.miners[ma]
	if !ok {
		return nil, nil, fmt.Errorf("not found miner: %s", ma)
	}

	wst, err := mi.api.WorkerStats(m.ctx)
	if err != nil {
		return nil, nil, err
	}

	jobs, err := mi.api.WorkerJobs(m.ctx)
	if err != nil {
		return nil, nil, err
	}

	return wst, jobs, nil
}

func (m *Miner) workerPick(req switchRequest) (map[string]struct{}, error) {
	wst, jobs, err := m.statsAndJobs(req.from)
	if err != nil {
		return nil, err
	}

	workerHostnames := map[uuid.UUID]string{}
	for wid, st := range wst {
		workerHostnames[wid] = st.Info.Hostname
		log.Debug("WorkerStats", "wid", wid, "TaskCounts", st.TaskCounts)
	}

	var out []Worker
	for wid, jobs := range jobs {
		w := Worker{WorkerID: wid, Hostname: workerHostnames[wid]}
		for _, job := range jobs {
			if job.RunWait < 0 {
				continue
			}
			if job.Task == sealtasks.TTAddPiece {
				w.Tasks["AP"] += 1
				if job.RunWait == storiface.RWRunning {
					if w.LastStart["AP"].Before(job.Start) {
						w.LastStart["AP"] = job.Start
					}
				}
			}
			if job.Task == sealtasks.TTPreCommit1 {
				w.Tasks["PC1"] += 1
				if job.RunWait == storiface.RWRunning {
					if w.LastStart["PC1"].Before(job.Start) {
						w.LastStart["PC1"] = job.Start
					}
				}
			}
			if job.Task == sealtasks.TTPreCommit2 {
				w.Tasks["PC2"] += 1
				if job.RunWait == storiface.RWRunning {
					if w.LastStart["PC2"].Before(job.Start) {
						w.LastStart["PC2"] = job.Start
					}
				}
			}
			out = append(out, w)
		}
	}

	//TODO: task in sched

	sort.Slice(out, func(i, j int) bool {
		if out[i].Tasks["AP"]+out[i].Tasks["PC1"] != out[j].Tasks["AP"]+out[j].Tasks["PC1"] {
			return out[i].Tasks["AP"]+out[i].Tasks["PC1"] < out[j].Tasks["AP"]+out[j].Tasks["PC1"]
		}
		if out[i].Tasks["PC2"] != out[j].Tasks["PC2"] {
			return out[i].Tasks["PC2"] < out[j].Tasks["PC2"]
		}
		if out[i].LastStart["PC1"].Equal(out[j].LastStart["PC1"]) {
			return out[i].Hostname < out[j].Hostname
		}
		return out[i].LastStart["PC1"].Before(out[j].LastStart["PC1"])
	})

	count := req.count
	if len(out) < req.count {
		count = len(out)
	}

	worker := map[string]struct{}{}
	for _, w := range out[0:count] {
		worker[w.Hostname] = struct{}{}
	}

	return worker, nil
}

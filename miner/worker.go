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

type stateWorker int

const (
	stateWorkerBegin stateWorker = iota
	stateWorkerDisabled
	stateWorkerSwithed
	stateWorkerStoped
)

type workerSort struct {
	workerID  uuid.UUID
	hostname  string
	tasks     map[string]int
	lastStart map[string]time.Time
}

type workerState struct {
	workerID uuid.UUID
	hostname string
	state    stateWorker
	errMsg   string
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

func (m *Miner) workerStats(ma address.Address) (wst, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	mi, ok := m.miners[ma]
	if !ok {
		return nil, fmt.Errorf("not found miner: %s", ma)
	}

	wst, err := mi.api.WorkerStats(m.ctx)
	if err != nil {
		return nil, err
	}

	return wst, nil
}

func (m *Miner) workerJobs(ma address.Address) (jobs, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	mi, ok := m.miners[ma]
	if !ok {
		return nil, fmt.Errorf("not found miner: %s", ma)
	}

	jobs, err := mi.api.WorkerJobs(m.ctx)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func (m *Miner) workerPick(req switchRequest) (map[uuid.UUID]*workerState, error) {
	wst, jobs, err := m.statsAndJobs(req.from)
	if err != nil {
		return nil, err
	}

	workerHostnames := map[uuid.UUID]string{}
	for wid, st := range wst {
		workerHostnames[wid] = st.Info.Hostname
		log.Debug("WorkerStats", "wid", wid, "TaskCounts", st.TaskCounts)
	}

	var worker []workerSort
	for wid, jobs := range jobs {
		w := workerSort{workerID: wid, hostname: workerHostnames[wid]}
		for _, job := range jobs {
			if job.RunWait < 0 {
				continue
			}
			if job.Task == sealtasks.TTAddPiece {
				w.tasks["AP"] += 1
				if job.RunWait == storiface.RWRunning {
					if w.lastStart["AP"].Before(job.Start) {
						w.lastStart["AP"] = job.Start
					}
				}
			}
			if job.Task == sealtasks.TTPreCommit1 {
				w.tasks["PC1"] += 1
				if job.RunWait == storiface.RWRunning {
					if w.lastStart["PC1"].Before(job.Start) {
						w.lastStart["PC1"] = job.Start
					}
				}
			}
			if job.Task == sealtasks.TTPreCommit2 {
				w.tasks["PC2"] += 1
				if job.RunWait == storiface.RWRunning {
					if w.lastStart["PC2"].Before(job.Start) {
						w.lastStart["PC2"] = job.Start
					}
				}
			}
			worker = append(worker, w)
		}
	}

	//TODO: task in sched

	sort.Slice(worker, func(i, j int) bool {
		if worker[i].tasks["AP"]+worker[i].tasks["PC1"] != worker[j].tasks["AP"]+worker[j].tasks["PC1"] {
			return worker[i].tasks["AP"]+worker[i].tasks["PC1"] < worker[j].tasks["AP"]+worker[j].tasks["PC1"]
		}
		if worker[i].tasks["PC2"] != worker[j].tasks["PC2"] {
			return worker[i].tasks["PC2"] < worker[j].tasks["PC2"]
		}
		if worker[i].lastStart["PC1"].Equal(worker[j].lastStart["PC1"]) {
			return worker[i].hostname < worker[j].hostname
		}
		return worker[i].lastStart["PC1"].Before(worker[j].lastStart["PC1"])
	})

	count := req.count
	if len(worker) < req.count {
		count = len(worker)
	}

	ret := map[uuid.UUID]*workerState{}
	for _, w := range worker[0:count] {
		ret[w.workerID] = &workerState{
			workerID: w.workerID,
			hostname: w.hostname,
			state:    stateWorkerBegin,
		}
	}

	return ret, nil
}

func (m *Miner) workerSwitch(req switchRequest, wl []uuid.UUID) error {
	jobs, err := m.workerJobs(req.from)
	if err != nil {
		return err
	}
	var filter []uuid.UUID
	for _, w := range wl {
		if _, ok := jobs[w]; !ok {
			filter = append(filter, w)
		}
	}

	//workerRunCmd()
	//update switch and worker state
	return nil
}

func (m *Miner) disabledWorker(id SwitchID) []uuid.UUID {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	out := []uuid.UUID{}
	ss, ok := m.switchs[id]
	if ok {
		for wid, w := range ss.worker {
			if w.state == stateWorkerDisabled {
				out = append(out, wid)
			}
		}
	}

	return out
}

func (m *Miner) switchedWorker(id SwitchID) []uuid.UUID {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	out := []uuid.UUID{}
	ss, ok := m.switchs[id]
	if ok {
		for wid, w := range ss.worker {
			if w.state == stateWorkerSwithed {
				out = append(out, wid)
			}
		}
	}

	return out
}

func (m *Miner) updateWorkerState() {
	m.swLk.Lock()
	defer m.swLk.Unlock()
}

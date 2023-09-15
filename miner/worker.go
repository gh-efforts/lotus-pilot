package miner

import (
	"fmt"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/google/uuid"
)

type wst = map[uuid.UUID]storiface.WorkerStats
type jobs = map[uuid.UUID][]storiface.WorkerJob

type stateWorker int

const (
	stateWorkerPicked stateWorker = iota
	stateWorkerAPDisabled
	stateWorkerSwithed
	stateWorkerStoped
)

type workerSort struct {
	workerID  uuid.UUID
	hostname  string
	runing    map[string]int //taskType
	prepared  map[string]int
	assigned  map[string]int
	lastStart map[string]time.Time //last runing start time
	sched     map[string]int       //task in sched
}

type workerState struct {
	workerID uuid.UUID
	hostname string
	state    stateWorker
	errMsg   string
}

func (w *workerSort) sum(tt string) int {
	return w.runing[tt] + w.prepared[tt] + w.assigned[tt] + w.sched[tt]
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

func (m *Miner) workerPick(req *switchRequest) (map[uuid.UUID]*workerState, error) {
	wst, jobs, err := m.statsAndJobs(req.from)
	if err != nil {
		return nil, err
	}

	if len(wst) < req.count {
		return nil, fmt.Errorf("not enough worker. miner: %s has: %d need: %d", req.from, len(wst), req.count)
	}

	worker := map[uuid.UUID]workerSort{}
	for wid, st := range wst {
		worker[wid] = workerSort{
			workerID:  wid,
			hostname:  st.Info.Hostname,
			runing:    map[string]int{},
			prepared:  map[string]int{},
			assigned:  map[string]int{},
			lastStart: make(map[string]time.Time),
		}
	}

	for wid, jobs := range jobs {
		for _, job := range jobs {
			if job.RunWait < 0 {
				continue
			}
			if _, ok := worker[wid]; !ok {
				log.Warnf("wid not found")
				continue
			}

			if job.RunWait == storiface.RWRunning {
				worker[wid].runing[job.Task.Short()] += 1
				if worker[wid].lastStart[job.Task.Short()].Before(job.Start) {
					worker[wid].lastStart[job.Task.Short()] = job.Start
				}
			} else if job.RunWait == storiface.RWPrepared {
				worker[wid].prepared[job.Task.Short()] += 1
			} else {
				//assigned
				worker[wid].assigned[job.Task.Short()] += 1
			}
		}
	}

	//TODO: task in sched

	var ws []workerSort
	for _, w := range worker {
		ws = append(ws, w)
	}
	sort.Slice(ws, func(i, j int) bool {
		wi := ws[i]
		wj := ws[j]

		if wi.sum("AP")+wi.sum("PC1") != wj.sum("AP")+wj.sum("PC1") {
			return wi.sum("AP")+wi.sum("PC1") < wj.sum("AP")+wj.sum("PC1")
		}

		if wi.sum("PC2") != wj.sum("PC2") {
			return wi.sum("PC2") < wj.sum("PC2")
		}

		if wi.lastStart["PC1"].Equal(wj.lastStart["PC1"]) {
			return wi.hostname < wj.hostname
		}

		return wi.lastStart["PC1"].Before(wj.lastStart["PC1"])
	})

	ret := map[uuid.UUID]*workerState{}
	for _, w := range ws[0:req.count] {
		ret[w.workerID] = &workerState{
			workerID: w.workerID,
			hostname: w.hostname,
			state:    stateWorkerPicked,
		}
	}

	return ret, nil
}

func (m *Miner) workerSwitch(req *switchRequest, wl []uuid.UUID) error {
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

	//TODO: task in sched

	//workerRunCmd()
	//update switch and worker state
	return nil
}

func (m *Miner) disabledWorker(id switchID) []uuid.UUID {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	out := []uuid.UUID{}
	ss, ok := m.switchs[id]
	if ok {
		for wid, w := range ss.worker {
			if w.state == stateWorkerAPDisabled {
				out = append(out, wid)
			}
		}
	}

	return out
}

func (m *Miner) switchedWorker(id switchID) []uuid.UUID {
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

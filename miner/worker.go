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
	stateWorkerPicked    stateWorker = iota
	stateWorkerSwitching             //waiting to switch
	stateWorkerSwithed
	stateWorkerStoped

	stateWorkerError
)

var stateWorkerNames = map[stateWorker]string{
	stateWorkerPicked:    "workerPicked",
	stateWorkerSwitching: "workerSwitching",
	stateWorkerSwithed:   "workerSwithed",
	stateWorkerStoped:    "workerStoped",
	stateWorkerError:     "workerError",
}

func (s stateWorker) String() string {
	return stateWorkerNames[s]
}

const ErrTryCount = 10

type workerInfo struct {
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
	try      int
}

func (w *workerState) updateErr(errMsg string) {
	w.try += 1
	w.errMsg = errMsg
	if w.try > ErrTryCount {
		w.state = stateWorkerError
	}
}

func (w *workerInfo) sum(tt string) int {
	return w.runing[tt] + w.prepared[tt] + w.assigned[tt] + w.sched[tt]
}

func (w *workerInfo) canSwitch() bool {
	return w.sum("AP")+w.sum("PC1")+w.sum("PC2") == 0
}

func (w *workerInfo) canStop() bool {
	var all int
	for _, v := range w.runing {
		all += v
	}
	for _, v := range w.prepared {
		all += v
	}
	for _, v := range w.assigned {
		all += v
	}
	for _, v := range w.sched {
		all += v
	}

	return all == 0
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

func (m *Miner) getWorkerInfo(ma address.Address) (map[uuid.UUID]workerInfo, error) {
	wst, jobs, err := m.statsAndJobs(ma)
	if err != nil {
		return nil, err
	}

	worker := map[uuid.UUID]workerInfo{}
	for wid, st := range wst {
		if !workerCheck(st) {
			log.Debugf("worker: %s illegal", wid)
			continue
		}

		worker[wid] = workerInfo{
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
				log.Warnf("worker: %s not found", wid)
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

	return worker, nil
}

func (m *Miner) workerPick(req *switchRequest) (map[uuid.UUID]*workerState, error) {
	out := map[uuid.UUID]*workerState{}

	if len(req.worker) != 0 {
		//specify worker from requst
		wst, err := m.workerStats(req.from)
		if err != nil {
			return nil, err
		}

		for _, w := range req.worker {
			ws, ok := wst[w]
			if !ok {
				return nil, fmt.Errorf("worker: %s not found in wst", w)
			}

			if !workerCheck(ws) {
				return nil, fmt.Errorf("specify worker: %s illegal", w)
			}

			out[w] = &workerState{
				workerID: w,
				hostname: ws.Info.Hostname,
				state:    stateWorkerPicked,
			}
		}

		return out, nil
	}

	worker, err := m.getWorkerInfo(req.from)
	if err != nil {
		return nil, err
	}

	if len(worker) < req.count {
		return nil, fmt.Errorf("not enough worker. miner: %s has: %d need: %d", req.from, len(worker), req.count)
	}

	var workerSort []workerInfo
	for _, w := range worker {
		workerSort = append(workerSort, w)
	}

	sort.Slice(workerSort, func(i, j int) bool {
		wi := workerSort[i]
		wj := workerSort[j]

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

	for _, w := range workerSort[0:req.count] {
		out[w.workerID] = &workerState{
			workerID: w.workerID,
			hostname: w.hostname,
			state:    stateWorkerPicked,
		}
	}

	return out, nil
}

func workerCheck(st storiface.WorkerStats) bool {
	//skip winPost worker
	if len(st.Tasks) > 0 {
		if st.Tasks[0].WorkerType() != sealtasks.WorkerSealing {
			return false
		}
	}

	//skip diabled worker
	if !st.Enabled {
		return false
	}

	//skip miner local worker
	enablePC1 := false
	enablePC2 := false
	for _, t := range st.Tasks {
		if t == sealtasks.TTPreCommit1 {
			enablePC1 = true
		}
		if t == sealtasks.TTPreCommit2 {
			enablePC2 = true
		}
	}
	if !enablePC1 && !enablePC2 {
		return false
	}

	return true
}

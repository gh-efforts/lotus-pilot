package miner

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/google/uuid"
)

type wst = map[uuid.UUID]storiface.WorkerStats
type jobs = map[uuid.UUID][]storiface.WorkerJob
type sts = map[storiface.ID][]storiface.Decl

const CacheTimeout = time.Second * 30

type workerInfoCache struct {
	worker    map[uuid.UUID]workerInfo
	cacheTime time.Time
}
type workerStatsCache struct {
	worker    map[uuid.UUID]storiface.WorkerStats
	cacheTime time.Time
}

type SchedInfo struct {
	SchedInfo    SchedDiagInfo
	ReturnedWork []string
	Waiting      []string
	CallToWork   map[string]string
	EarlyRet     []string
}
type SchedDiagInfo struct {
	Requests    []SchedDiagRequestInfo
	OpenWindows []string
}
type SchedDiagRequestInfo struct {
	Sector   abi.SectorID
	TaskType sealtasks.TaskType
	Priority int
	SchedId  uuid.UUID
}

type StateWorker int

const (
	StateWorkerPicked StateWorker = iota
	StateWorkerDisableAPConfirming
	StateWorkerSwitchWaiting
	StateWorkerSwitchConfirming
	StateWorkerStopWaiting
	StateWorkerStopConfirming
	StateWorkerComplete

	StateWorkerError
)

var stateWorkerNames = map[StateWorker]string{
	StateWorkerPicked:              "workerPicked",
	StateWorkerDisableAPConfirming: "workerDisableAPConfirming",
	StateWorkerSwitchWaiting:       "workerSwitchWaiting",
	StateWorkerSwitchConfirming:    "workerSwitchConfirming",
	StateWorkerStopWaiting:         "workerStopWaiting",
	StateWorkerStopConfirming:      "workerStopConfirming",
	StateWorkerComplete:            "workeComplete",
	StateWorkerError:               "workerError",
}

func (s StateWorker) String() string {
	return stateWorkerNames[s]
}

const ErrTryCount = 10

type workerInfo struct {
	workerID  uuid.UUID
	storageID storiface.ID
	hostname  string
	runing    map[string]int //taskType
	prepared  map[string]int
	assigned  map[string]int
	lastStart map[string]time.Time //last runing start time
	sched     map[string]int       //task in sched
	sectors   map[abi.SectorID]struct{}
	tasks     map[sealtasks.TaskType]struct{}
}

type WorkerState struct {
	WorkerID uuid.UUID   `json:"workerID"`
	Hostname string      `json:"hostname"`
	State    StateWorker `json:"state"`
	ErrMsg   string      `json:"errMsg"`
	Try      int         `json:"try"`
}

func (w *WorkerState) updateErr(errMsg string) {
	w.Try += 1
	w.ErrMsg = errMsg
	if w.Try > ErrTryCount {
		w.State = StateWorkerError
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

	return all+len(w.sectors) == 0
}

func (m *Miner) workerInfoAPI(ma address.Address) (wst, jobs, sts, SchedDiagInfo, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	mi, ok := m.miners[ma]
	if !ok {
		return nil, nil, nil, SchedDiagInfo{}, fmt.Errorf("not found miner: %s", ma)
	}

	wst, err := mi.api.WorkerStats(m.ctx)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	jobs, err := mi.api.WorkerJobs(m.ctx)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	sts, err := mi.api.StorageList(m.ctx)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	schedb, err := mi.api.SealingSchedDiag(m.ctx, false)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	j, err := json.Marshal(&schedb)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	var b SchedInfo
	err = json.Unmarshal(j, &b)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	log.Debug(b.SchedInfo)
	return wst, jobs, sts, b.SchedInfo, nil
}

func (m *Miner) getWorkerStats(ma address.Address) (map[uuid.UUID]storiface.WorkerStats, error) {
	cache, ok := m.statsCache[ma]
	if ok && time.Now().Before(cache.cacheTime.Add(CacheTimeout)) {
		return cache.worker, nil
	}

	worker, err := m.workerStats(ma)
	if err != nil {
		return nil, err
	}
	m.statsCache[ma] = workerStatsCache{
		worker:    worker,
		cacheTime: time.Now(),
	}

	return worker, nil
}

func (m *Miner) getWorkerInfo(ma address.Address) (map[uuid.UUID]workerInfo, error) {
	cache, ok := m.infoCache[ma]
	if ok && time.Now().Before(cache.cacheTime.Add(CacheTimeout)) {
		return cache.worker, nil
	}

	worker, err := m._getWorkerInfo(ma)
	if err != nil {
		return nil, err
	}
	m.infoCache[ma] = workerInfoCache{
		worker:    worker,
		cacheTime: time.Now(),
	}

	return worker, nil
}

func (m *Miner) _getWorkerInfo(ma address.Address) (map[uuid.UUID]workerInfo, error) {
	wst, jobs, sts, diag, err := m.workerInfoAPI(ma)
	if err != nil {
		return nil, err
	}

	worker := map[uuid.UUID]workerInfo{}
	sectorWorker := map[abi.SectorID]uuid.UUID{}
	for wid, st := range wst {
		if !workerCheck(st) {
			log.Debugf("worker: %s illegal", wid)
			continue
		}

		var id storiface.ID
		for _, p := range st.Paths {
			if p.CanSeal {
				id = p.ID
				break
			}
		}

		sectors := map[abi.SectorID]struct{}{}
		for _, d := range sts[id] {
			sectors[d.SectorID] = struct{}{}
			sectorWorker[d.SectorID] = wid
		}

		tasks := map[sealtasks.TaskType]struct{}{}
		for _, t := range st.Tasks {
			tasks[t] = struct{}{}
		}

		worker[wid] = workerInfo{
			workerID:  wid,
			storageID: id,
			hostname:  st.Info.Hostname,
			runing:    map[string]int{},
			prepared:  map[string]int{},
			assigned:  map[string]int{},
			lastStart: make(map[string]time.Time),
			sectors:   sectors,
			tasks:     tasks,
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

	//task in sched
	for _, req := range diag.Requests {
		wid, ok := sectorWorker[req.Sector]
		if !ok {
			log.Debugf("sector: %s not found in sectorWorker", req.Sector)
			continue
		}
		if _, ok := worker[wid]; !ok {
			log.Warnf("worker: %s not found", wid)
			continue
		}
		worker[wid].sched[req.TaskType.Short()] += 1
	}

	return worker, nil
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

func (m *Miner) workerPick(req SwitchRequest) (map[uuid.UUID]*WorkerState, error) {
	switchingWorkers := m.switchingWorkers()
	out := map[uuid.UUID]*WorkerState{}

	if len(req.Worker) != 0 {
		//specify worker from requst
		wst, err := m.workerStats(req.From)
		if err != nil {
			return nil, err
		}

		for _, w := range req.Worker {
			ws, ok := wst[w]
			if !ok {
				return nil, fmt.Errorf("worker: %s not found in wst", w)
			}

			if !workerCheck(ws) {
				return nil, fmt.Errorf("specify worker: %s illegal", w)
			}

			if _, ok := switchingWorkers[w]; ok {
				return nil, fmt.Errorf("specify worker: %s already switching", w)
			}

			out[w] = &WorkerState{
				WorkerID: w,
				Hostname: ws.Info.Hostname,
				State:    StateWorkerPicked,
			}
		}

		return out, nil
	}

	worker, err := m._getWorkerInfo(req.From)
	if err != nil {
		return nil, err
	}

	if len(worker) < req.Count {
		return nil, fmt.Errorf("not enough worker. miner: %s has: %d need: %d", req.From, len(worker), req.Count)
	}

	var workerSort []workerInfo
	for _, w := range worker {
		//skip switchingWorkers
		if _, ok := switchingWorkers[w.workerID]; ok {
			continue
		}

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

	for _, w := range workerSort[0:req.Count] {
		out[w.workerID] = &WorkerState{
			WorkerID: w.workerID,
			Hostname: w.hostname,
			State:    StateWorkerPicked,
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

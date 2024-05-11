package pilot

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/gh-efforts/lotus-pilot/build"
	"github.com/google/uuid"
)

type wst = map[uuid.UUID]storiface.WorkerStats
type jobs = map[uuid.UUID][]storiface.WorkerJob
type sts = map[storiface.ID][]storiface.Decl

const CacheTimeout = time.Second * 30

type workerInfoCache struct {
	worker    map[uuid.UUID]WorkerInfo
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

type WorkerInfo struct {
	WorkerID  uuid.UUID            `json:"workerID"`
	StorageID storiface.ID         `json:"storageID"`
	Hostname  string               `json:"hostname"`
	Runing    map[string]int       `json:"runing"` //taskType
	Prepared  map[string]int       `json:"prepared"`
	Assigned  map[string]int       `json:"assigned"`
	LastStart map[string]time.Time `json:"lastStart"` //last runing start time
	Sched     map[string]int       `json:"sched"`     //task in sched
	Sectors   map[string]struct{}  `json:"sectors"`   //sectorID
	Tasks     map[string]struct{}  `json:"tasks"`
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

func (w *WorkerInfo) sum(tt string) int {
	return w.Runing[tt] + w.Prepared[tt] + w.Assigned[tt] + w.Sched[tt]
}

func (w *WorkerInfo) canSwitch() bool {
	return w.sum("AP")+w.sum("PC1")+w.sum("PC2") == 0
}

func (w *WorkerInfo) canStop() bool {
	var all int
	for _, v := range w.Runing {
		all += v
	}
	for _, v := range w.Prepared {
		all += v
	}
	for _, v := range w.Assigned {
		all += v
	}
	for _, v := range w.Sched {
		all += v
	}

	return all+len(w.Sectors) == 0
}

func (p *Pilot) workerInfoAPI(ma address.Address) (wst, jobs, sts, SchedDiagInfo, error) {
	p.lk.RLock()
	defer p.lk.RUnlock()

	mi, ok := p.miners[ma]
	if !ok {
		return nil, nil, nil, SchedDiagInfo{}, fmt.Errorf("not found miner: %s", ma)
	}

	wst, err := mi.api.WorkerStats(p.ctx)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	jobs, err := mi.api.WorkerJobs(p.ctx)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	sts, err := mi.api.StorageList(p.ctx)
	if err != nil {
		return nil, nil, nil, SchedDiagInfo{}, err
	}

	if build.SkipSchedDiag {
		return wst, jobs, sts, SchedDiagInfo{}, nil
	}

	schedb, err := mi.api.SealingSchedDiag(p.ctx, false)
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

func (p *Pilot) getWorkerStats(ma address.Address) (map[uuid.UUID]storiface.WorkerStats, error) {
	cache, ok := p.statsCache[ma]
	if ok && time.Now().Before(cache.cacheTime.Add(CacheTimeout)) {
		log.Debugw("getWorkerStats", "cacheTime", cache.cacheTime, "miner", ma)
		return cache.worker, nil
	}

	worker, err := p.workerStats(ma)
	if err != nil {
		return nil, err
	}
	p.statsCache[ma] = workerStatsCache{
		worker:    worker,
		cacheTime: time.Now(),
	}

	return worker, nil
}

func (p *Pilot) getWorkerInfo(ma address.Address) (map[uuid.UUID]WorkerInfo, error) {
	cache, ok := p.infoCache[ma]
	if ok && time.Now().Before(cache.cacheTime.Add(CacheTimeout)) {
		log.Debugw("getWorkerInfo", "cacheTime", cache.cacheTime, "miner", ma)
		return cache.worker, nil
	}

	worker, err := p._getWorkerInfo(ma)
	if err != nil {
		return nil, err
	}
	p.infoCache[ma] = workerInfoCache{
		worker:    worker,
		cacheTime: time.Now(),
	}

	return worker, nil
}

func (p *Pilot) _getWorkerInfo(ma address.Address) (map[uuid.UUID]WorkerInfo, error) {
	wst, jobs, sts, diag, err := p.workerInfoAPI(ma)
	if err != nil {
		return nil, err
	}

	worker := map[uuid.UUID]WorkerInfo{}
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

		sectors := map[string]struct{}{}
		for _, d := range sts[id] {
			sectors[d.SectorID.Number.String()] = struct{}{}
			sectorWorker[d.SectorID] = wid
		}

		tasks := map[string]struct{}{}
		for _, t := range st.Tasks {
			tasks[t.Short()] = struct{}{}
		}

		worker[wid] = WorkerInfo{
			WorkerID:  wid,
			StorageID: id,
			Hostname:  st.Info.Hostname,
			Runing:    map[string]int{},
			Prepared:  map[string]int{},
			Assigned:  map[string]int{},
			LastStart: make(map[string]time.Time),
			Sched:     make(map[string]int),
			Sectors:   sectors,
			Tasks:     tasks,
		}
	}

	for wid, jobs := range jobs {
		for _, job := range jobs {
			if job.RunWait < 0 {
				continue
			}
			if _, ok := worker[wid]; !ok {
				log.Debugf("worker: %s not found", wid)
				continue
			}

			if job.RunWait == storiface.RWRunning {
				worker[wid].Runing[job.Task.Short()] += 1
				if worker[wid].LastStart[job.Task.Short()].Before(job.Start) {
					worker[wid].LastStart[job.Task.Short()] = job.Start
				}
			} else if job.RunWait == storiface.RWPrepared {
				worker[wid].Prepared[job.Task.Short()] += 1
			} else {
				//assigned
				worker[wid].Assigned[job.Task.Short()] += 1
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
			log.Debugf("worker: %s not found", wid)
			continue
		}
		worker[wid].Sched[req.TaskType.Short()] += 1
	}

	return worker, nil
}

func (p *Pilot) workerStats(ma address.Address) (wst, error) {
	p.lk.RLock()
	defer p.lk.RUnlock()

	mi, ok := p.miners[ma]
	if !ok {
		return nil, fmt.Errorf("not found miner: %s", ma)
	}

	wst, err := mi.api.WorkerStats(p.ctx)
	if err != nil {
		return nil, err
	}

	return wst, nil
}

func (p *Pilot) workerPick(req SwitchRequest) (map[uuid.UUID]*WorkerState, error) {
	switchingWorkers := p.switchingWorkers()
	out := map[uuid.UUID]*WorkerState{}

	if len(req.Worker) != 0 {
		//specify worker from requst
		wst, err := p.workerStats(req.From)
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

	if req.Count == 0 {
		//switch all worker
		wst, err := p.workerStats(req.From)
		if err != nil {
			return nil, err
		}
		for wid, st := range wst {
			if !workerCheck(st) {
				continue
			}
			if _, ok := switchingWorkers[wid]; ok {
				continue
			}
			out[wid] = &WorkerState{
				WorkerID: wid,
				Hostname: st.Info.Hostname,
				State:    StateWorkerPicked,
			}
		}
		return out, nil
	}

	worker, err := p._getWorkerInfo(req.From)
	if err != nil {
		return nil, err
	}

	if len(worker) < req.Count {
		return nil, fmt.Errorf("not enough worker. miner: %s has: %d need: %d", req.From, len(worker), req.Count)
	}

	var workerSort []WorkerInfo
	for _, w := range worker {
		//skip switchingWorkers
		if _, ok := switchingWorkers[w.WorkerID]; ok {
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

		if wi.LastStart["PC1"].Equal(wj.LastStart["PC1"]) {
			return wi.Hostname < wj.Hostname
		}

		return wi.LastStart["PC1"].Before(wj.LastStart["PC1"])
	})

	for _, w := range workerSort[0:req.Count] {
		out[w.WorkerID] = &WorkerState{
			WorkerID: w.WorkerID,
			Hostname: w.Hostname,
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

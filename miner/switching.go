package miner

import (
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/google/uuid"
)

type stateSwitch int

const (
	stateAccepted  stateSwitch = iota
	stateSwitching             //waiting to switch
	stateSwitched
	stateCanceled
	stateError
)

var stateSwitchNames = map[stateSwitch]string{
	stateAccepted:  "accepted",
	stateSwitching: "switching",
	stateSwitched:  "switched",
	stateCanceled:  "canceled",
	stateError:     "error",
}

func (s stateSwitch) String() string {
	return stateSwitchNames[s]
}

type switchID uuid.UUID

func (s switchID) String() string {
	return uuid.UUID(s).String()
}

type switchRequest struct {
	from  address.Address
	to    address.Address
	count int
	//指定要切换的worker列表，如果为空，则由pilot选择
	worker []uuid.UUID
	//切换前是否禁止AP任务，如果不禁止，则fromMiner的任务全部完成后再切到toMiner
	disableAP bool
}

type switchRequestResponse struct {
	rsp chan switchResponse
	req *switchRequest
}

type switchResponse struct {
	id     switchID
	worker map[uuid.UUID]*workerState //hostname
	err    error
}

type switchState struct {
	id     switchID
	state  stateSwitch
	errMsg string

	req    *switchRequest
	worker map[uuid.UUID]*workerState //workerID

	//to miner info
	size  abi.SectorSize
	token string

	cancel chan struct{}
}

func (m *Miner) sendSwitch(req *switchRequest) switchResponse {
	srr := switchRequestResponse{
		rsp: make(chan switchResponse),
		req: req,
	}

	m.ch <- srr

	return <-srr.rsp
}

func (m *Miner) run() {
	go func() {
		for {
			select {
			case srr := <-m.ch:
				go m.process(srr)
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

func (m *Miner) process(srr switchRequestResponse) {
	mi, err := m.getMiner(srr.req.to)
	if err != nil {
		srr.rsp <- switchResponse{err: err}
		return
	}

	worker, err := m.workerPick(srr.req)
	if err != nil {
		srr.rsp <- switchResponse{err: err}
		return
	}

	ss := &switchState{
		id:     switchID(uuid.New()),
		state:  stateAccepted,
		req:    srr.req,
		worker: worker,
		size:   mi.size,
		token:  mi.token,
		cancel: make(chan struct{}),
	}
	srr.rsp <- switchResponse{id: ss.id, worker: worker, err: nil}
	m.addSwitch(ss)

	if err := m.disableAP(ss.id); err != nil {
		log.Error(err)
		return
	}

	t := time.NewTicker(m.interval)
	for {
		select {
		case <-t.C:
			wi, err := m.getWorkerInfo(ss.req.from)
			if err != nil {
				log.Errorf("getWorkerInfo: %s", err)
				continue
			}
			complete, err := m.update(ss.id, wi)
			if err != nil {
				log.Errorf("update: %s", err)
				continue
			}
			if complete {
				log.Infof("switchID: %s complete", ss.id)
				return
			}
		case <-ss.cancel:
			log.Infof("switch ID: %s canceled", ss.id)
			return
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *Miner) disableAP(id switchID) error {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if !ok {
		return fmt.Errorf("switchID: %s not found", id)
	}

	for _, ws := range ss.worker {
		if ss.req.disableAP {
			err := disableAPCmd(m.ctx, ws.hostname, ss.req.from.String())
			if err != nil {
				log.Errorw("disable ap cmd", "err", err.Error())
				ws.try += 1
				ws.errMsg = err.Error()
				continue
			}
		}
		ws.state = stateWorkerSwitching
	}

	ss.state = stateSwitching
	return nil
}

func (m *Miner) update(id switchID, wi map[uuid.UUID]workerInfo) (bool, error) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if !ok {
		return false, fmt.Errorf("switchID: %s not found", id)
	}

	workerCompleted := 0
	for wid, ws := range ss.worker {
		switch ws.state {
		case stateWorkerPicked:
			if ss.req.disableAP {
				err := disableAPCmd(m.ctx, ws.hostname, ss.req.from.String())
				if err != nil {
					log.Errorf("disableAPCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
			}
			//TODO: check disableAP success or not
			ws.state = stateWorkerSwitching
		case stateWorkerSwitching:
			w, ok := wi[wid]
			if !ok {
				log.Errorf("not found workerID: %s", wid)
				continue
			}
			if w.canSwitch() {
				err := workerRunCmd(m.ctx, w.hostname, ss.req.to.String(), ss.token, ss.size)
				if err != nil {
					log.Errorf("workerRunCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
				//TODO: check worker run success or not
				ws.state = stateWorkerSwithed
			}
		case stateWorkerSwithed:
			w, ok := wi[wid]
			if !ok {
				log.Errorf("not found workerID: %s", wid)
				continue
			}
			if w.canStop() {
				err := workerStopCmd(m.ctx, ws.hostname, ss.req.from.String())
				if err != nil {
					log.Errorf("workerStopCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
				//TODO: check worker stop success or not
				ws.state = stateWorkerStoped
			}
		case stateWorkerStoped:
			fallthrough
		case stateWorkerError:
			workerCompleted += 1
		default:
			log.Warnw("switch state", "id", id, "workerID", ws.workerID, "worker state", ws.state)
		}
	}

	if workerCompleted == len(ss.worker) {
		ss.state = stateSwitched
		return true, nil
	}

	return false, nil
}

func (m *Miner) addSwitch(ss *switchState) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	m.switchs[ss.id] = ss
}

func (m *Miner) cancelSwitch(id switchID) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if ok {
		ss.state = stateCanceled
		close(ss.cancel)
	}
}

func (m *Miner) removeSwitch(id switchID) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	delete(m.switchs, id)
}

func (m *Miner) getSwitch(id switchID) SwitchState {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	ss, ok := m.switchs[id]
	if !ok {
		return SwitchState{}
	}

	worker := []string{}
	for _, w := range ss.req.worker {
		worker = append(worker, w.String())
	}
	req := SwitchRequest{
		From:      ss.req.from.String(),
		To:        ss.req.to.String(),
		Count:     ss.req.count,
		Worker:    worker,
		DisableAP: ss.req.disableAP,
	}

	ws := []WorkerState{}
	for _, w := range ss.worker {
		ws = append(ws, WorkerState{
			WorkerID: w.workerID.String(),
			Hostname: w.hostname,
			State:    w.state.String(),
			ErrMsg:   w.errMsg,
			Try:      w.try,
		})
	}

	ret := SwitchState{
		ID:     ss.id.String(),
		State:  ss.state.String(),
		ErrMsg: ss.errMsg,
		Req:    req,
		Worker: ws,
	}

	return ret
}

func (m *Miner) listSwitch() []string {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	var out []string
	for _, s := range m.switchs {
		out = append(out, s.id.String())
	}

	return out
}

func (m *Miner) switchingWorkers() map[uuid.UUID]struct{} {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	out := map[uuid.UUID]struct{}{}

	for _, s := range m.switchs {
		for w, ws := range s.worker {
			if ws.state != stateWorkerStoped {
				out[w] = struct{}{}
			}
		}
	}

	return out
}

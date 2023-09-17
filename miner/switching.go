package miner

import (
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

	stateError
)

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

	m.disableAP(ss.id)

	t := time.NewTicker(time.Minute * 5)
	for {
		select {
		case <-t.C:
			wi, err := m.getWorkerInfo(ss.req.from)
			if err != nil {
				log.Errorf("getWorkerInfo: %s", err)
				continue
			}
			m.update(ss.id, wi)
		case <-ss.cancel:
			//TODO: cancel switch
		case <-m.ctx.Done():
			return
		}
	}

	//switch complete

}

func (m *Miner) disableAP(id switchID) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if !ok {
		log.Errorw("switchID not found", "id", id)
		return
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
}

func (m *Miner) update(id switchID, wi map[uuid.UUID]workerInfo) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if !ok {
		log.Errorf("switchID not found: %s", id)
		return
	}

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
			ws.state = stateWorkerSwitching
		case stateWorkerSwitching:
			w, ok := wi[wid]
			if !ok {
				log.Errorf("not found workerID: %s", wid)
				continue
			}
			if w.canSwitch() {
				err := workerRunCmd(m.ctx, w.hostname, ss.req.from.String(), ss.token, ss.size)
				if err != nil {
					log.Errorf("workerRunCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
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
				ws.state = stateWorkerStoped
			}
		default:
			log.Debugw("switch state", "id", id, "workerID", ws.workerID, "worker state", ws.state)
		}

	}

}

func (m *Miner) addSwitch(ss *switchState) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	m.switchs[ss.id] = ss
}

func (m *Miner) cancelSwitch(id switchID) {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	ss, ok := m.switchs[id]
	if ok {
		close(ss.cancel)
	}
}

func (m *Miner) removeSwitch(id switchID) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	delete(m.switchs, id)
}

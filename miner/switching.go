package miner

import (
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/google/uuid"
)

type stateSwitch int

const (
	stateAccepted stateSwitch = iota
	stateAPDisabled
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

	cancel chan struct{}
}

func newSwitchState(req *switchRequest, worker map[uuid.UUID]*workerState) *switchState {
	return &switchState{
		id:     switchID(uuid.New()),
		state:  stateAccepted,
		req:    req,
		worker: worker,
		cancel: make(chan struct{}),
	}
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

func (m *Miner) process(srr switchRequestResponse) error {
	worker, err := m.workerPick(srr.req)
	if err != nil {
		srr.rsp <- switchResponse{err: err}
		return err
	}

	ss := newSwitchState(srr.req, worker)
	srr.rsp <- switchResponse{id: ss.id, worker: worker, err: nil}
	m.addSwitch(ss)

	//disableAP
	if srr.req.disableAP {
		m.disableAP(ss.id)
	}

	m.watch(ss)

	//switch complete

	return nil
}

func (m *Miner) disableAP(id switchID) error {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if !ok {
		return fmt.Errorf("switch id: %s not found", id)
	}

	for _, ws := range ss.worker {
		err := disableAPCmd(m.ctx, ws.hostname, ss.req.from.String())
		if err != nil {
			continue
		}
		ws.state = stateWorkerAPDisabled
	}
	return nil
}

func (m *Miner) addSwitch(ss *switchState) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	m.switchs[ss.id] = ss
}

func (m *Miner) updateErr(id switchID, err string) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if ok {
		ss.state = stateError
		ss.errMsg = err
	}
}

func (m *Miner) watch(ss *switchState) {
	t := time.NewTicker(time.Minute * 5)
	for {
		select {
		case <-t.C:
			wl := m.disabledWorker(ss.id)
			if len(wl) == 0 {
				log.Info("no switch worker to found")
				return
			}
			m.workerSwitch(ss.req, wl)
		case <-ss.cancel:

		case <-m.ctx.Done():
			return
		}
	}
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

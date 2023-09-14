package miner

import (
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/google/uuid"
)

type stateSwitch int

const (
	stateRequst stateSwitch = iota
	statePickWorker
	stateDisabledAP
	stateSwitching
	stateSwitchFinish

	stateError
)

type SwitchID uuid.UUID

func (s SwitchID) String() string {
	return uuid.UUID(s).String()
}

type switchRequest struct {
	id    SwitchID
	from  address.Address
	to    address.Address
	count int
}

type switchState struct {
	state  stateSwitch
	errMsg string

	req    switchRequest
	worker map[uuid.UUID]*workerState //workerID

	cancel chan struct{}
}

func (m *Miner) run() {
	go func() {
		for {
			select {
			case req := <-m.ch:
				go m.process(req)
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

func (m *Miner) process(req switchRequest) error {
	ss := &switchState{
		state:  stateRequst,
		req:    req,
		worker: make(map[uuid.UUID]*workerState),
		cancel: make(chan struct{}),
	}
	m.update(ss)

	worker, err := m.workerPick(req)
	if err != nil {
		m.updateErr(ss.req.id, err.Error())
		return err
	}
	ss.state = statePickWorker
	ss.worker = worker
	m.update(ss)

	//disableAP(worker)
	ss.state = stateDisabledAP
	m.update(ss)
	//update worker state: stateWorkerDisabledAP

	ss.state = stateSwitching
	m.update(ss)
	m.watch(ss)

	ss.state = stateSwitchFinish
	m.update(ss)

	return nil
}

func (m *Miner) update(ss *switchState) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	m.switchs[ss.req.id] = ss
}

func (m *Miner) updateErr(id SwitchID, err string) {
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
			wl := m.disabledWorker(ss.req.id)
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

func (m *Miner) cancelSwitch(id SwitchID) {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	ss, ok := m.switchs[id]
	if ok {
		close(ss.cancel)
	}
}

func (m *Miner) removeSwitch(id SwitchID) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	delete(m.switchs, id)
}

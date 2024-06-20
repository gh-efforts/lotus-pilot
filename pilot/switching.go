package pilot

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/google/uuid"
)

type StateSwitch int

const (
	StateSwitching StateSwitch = iota
	StateComplete
	StateCanceled
	StateError
)

var stateSwitchNames = map[StateSwitch]string{
	StateSwitching: "switching",
	StateComplete:  "complete",
	StateCanceled:  "canceled",
	StateError:     "error",
}

func (s StateSwitch) String() string {
	return stateSwitchNames[s]
}

type SwitchRequest struct {
	From address.Address `json:"from"`
	To   address.Address `json:"to"`
	//如果Count为0，则切换所有worker
	Count int `json:"count"`
	//指定要切换的worker列表，如果为空，则由pilot选择
	Worker []uuid.UUID `json:"worker"`
	//切换前是否禁止AP任务，如果不禁止，则fromMiner的任务全部完成后再切到toMiner
	DisableAP bool `json:"disableAP"`
}

type SwitchState struct {
	ID     uuid.UUID                  `json:"id"`
	State  StateSwitch                `json:"state"`
	ErrMsg string                     `json:"errMsg"`
	Req    SwitchRequest              `json:"req"`
	Worker map[uuid.UUID]*WorkerState `json:"worker"`
}

func (s *SwitchState) update(m *Pilot) {
	var wg sync.WaitGroup
	throttle := make(chan struct{}, m.parallel)

	for wid, ws := range s.Worker {
		//skip complete or error
		if ws.State == StateWorkerComplete || ws.State == StateWorkerError {
			continue
		}

		wg.Add(1)
		throttle <- struct{}{}
		go func(wid uuid.UUID, ws *WorkerState) {
			defer wg.Done()
			defer func() {
				<-throttle
			}()
			//for{}
			switch ws.State {
			case StateWorkerPicked:
				if s.Req.DisableAP {
					err := disableAPCmd(m.ctx, ws.Hostname, s.Req.From.String())
					if err != nil {
						log.Errorw("disableAPCmd", "switchID", s.ID, "workerID", wid, "err", err.Error())
						ws.updateErr(err.Error())
						return
					}
					log.Debugw("disableAPCmd to confirming", "switchID", s.ID, "workerID", wid)
					ws.State = StateWorkerDisableAPConfirming
				} else {
					log.Debugw("no need disableAP to switching", "switchID", s.ID, "workerID", wid)
					ws.State = StateWorkerSwitchWaiting
				}
			case StateWorkerDisableAPConfirming:
				worker, err := m.getWorkerStats(s.Req.From)
				if err != nil {
					log.Errorw("getWorkerStats", "wid", wid, "from", s.Req.From, "err", err)
					return
				}
				w, ok := worker[wid]
				if !ok {
					errMsg := fmt.Sprintf("not found workerID: %s", wid)
					log.Error(errMsg)
					ws.updateErr(errMsg)
					return
				}

				for _, t := range w.Tasks {
					if t == sealtasks.TTAddPiece {
						errMsg := fmt.Sprintf("DisableAPConfirming still has AP task: %s", wid)
						log.Error(errMsg)
						ws.updateErr(errMsg)
						return
					}
				}

				log.Infow("disableAP success", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname)
				ws.State = StateWorkerSwitchWaiting
			case StateWorkerSwitchWaiting:
				worker, err := m.getWorkerInfo(s.Req.From)
				if err != nil {
					log.Errorw("getWorkerInfo", "wid", wid, "from", s.Req.From, "err", err)
					return
				}
				w, ok := worker[wid]
				if !ok {
					errMsg := fmt.Sprintf("not found workerID: %s", wid)
					log.Error(errMsg)
					ws.updateErr(errMsg)
					return
				}
				if !w.canSwitch() {
					log.Debugw("Switching conditions not met", "switchID", s.ID, "workerID", ws.WorkerID)
					return
				}

				err = workerRunCmd(m.ctx, w.Hostname, s.Req.To.String(), m.repo.ScriptsPath())
				if err != nil {
					log.Errorw("workerRunCmd", "switchID", s.ID, "wid", wid, "to", s.Req.To, "err", err.Error())
					ws.updateErr(err.Error())
					return
				}

				log.Debugw("workerRunCmd sunccess", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname, "to", s.Req.To)
				ws.State = StateWorkerSwitchConfirming
			case StateWorkerSwitchConfirming:
				worker, err := m.getWorkerStats(s.Req.To)
				if err != nil {
					log.Errorw("getWorkerStats", "wid", wid, "from", s.Req.From, "err", err)
					return
				}
				has := false
				for _, w := range worker {
					if w.Info.Hostname == ws.Hostname {
						has = true
						break
					}
				}
				if !has {
					errMsg := fmt.Sprintf("worker: %s not found in miner: %s", ws.Hostname, s.Req.To)
					log.Error(errMsg)
					ws.updateErr(errMsg)
					return
				}
				log.Infow("switch success", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname, "to", s.Req.To)
				ws.State = StateWorkerStopWaiting
			case StateWorkerStopWaiting:
				worker, err := m.getWorkerInfo(s.Req.From)
				if err != nil {
					log.Errorw("getWorkerInfo", "wid", wid, "from", s.Req.From, "err", err)
					return
				}
				w, ok := worker[wid]
				if !ok {
					errMsg := fmt.Sprintf("not found workerID: %s", wid)
					log.Error(errMsg)
					ws.updateErr(errMsg)
					return
				}
				if !w.canStop() {
					log.Debugw("Stoping conditions not met", "switchID", s.ID, "workerID", ws.WorkerID)
					return
				}
				err = workerStopCmd(m.ctx, ws.Hostname, s.Req.From.String())
				if err != nil {
					log.Errorw("workerStopCmd", "wid", wid, "from", s.Req.From, "err", err.Error())
					ws.updateErr(err.Error())
					return
				}
				log.Debugw("workerStopCmd", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname, "from", s.Req.From)
				ws.State = StateWorkerStopConfirming
			case StateWorkerStopConfirming:
				worker, err := m.getWorkerStats(s.Req.From)
				if err != nil {
					log.Errorw("getWorkerStats", "wid", wid, "from", s.Req.From, "err", err)
					return
				}
				if _, ok := worker[wid]; ok {
					errMsg := fmt.Sprintf("worker: %s still in miner: %s", wid, s.Req.From)
					log.Error(errMsg)
					ws.updateErr(errMsg)
					return
				}
				log.Infow("stop success", "switchID", s.ID, "wid", ws.WorkerID, "hostname", ws.Hostname)
				ws.State = StateWorkerComplete
			case StateWorkerComplete:
			case StateWorkerError:
			default:
				log.Warnw("unknown worker state", "switchID", s.ID, "wid", ws.WorkerID, "worker state", ws.State)
			}
		}(wid, ws)
	}
	wg.Wait()

	workerCompleted := 0
	workerError := 0
	for _, ws := range s.Worker {
		if ws.State == StateWorkerComplete {
			workerCompleted += 1
		}
		if ws.State == StateWorkerError {
			workerError += 1
		}
	}

	if workerCompleted+workerError == len(s.Worker) {
		if workerError != 0 {
			s.State = StateError
			log.Infof("switchID: %s error", s.ID)
		} else {
			s.State = StateComplete
			log.Infof("switchID: %s complete", s.ID)
		}
	}
}

func (p *Pilot) newSwitch(req SwitchRequest) (*SwitchState, error) {
	worker, err := p.workerPick(req)
	if err != nil {
		return nil, err
	}

	ss := &SwitchState{
		ID:     uuid.New(),
		State:  StateSwitching,
		Req:    req,
		Worker: worker,
	}

	err = p.addSwitch(ss)
	if err != nil {
		return nil, err
	}

	log.Infof("new switch: %s", ss.ID)
	return ss, nil
}

func (p *Pilot) process() {
	p.swLk.Lock()
	defer p.swLk.Unlock()

	needWrite := false
	for _, ss := range p.switchs {
		if ss.State != StateSwitching {
			continue
		}

		ss.update(p)
		needWrite = true
	}

	if needWrite {
		err := p.writeSwitch()
		if err != nil {
			log.Error(err)
		}
	}
}

// write switchs state to repo/state
// caller need keep swLk lock
func (p *Pilot) writeSwitch() error {
	data, err := json.Marshal(p.switchs)
	if err != nil {
		return err
	}

	err = p.repo.WriteSwitchState(data)
	if err != nil {
		return err
	}
	log.Debug("writeSwitch")
	return nil
}

func (p *Pilot) addSwitch(ss *SwitchState) error {
	p.swLk.Lock()
	defer p.swLk.Unlock()

	p.switchs[ss.ID] = ss

	return p.writeSwitch()
}

func (p *Pilot) cancelSwitch(id uuid.UUID) error {
	p.swLk.Lock()
	defer p.swLk.Unlock()

	ss, ok := p.switchs[id]
	if !ok {
		return fmt.Errorf("switchID: %s not found", id)
	}
	if ss.State != StateSwitching {
		return fmt.Errorf("switch state: %s can not cancel", ss.State)
	}

	ss.State = StateCanceled
	log.Infof("switch: %s canceled", ss.ID)

	return p.writeSwitch()
}

func (p *Pilot) removeSwitch(id uuid.UUID) error {
	p.swLk.Lock()
	defer p.swLk.Unlock()

	delete(p.switchs, id)
	log.Infof("switch: %s deleted", id)

	return p.writeSwitch()
}

func (p *Pilot) getSwitch(id uuid.UUID) *SwitchState {
	p.swLk.RLock()
	defer p.swLk.RUnlock()

	return p.switchs[id]
}

func (p *Pilot) listSwitch() []string {
	p.swLk.RLock()
	defer p.swLk.RUnlock()

	var out []string
	for _, s := range p.switchs {
		out = append(out, s.ID.String())
	}

	return out
}

func (p *Pilot) switchingWorkers() map[uuid.UUID]struct{} {
	p.swLk.RLock()
	defer p.swLk.RUnlock()

	out := map[uuid.UUID]struct{}{}

	for _, s := range p.switchs {
		for w, ws := range s.Worker {
			if ws.State != StateWorkerComplete {
				out[w] = struct{}{}
			}
		}
	}

	return out
}

func (p *Pilot) resumeSwitch(id uuid.UUID) error {
	p.swLk.Lock()
	defer p.swLk.Unlock()

	ss, ok := p.switchs[id]
	if !ok {
		return fmt.Errorf("switchID: %s not found", id)
	}
	if ss.State == StateSwitching || ss.State == StateComplete {
		return fmt.Errorf("switch state: %s can not resume", ss.State)
	}

	ss.State = StateSwitching
	log.Infof("switch: %s resumed", ss.ID)

	return p.writeSwitch()
}

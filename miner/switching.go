package miner

import (
	"context"
	"encoding/json"

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

func (s *SwitchState) disableAP(ctx context.Context) {
	for _, ws := range s.Worker {
		if s.Req.DisableAP {
			err := disableAPCmd(ctx, ws.Hostname, s.Req.From.String())
			if err != nil {
				log.Errorf("disableAPCmd", err.Error())
				ws.updateErr(err.Error())
				continue
			}
			ws.State = StateWorkerDisableAPConfirming
		} else {
			ws.State = StateWorkerSwitchWaiting
		}
	}
}

func (s *SwitchState) update(m *Miner) {
	workerCompleted := 0
	for wid, ws := range s.Worker {
		switch ws.State {
		case StateWorkerPicked:
			if s.Req.DisableAP {
				err := disableAPCmd(m.ctx, ws.Hostname, s.Req.From.String())
				if err != nil {
					log.Errorf("disableAPCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
				ws.State = StateWorkerDisableAPConfirming
			} else {
				ws.State = StateWorkerSwitchWaiting
			}
		case StateWorkerDisableAPConfirming:
			worker, err := m.getWorkerStats(s.Req.From)
			if err != nil {
				log.Errorf("miner: %s getWorkerStats: %s", s.Req.From, err)
				continue
			}
			w, ok := worker[wid]
			if !ok {
				log.Errorf("not found workerID: %s", wid)
				continue
			}
			hasAP := false
			for _, t := range w.Tasks {
				if t == sealtasks.TTAddPiece {
					hasAP = true
				}
			}
			if hasAP {
				log.Infow("disableAP retry", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname)
				err := disableAPCmd(m.ctx, ws.Hostname, s.Req.From.String())
				if err != nil {
					log.Errorf("disableAPCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
			} else {
				log.Infow("disableAP success", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname)
				ws.State = StateWorkerSwitchWaiting
			}
		case StateWorkerSwitchWaiting:
			worker, err := m.getWorkerInfo(s.Req.From)
			if err != nil {
				log.Errorf("miner: %s getWorkerInfo: %s", s.Req.From, err)
				continue
			}
			w, ok := worker[wid]
			if !ok {
				log.Errorf("not found workerID: %s", wid)
				continue
			}
			if w.canSwitch() {
				err := workerRunCmd(m.ctx, w.hostname, s.Req.To.String(), m.repo.ScriptsPath())
				if err != nil {
					log.Errorf("workerRunCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
				ws.State = StateWorkerSwitchConfirming
			}
		case StateWorkerSwitchConfirming:
			worker, err := m.getWorkerStats(s.Req.To)
			if err != nil {
				log.Errorf("miner: %s getWorkerStats: %s", s.Req.From, err)
				continue
			}
			has := false
			for _, w := range worker {
				if w.Info.Hostname == ws.Hostname {
					has = true
					break
				}
			}
			if has {
				log.Infow("switch success", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname)
				ws.State = StateWorkerStopWaiting
			} else {
				log.Warnw("switch failed", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname)
				//TODO: re-switch
			}
		case StateWorkerStopWaiting:
			worker, err := m.getWorkerInfo(s.Req.From)
			if err != nil {
				log.Errorf("miner: %s getWorkerInfo: %s", s.Req.From, err)
				continue
			}
			w, ok := worker[wid]
			if !ok {
				log.Errorf("not found workerID: %s", wid)
				continue
			}
			if w.canStop() {
				err := workerStopCmd(m.ctx, ws.Hostname, s.Req.From.String())
				if err != nil {
					log.Errorf("workerStopCmd", err.Error())
					ws.updateErr(err.Error())
					continue
				}
				ws.State = StateWorkerStopConfirming
			}
		case StateWorkerStopConfirming:
			worker, err := m.getWorkerStats(s.Req.From)
			if err != nil {
				log.Errorf("miner: %s getWorkerStats: %s", s.Req.From, err)
				continue
			}
			if _, ok := worker[wid]; ok {
				log.Infow("stop retry", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname)
				err := workerStopCmd(m.ctx, ws.Hostname, s.Req.From.String())
				if err != nil {
					log.Errorf("workerStopCmd", err.Error())
					ws.updateErr(err.Error())
				}
			} else {
				log.Infow("stop success", "switchID", s.ID, "workerID", ws.WorkerID, "hostname", ws.Hostname)
				ws.State = StateWorkerComplete
			}
		case StateWorkerComplete:
			fallthrough
		case StateWorkerError:
			workerCompleted += 1
		default:
			log.Warnw("switch state", "id", s.ID, "workerID", ws.WorkerID, "worker state", ws.State)
		}
	}

	if workerCompleted == len(s.Worker) {
		s.State = StateComplete
		log.Infof("switchID: %s complete", s.ID)
	}
}

func (m *Miner) newSwitch(ctx context.Context, req SwitchRequest) (*SwitchState, error) {
	worker, err := m.workerPick(req)
	if err != nil {
		return nil, err
	}

	ss := &SwitchState{
		ID:     uuid.New(),
		State:  StateSwitching,
		Req:    req,
		Worker: worker,
	}

	ss.disableAP(ctx)

	err = m.addSwitch(ss)
	if err != nil {
		return nil, err
	}

	log.Infof("new switch: %s", ss.ID)
	return ss, nil
}

func (m *Miner) process() {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	for _, ss := range m.switchs {
		if ss.State == StateCanceled || ss.State == StateComplete {
			continue
		}

		ss.update(m)
	}

	err := m.writeSwitch()
	if err != nil {
		log.Error(err)
	}
}

// write switchs state to repo/state
// caller need keep swLk lock
func (m *Miner) writeSwitch() error {
	data, err := json.Marshal(m.switchs)
	if err != nil {
		return err
	}

	err = m.repo.WriteSwitchState(data)
	if err != nil {
		return err
	}

	return nil
}

func (m *Miner) addSwitch(ss *SwitchState) error {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	m.switchs[ss.ID] = ss

	return m.writeSwitch()
}

func (m *Miner) cancelSwitch(id uuid.UUID) error {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if ok && ss.State == StateSwitching {
		ss.State = StateCanceled
		log.Infof("switch: %s canceled", ss.ID)
	}

	return m.writeSwitch()
}

func (m *Miner) removeSwitch(id uuid.UUID) error {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	delete(m.switchs, id)
	log.Infof("switch: %s deleted", id)

	return m.writeSwitch()
}

func (m *Miner) getSwitch(id uuid.UUID) *SwitchState {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	return m.switchs[id]
}

func (m *Miner) listSwitch() []string {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	var out []string
	for _, s := range m.switchs {
		out = append(out, s.ID.String())
	}

	return out
}

func (m *Miner) switchingWorkers() map[uuid.UUID]struct{} {
	m.swLk.RLock()
	defer m.swLk.RUnlock()

	out := map[uuid.UUID]struct{}{}

	for _, s := range m.switchs {
		for w, ws := range s.Worker {
			if ws.State != StateWorkerComplete {
				out[w] = struct{}{}
			}
		}
	}

	return out
}

package miner

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/google/uuid"
)

type StateSwitch int

const (
	StateAccepted  StateSwitch = iota
	StateSwitching             //waiting to switch
	StateSwitched
	StateCanceled
	StateError
)

var stateSwitchNames = map[StateSwitch]string{
	StateAccepted:  "accepted",
	StateSwitching: "switching",
	StateSwitched:  "switched",
	StateCanceled:  "canceled",
	StateError:     "error",
}

func (s StateSwitch) String() string {
	return stateSwitchNames[s]
}

type SwitchRequest struct {
	From  address.Address `json:"from"`
	To    address.Address `json:"to"`
	Count int             `json:"count"`
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
	if !s.Req.DisableAP {
		return
	}

	for _, ws := range s.Worker {
		err := disableAPCmd(ctx, ws.Hostname, s.Req.From.String())
		if err != nil {
			log.Errorw("disable ap cmd", "err", err.Error())
			ws.Try += 1
			ws.ErrMsg = err.Error()
			continue
		}

		ws.State = StateWorkerSwitching
	}

	s.State = StateSwitching
}

func (s *SwitchState) update(m *Miner, fromWorker, toWorker map[uuid.UUID]workerInfo) {
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
			}
			//TODO: check disableAP success or not
			ws.State = StateWorkerSwitching
		case StateWorkerSwitching:
			w, ok := fromWorker[wid]
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
				//TODO: check worker run success or not
				ws.State = StateWorkerSwithed
			}
		case StateWorkerSwithed:
			w, ok := fromWorker[wid]
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
				//TODO: check worker stop success or not
				ws.State = StateWorkerStoped
			}
		case StateWorkerStoped:
			fallthrough
		case StateWorkerError:
			workerCompleted += 1
		default:
			log.Warnw("switch state", "id", s.ID, "workerID", ws.WorkerID, "worker state", ws.State)
		}
	}

	if workerCompleted == len(s.Worker) {
		s.State = StateSwitched
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
		State:  StateAccepted,
		Req:    req,
		Worker: worker,
	}

	ss.disableAP(ctx)

	m.addSwitch(ss)
	return ss, nil
}

func (m *Miner) process() {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	for _, ss := range m.switchs {
		if ss.State == StateCanceled {
			continue
		}

		from, err := m.getWorkerInfo(ss.Req.From)
		if err != nil {
			log.Errorf("getWorkerInfo: %s", err)
		}

		to, err := m.getWorkerInfo(ss.Req.To)
		if err != nil {
			log.Errorf("getWorkerInfo: %s", err)
		}

		ss.update(m, from, to)
	}
}

func (m *Miner) addSwitch(ss *SwitchState) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	m.switchs[ss.ID] = ss
}

func (m *Miner) cancelSwitch(id uuid.UUID) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	ss, ok := m.switchs[id]
	if ok {
		ss.State = StateCanceled
	}
}

func (m *Miner) removeSwitch(id uuid.UUID) {
	m.swLk.Lock()
	defer m.swLk.Unlock()

	delete(m.switchs, id)
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
			if ws.State != StateWorkerStoped {
				out[w] = struct{}{}
			}
		}
	}

	return out
}

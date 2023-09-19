package miner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/google/uuid"
)

type SwitchRequest struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Count     int      `json:"count"`
	Worker    []string `json:"worker"`
	DisableAP bool     `json:"disableAP"`
}

type Worker struct {
	WorkerID string `json:"workerID"`
	Hostname string `json:"hostname"`
}
type SwitchResponse struct {
	ID     string   `json:"id"`
	Worker []Worker `json:"worker"`
}

type SwitchState struct {
	ID     string        `json:"id"`
	State  string        `json:"state"`
	ErrMsg string        `json:"errMsg"`
	Req    SwitchRequest `json:"req"`
	Worker []WorkerState `json:"worker"`
}

type WorkerState struct {
	WorkerID string `json:"workerID"`
	Hostname string `json:"hostname"`
	State    string `json:"state"`
	ErrMsg   string `json:"errMsg"`
	Try      int    `json:"try"`
}

func (m *Miner) Handle() {
	http.HandleFunc("/miner/add", middlewareTimer(m.addHandle))
	http.HandleFunc("/miner/remove/", middlewareTimer(m.removeHandle))
	http.HandleFunc("/miner/list", middlewareTimer(m.listHandle))

	http.HandleFunc("/switch/new", middlewareTimer(m.switchHandle))
	http.HandleFunc("/switch/get/", middlewareTimer(m.getSwitchHandle))
	http.HandleFunc("/switch/cancel/", middlewareTimer(m.cancelSwitchHandle))
	http.HandleFunc("/switch/remove/", middlewareTimer(m.removeSwitchHandle))
	http.HandleFunc("/switch/list", middlewareTimer(m.listSwitchHandle))

	http.HandleFunc("/script/create/", middlewareTimer(m.createScriptHandle))

	//TODO: worker list
}

func (m *Miner) addHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("addHandle", "path", r.URL.Path)
	var minerAPI MinerAPI
	err := json.NewDecoder(r.Body).Decode(&minerAPI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	maddr, err := address.NewFromString(minerAPI.Miner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if m.has(maddr) {
		http.Error(w, "miner already has", http.StatusBadRequest)
		return
	}

	mi, err := toMinerInfo(r.Context(), minerAPI.Miner, minerAPI.API)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = createScript(mi.address.String(), mi.token, mi.size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.add(mi)
}

func (m *Miner) removeHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("removeHandle", "path", r.URL.Path)
	id := strings.TrimPrefix(r.URL.Path, "/miner/remove/")
	maddr, err := address.NewFromString(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.remove(maddr)

	err = os.Remove(fmt.Sprintf("%s/%s.sh", SCRIPTS_PATH, maddr.String()))
	if err != nil {
		log.Error(err)
	}
}

func (m *Miner) listHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("listHandle", "path", r.URL.Path)

	w.Write([]byte(fmt.Sprintf("%s", m.list())))
}

func (m *Miner) switchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("switchHandle", "path", r.URL.Path)
	var sr SwitchRequest
	err := json.NewDecoder(r.Body).Decode(&sr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	from, err := address.NewFromString(sr.From)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	to, err := address.NewFromString(sr.To)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	worker := []uuid.UUID{}
	for _, ww := range sr.Worker {
		i, err := uuid.Parse(ww)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		worker = append(worker, i)
	}

	req := &switchRequest{
		from:      from,
		to:        to,
		count:     sr.Count,
		disableAP: sr.DisableAP,
		worker:    worker,
	}

	rsp := m.sendSwitch(req)
	if rsp.err != nil {
		http.Error(w, rsp.err.Error(), http.StatusInternalServerError)
		return
	}

	ws := []Worker{}
	for id, s := range rsp.worker {
		ws = append(ws, Worker{
			WorkerID: id.String(),
			Hostname: s.hostname,
		})

	}
	srsp := SwitchResponse{
		ID:     rsp.id.String(),
		Worker: ws,
	}
	body, err := json.Marshal(srsp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (m *Miner) getSwitchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("getSwitchHandle", "path", r.URL.Path)
	s := strings.TrimPrefix(r.URL.Path, "/switch/get/")
	id, err := uuid.Parse(s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ss := m.getSwitch(switchID(id))
	body, err := json.Marshal(&ss)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (m *Miner) cancelSwitchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("cancelSwitchHandle", "path", r.URL.Path)
	s := strings.TrimPrefix(r.URL.Path, "/switch/cancel/")
	id, err := uuid.Parse(s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.cancelSwitch(switchID(id))
}

func (m *Miner) removeSwitchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("removeSwitchHandle", "path", r.URL.Path)
	s := strings.TrimPrefix(r.URL.Path, "/switch/remove/")
	id, err := uuid.Parse(s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.removeSwitch(switchID(id))
}

func (m *Miner) listSwitchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("listSwitchHandle", "path", r.URL.Path)

	w.Write([]byte(fmt.Sprintf("%s", m.listSwitch())))
}

func (m *Miner) createScriptHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("createScriptHandle", "path", r.URL.Path)
	id := strings.TrimPrefix(r.URL.Path, "/script/create/")
	err := m.createScript(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

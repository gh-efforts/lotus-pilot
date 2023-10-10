package miner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/gh-efforts/lotus-pilot/repo/config"
	"github.com/google/uuid"
)

type MinerAPI struct {
	Miner string         `json:"miner"`
	API   config.APIInfo `json:"api"`
}

func (m *Miner) Handle() {
	http.HandleFunc("/miner/add", middlewareTimer(m.addHandle))
	http.HandleFunc("/miner/remove/", middlewareTimer(m.removeHandle))
	http.HandleFunc("/miner/list", middlewareTimer(m.listHandle))
	http.HandleFunc("/miner/worker/", middlewareTimer(m.workerHandle))

	http.HandleFunc("/switch/new", middlewareTimer(m.switchHandle))
	http.HandleFunc("/switch/get/", middlewareTimer(m.getSwitchHandle))
	http.HandleFunc("/switch/cancel/", middlewareTimer(m.cancelSwitchHandle))
	http.HandleFunc("/switch/remove/", middlewareTimer(m.removeSwitchHandle))
	http.HandleFunc("/switch/list", middlewareTimer(m.listSwitchHandle))

	http.HandleFunc("/script/create/", middlewareTimer(m.createScriptHandle))
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

	mi, err := toMinerInfo(m.ctx, minerAPI.Miner, minerAPI.API)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = m.repo.CreateScript(mi.address, mi.token, mi.size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = m.repo.UpdateConfig(mi.address.String(), minerAPI.API)
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

	if !m.has(maddr) {
		http.Error(w, "miner not found", http.StatusBadRequest)
		return
	}

	err = m.repo.UpdateConfig(maddr.String(), config.APIInfo{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = m.repo.RemoveScript(maddr.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.remove(maddr)
}

func (m *Miner) listHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("listHandle", "path", r.URL.Path)

	w.Write([]byte(fmt.Sprintf("%s", m.list())))
}

func (m *Miner) workerHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("workerHandle", "path", r.URL.Path)

	id := strings.TrimPrefix(r.URL.Path, "/miner/worker/")
	maddr, err := address.NewFromString(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !m.has(maddr) {
		http.Error(w, "miner not found", http.StatusBadRequest)
		return
	}

	wi, err := m.getWorkerInfo(maddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(&wi)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (m *Miner) switchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("switchHandle", "path", r.URL.Path)
	var req SwitchRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ss, err := m.newSwitch(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(ss)
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

	body, err := json.Marshal(m.getSwitch(id))
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

	err = m.cancelSwitch(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (m *Miner) removeSwitchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("removeSwitchHandle", "path", r.URL.Path)
	s := strings.TrimPrefix(r.URL.Path, "/switch/remove/")
	id, err := uuid.Parse(s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = m.removeSwitch(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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

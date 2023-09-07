package miner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/gh-efforts/lotus-pilot/config"
)

type MinerAPI struct {
	Miner string         `json:"miner"`
	API   config.APIInfo `json:"api"`
}

func (m *Miner) Handle() {
	http.HandleFunc("/miner/add", middlewareTimer(m.addHandle))
	http.HandleFunc("/miner/remove/", middlewareTimer(m.removeHandle))
	http.HandleFunc("/miner/list", middlewareTimer(m.listHandle))
	http.HandleFunc("/miner/switch/", middlewareTimer(m.switchHandle))
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
}

func (m *Miner) listHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("listHandle", "path", r.URL.Path)

	w.Write([]byte(fmt.Sprintf("%s", m.list())))
}

func (m *Miner) switchHandle(w http.ResponseWriter, r *http.Request) {
	log.Debugw("switchHandle", "path", r.URL.Path)
	ss := strings.Split(strings.TrimPrefix(r.URL.Path, "/miner/switch/"), "/")
	if len(ss) != 3 {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}
	from, err := address.NewFromString(ss[0])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	to, err := address.NewFromString(ss[1])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	count, err := strconv.ParseInt(ss[2], 10, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = m.doSwitch(from, to, count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

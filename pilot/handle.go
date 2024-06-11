package pilot

import (
	"encoding/json"
	"net/http"

	"github.com/filecoin-project/go-address"
	"github.com/gh-efforts/lotus-pilot/middleware"
	"github.com/gh-efforts/lotus-pilot/repo/config"
	"github.com/google/uuid"
)

type MinerAPI struct {
	Miner string         `json:"miner"`
	API   config.APIInfo `json:"api"`
}

func (p *Pilot) Handle() {
	http.HandleFunc("POST /miner/add", middleware.Timer(p.addMinerHandle))
	http.HandleFunc("GET /miner/remove/{id}", middleware.Timer(p.removeMinerHandle))
	http.HandleFunc("GET /miner/list", middleware.Timer(p.listMinerHandle))
	http.HandleFunc("GET /miner/worker/{id}", middleware.Timer(p.workerHandle))
	http.HandleFunc("GET /miner/worker/all", middleware.Timer(p.minerWorkerAllHandle))

	http.HandleFunc("POST /switch/new", middleware.Timer(p.switchHandle))
	http.HandleFunc("GET /switch/get/{id}", middleware.Timer(p.getSwitchHandle))
	http.HandleFunc("GET /switch/cancel/{id}", middleware.Timer(p.cancelSwitchHandle))
	http.HandleFunc("GET /switch/remove/{id}", middleware.Timer(p.removeSwitchHandle))
	http.HandleFunc("GET /switch/list", middleware.Timer(p.listSwitchHandle))

	http.HandleFunc("GET /script/create/{id}", middleware.Timer(p.createScriptHandle))
}

func (p *Pilot) addMinerHandle(w http.ResponseWriter, r *http.Request) {
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
	if p.hasMiner(maddr) {
		http.Error(w, "miner already has", http.StatusBadRequest)
		return
	}

	mi, err := toMinerInfo(p.ctx, minerAPI.Miner, minerAPI.API)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = p.repo.CreateScript(mi.address, mi.token, mi.size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = p.repo.UpdateConfig(mi.address.String(), minerAPI.API)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p.addMiner(mi)
}

func (p *Pilot) removeMinerHandle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	maddr, err := address.NewFromString(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !p.hasMiner(maddr) {
		http.Error(w, "miner not found", http.StatusBadRequest)
		return
	}

	err = p.repo.UpdateConfig(maddr.String(), config.APIInfo{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = p.repo.RemoveScript(maddr.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p.removeMiner(maddr)
}

func (p *Pilot) listMinerHandle(w http.ResponseWriter, r *http.Request) {
	miners := p.listMiner()

	body, err := json.Marshal(&miners)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (p *Pilot) workerHandle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	maddr, err := address.NewFromString(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !p.hasMiner(maddr) {
		http.Error(w, "miner not found", http.StatusBadRequest)
		return
	}

	wi, err := p.getWorkerInfo(maddr)
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

func (p *Pilot) minerWorkerAllHandle(w http.ResponseWriter, r *http.Request) {
	out, err := p.minerWorkerAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(&out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (p *Pilot) switchHandle(w http.ResponseWriter, r *http.Request) {
	var req SwitchRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ss, err := p.newSwitch(req)
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

func (p *Pilot) getSwitchHandle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	uid, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(p.getSwitch(uid))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (p *Pilot) cancelSwitchHandle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	uid, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = p.cancelSwitch(uid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (p *Pilot) removeSwitchHandle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	uid, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = p.removeSwitch(uid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (p *Pilot) listSwitchHandle(w http.ResponseWriter, r *http.Request) {
	ss := p.listSwitch()

	body, err := json.Marshal(&ss)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (p *Pilot) createScriptHandle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := p.createScript(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

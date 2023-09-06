package miner

import "net/http"

func (m *Miner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugw("URL Path", r.URL.Path)
	switch r.URL.Path {
	case "/miner/add":
		m.addHandle(w, r)
	case "/miner/remove":
		m.removeHandle(w, r)
	case "/miner/list":
		m.listHandle(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (m *Miner) addHandle(w http.ResponseWriter, r *http.Request) {

}

func (m *Miner) removeHandle(w http.ResponseWriter, r *http.Request) {

}

func (m *Miner) listHandle(w http.ResponseWriter, r *http.Request) {

}

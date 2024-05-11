package pilot

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api/v0api"
)

type MinerInfo struct {
	api     v0api.StorageMiner
	closer  jsonrpc.ClientCloser
	address address.Address
	size    abi.SectorSize
	token   string
}

func (p *Pilot) addMiner(mi MinerInfo) {
	p.lk.Lock()
	defer p.lk.Unlock()

	p.miners[mi.address] = mi
	log.Infof("add miner: %s", mi.address)
}

func (p *Pilot) removeMiner(ma address.Address) {
	p.lk.Lock()
	defer p.lk.Unlock()

	if c := p.miners[ma].closer; c != nil {
		log.Infow("remove closed miner api", "miner", ma)
		c()
	}

	delete(p.miners, ma)
	log.Infof("remove miner: %s", ma)
}

func (p *Pilot) listMiner() []string {
	p.lk.RLock()
	defer p.lk.RUnlock()

	var miners []string
	for miner := range p.miners {
		miners = append(miners, miner.String())
	}

	return miners
}

func (p *Pilot) hasMiner(ma address.Address) bool {
	p.lk.RLock()
	defer p.lk.RUnlock()

	_, ok := p.miners[ma]
	return ok
}

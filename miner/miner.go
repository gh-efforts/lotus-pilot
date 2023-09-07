package miner

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/gh-efforts/lotus-pilot/config"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("pilot/miner")

type MinerInfo struct {
	api     v0api.StorageMiner
	closer  jsonrpc.ClientCloser
	address address.Address
	size    abi.SectorSize
}

type Miner struct {
	ctx context.Context

	lk     sync.RWMutex
	miners map[address.Address]MinerInfo
}

func NewMiner(ctx context.Context, cfg *config.Config) (*Miner, error) {
	miners := map[address.Address]MinerInfo{}
	for miner, info := range cfg.Miners {
		mi, err := toMinerInfo(ctx, miner, info)
		if err != nil {
			return nil, err
		}

		miners[mi.address] = mi
	}
	m := &Miner{
		ctx:    ctx,
		miners: miners,
	}
	return m, nil
}

func (m *Miner) add(mi MinerInfo) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.miners[mi.address] = mi
}

func (m *Miner) remove(ma address.Address) {
	m.lk.Lock()
	defer m.lk.Unlock()

	if c := m.miners[ma].closer; c != nil {
		log.Infow("remove closed miner api", "miner", ma)
		c()
	}

	delete(m.miners, ma)
}

func (m *Miner) list() []string {
	m.lk.RLock()
	defer m.lk.RUnlock()

	var miners []string
	for miner := range m.miners {
		miners = append(miners, miner.String())
	}

	return miners
}

func (m *Miner) has(ma address.Address) bool {
	m.lk.RLock()
	defer m.lk.RUnlock()

	_, ok := m.miners[ma]
	return ok
}

func (m *Miner) doSwitch(from address.Address, to address.Address, count int64) error {
	log.Infow("doing switch...", "from", from, "to", to, "count", count)
	return nil
}

func (m *Miner) Close() {
	m.lk.Lock()
	defer m.lk.Unlock()

	for _, miner := range m.miners {
		if miner.closer != nil {
			miner.closer()
		}
	}
}

func toMinerInfo(ctx context.Context, m string, info config.APIInfo) (MinerInfo, error) {
	maddr, err := address.NewFromString(m)
	if err != nil {
		return MinerInfo{}, err
	}

	if info.Addr == "" || info.Token == "" {
		return MinerInfo{}, fmt.Errorf("miner: %s info is empty", m)
	}

	addr := "ws://" + info.Addr + "/rpc/v0"
	headers := http.Header{"Authorization": []string{"Bearer " + info.Token}}
	api, closer, err := client.NewStorageMinerRPCV0(ctx, addr, headers)
	if err != nil {
		return MinerInfo{}, err
	}

	apiAddr, err := api.ActorAddress(ctx)
	if err != nil {
		return MinerInfo{}, err
	}
	if apiAddr != maddr {
		closer()
		return MinerInfo{}, fmt.Errorf("maddr not match, config maddr: %s, api maddr: %s", maddr, apiAddr)
	}
	size, err := api.ActorSectorSize(ctx, maddr)
	if err != nil {
		return MinerInfo{}, err
	}
	log.Infow("connected to miner", "miner", maddr, "addr", info.Addr)

	return MinerInfo{api: api, closer: closer, address: maddr, size: size}, nil
}

package miner

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/gh-efforts/lotus-pilot/repo"
	"github.com/gh-efforts/lotus-pilot/repo/config"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("pilot/miner")

type MinerAPI struct {
	Miner string         `json:"miner"`
	API   config.APIInfo `json:"api"`
}

type MinerInfo struct {
	api     v0api.StorageMiner
	closer  jsonrpc.ClientCloser
	address address.Address
	size    abi.SectorSize
	token   string
}

type Miner struct {
	ctx      context.Context
	interval time.Duration

	lk     sync.RWMutex
	miners map[address.Address]MinerInfo

	ch      chan switchRequestResponse
	swLk    sync.RWMutex
	switchs map[switchID]*switchState

	repo *repo.Repo
}

func NewMiner(ctx context.Context, r *repo.Repo) (*Miner, error) {
	conf, err := r.LoadConfig()
	if err != nil {
		return nil, err
	}

	miners := map[address.Address]MinerInfo{}
	for miner, info := range conf.Miners {
		mi, err := toMinerInfo(ctx, miner, info)
		if err != nil {
			return nil, err
		}

		miners[mi.address] = mi

		err = r.CreateScript(mi.address.String(), info.ToAPIInfo(), mi.size)
		if err != nil {
			return nil, err
		}
	}
	m := &Miner{
		ctx:      ctx,
		interval: time.Duration(conf.Interval),
		miners:   miners,
		ch:       make(chan switchRequestResponse, 20),
		switchs:  make(map[switchID]*switchState),
		repo:     r,
	}
	m.run()
	return m, nil
}

func (m *Miner) add(mi MinerInfo) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.miners[mi.address] = mi
}

func (m *Miner) getMiner(ma address.Address) (MinerInfo, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()

	mi, ok := m.miners[ma]
	if !ok {
		return MinerInfo{}, fmt.Errorf("not found miner: %s", ma)
	}
	return mi, nil
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

func (m *Miner) Close() {
	m.lk.Lock()
	defer m.lk.Unlock()

	for _, miner := range m.miners {
		if miner.closer != nil {
			miner.closer()
		}
	}
}

func (m *Miner) createScript(id string) error {
	m.lk.Lock()
	defer m.lk.Unlock()

	if id == "all" {
		for _, mi := range m.miners {
			err := m.repo.CreateScript(mi.address.String(), mi.token, mi.size)
			if err != nil {
				return err
			}
		}

		return nil
	}

	maddr, err := address.NewFromString(id)
	if err != nil {
		return err
	}
	mi, ok := m.miners[maddr]
	if !ok {
		return fmt.Errorf("miner: %s not found", id)
	}
	err = m.repo.CreateScript(mi.address.String(), mi.token, mi.size)
	if err != nil {
		return err
	}

	return nil
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

	return MinerInfo{api: api, closer: closer, address: maddr, size: size, token: info.ToAPIInfo()}, nil
}

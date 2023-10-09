package miner

import (
	"context"
	"encoding/json"
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
	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("pilot/miner")

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

	//TODO: use datastore replace map
	swLk    sync.RWMutex
	switchs map[uuid.UUID]*SwitchState

	repo *repo.Repo

	infoCache  map[address.Address]workerInfoCache
	statsCache map[address.Address]workerStatsCache
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

	data, err := r.ReadSwitchState()
	if err != nil {
		return nil, err
	}
	var switchs map[uuid.UUID]*SwitchState
	err = json.Unmarshal(data, &switchs)
	if err != nil {
		return nil, err
	}

	m := &Miner{
		ctx:        ctx,
		interval:   time.Duration(conf.Interval),
		miners:     miners,
		switchs:    switchs,
		repo:       r,
		infoCache:  make(map[address.Address]workerInfoCache),
		statsCache: make(map[address.Address]workerStatsCache),
	}
	m.run()
	return m, nil
}

func (m *Miner) run() {
	go func() {
		t := time.NewTicker(m.interval)
		for {
			select {
			case <-t.C:
				m.process()
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

func (m *Miner) add(mi MinerInfo) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.miners[mi.address] = mi
	log.Infof("add miner: %s", mi.address)
}

func (m *Miner) remove(ma address.Address) {
	m.lk.Lock()
	defer m.lk.Unlock()

	if c := m.miners[ma].closer; c != nil {
		log.Infow("remove closed miner api", "miner", ma)
		c()
	}

	delete(m.miners, ma)
	log.Infof("remove miner: %s", ma)
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

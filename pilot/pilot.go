package pilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/gh-efforts/lotus-pilot/repo"
	"github.com/gh-efforts/lotus-pilot/repo/config"
	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("pilot/pilot")

type Pilot struct {
	ctx          context.Context
	interval     time.Duration
	cacheTimeout time.Duration

	lk     sync.RWMutex
	miners map[address.Address]MinerInfo

	swLk    sync.RWMutex
	switchs map[uuid.UUID]*SwitchState

	repo *repo.Repo

	icLk      sync.Mutex
	infoCache map[address.Address]workerInfoCache

	scLk       sync.Mutex
	statsCache map[address.Address]workerStatsCache

	parallel int
}

func NewPilot(ctx context.Context, r *repo.Repo) (*Pilot, error) {
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

		err = r.CreateScript(mi.address, info.ToAPIInfo(), mi.size)
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

	p := &Pilot{
		ctx:          ctx,
		interval:     time.Duration(conf.Interval),
		cacheTimeout: time.Duration(conf.CacheTimeout),
		miners:       miners,
		switchs:      switchs,
		repo:         r,
		infoCache:    make(map[address.Address]workerInfoCache),
		statsCache:   make(map[address.Address]workerStatsCache),
		parallel:     conf.Parallel,
	}
	p.run()
	return p, nil
}

func (p *Pilot) run() {
	go func() {
		t := time.NewTicker(p.interval)
		for {
			select {
			case <-t.C:
				p.process()
			case <-p.ctx.Done():
				return
			}
		}
	}()
}

func (p *Pilot) Close() {
	p.lk.Lock()
	defer p.lk.Unlock()

	for _, miner := range p.miners {
		if miner.closer != nil {
			miner.closer()
		}
	}
}

func (p *Pilot) createScript(id string) error {
	p.lk.Lock()
	defer p.lk.Unlock()

	if id == "all" {
		for _, mi := range p.miners {
			err := p.repo.CreateScript(mi.address, mi.token, mi.size)
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
	mi, ok := p.miners[maddr]
	if !ok {
		return fmt.Errorf("miner: %s not found", id)
	}
	err = p.repo.CreateScript(mi.address, mi.token, mi.size)
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

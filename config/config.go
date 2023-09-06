package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v0api"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("pilot/config")

type APIInfo struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
}

type MinerInfo struct {
	Api     v0api.StorageMiner
	closer  jsonrpc.ClientCloser
	Address address.Address
	Size    abi.SectorSize
}

type Config struct {
	Miners map[string]APIInfo `json:"miners"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c Config
	err = json.Unmarshal(raw, &c)
	if err != nil {
		return nil, err
	}
	log.Info("load json config success")

	return &c, nil
}

func DefaultConfig() *Config {
	miner := APIInfo{
		Addr:  "10.122.1.29:2345",
		Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.tlJ8d4RIudknLHrKDSjyKzfbh8hGp9Ez1FZszblQLAI",
	}
	miner64 := APIInfo{
		Addr:  "10.122.1.29:2346",
		Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.7ZoJAcyY9ictWUdWsiV5AwmSTPHCczkT8Y6mTiN3Azw",
	}

	miners := make(map[string]APIInfo)
	miners["t017387"] = miner
	miners["t028064"] = miner64

	return &Config{
		Miners: miners,
	}
}

func toMinerInfo(ctx context.Context, m string, info APIInfo) (MinerInfo, error) {
	maddr, err := address.NewFromString(m)
	if err != nil {
		return MinerInfo{}, err
	}

	if info.Addr == "" || info.Token == "" {
		log.Warnf("miner: %s api info empty", maddr)
		return MinerInfo{Api: nil, Address: maddr}, nil
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
		return MinerInfo{}, fmt.Errorf("maddr not match, config maddr: %s, api maddr: %s", maddr, apiAddr)
	}
	size, err := api.ActorSectorSize(ctx, maddr)
	if err != nil {
		return MinerInfo{}, err
	}
	log.Infow("connected to miner", "miner", maddr, "addr", info.Addr)

	return MinerInfo{Api: api, closer: closer, Address: maddr, Size: size}, nil
}

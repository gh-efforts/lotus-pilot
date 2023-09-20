package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("pilot/config")

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	td, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(td)

	return nil
}

type APIInfo struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
}

func (a *APIInfo) ToAPIInfo() string {
	ss := strings.Split(a.Addr, ":")
	if len(ss) != 2 {
		return ""
	}
	return fmt.Sprintf("%s:/ip4/%s/tcp/%s/http", a.Token, ss[0], ss[1])
}

type Config struct {
	Interval Duration           `json:"interval"`
	Miners   map[string]APIInfo `json:"miners"`
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
		Interval: Duration(time.Minute),
		Miners:   miners,
	}
}

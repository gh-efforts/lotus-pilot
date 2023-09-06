package config_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gh-efforts/lotus-pilot/config"
)

func TestConfig(t *testing.T) {
	c := config.DefaultConfig()
	data, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		panic(err)
	}
	err = os.WriteFile("../config.json", data, 0666)
	if err != nil {
		panic(err)
	}
}

func TestLoadConfig(t *testing.T) {
	conf, err := config.LoadConfig("../config.json")
	if err != nil {
		panic(err)
	}
	t.Log(conf)
}

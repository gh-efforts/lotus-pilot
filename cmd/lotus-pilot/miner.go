package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/gh-efforts/lotus-pilot/pilot"
	"github.com/gh-efforts/lotus-pilot/repo/config"
	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
)

var minerCmd = &cli.Command{
	Name:  "miner",
	Usage: "manage miner list",
	Subcommands: []*cli.Command{
		minerAddCmd,
		minerRemoveCmd,
		minerListCmd,
		minerWorkerCmd,
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "connect",
			Value: "127.0.0.1:6788",
		},
	},
}

var minerAddCmd = &cli.Command{
	Name:  "add",
	Usage: "add new miner",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner-id",
		},
		&cli.StringFlag{
			Name: "addr",
		},
		&cli.StringFlag{
			Name: "token",
		},
	},
	Action: func(cctx *cli.Context) error {
		id := cctx.String("miner-id")
		addr := cctx.String("addr")
		token := cctx.String("token")
		if id == "" || addr == "" || token == "" {
			return errors.New("param is empty")
		}

		api := config.APIInfo{
			Addr:  addr,
			Token: token,
		}
		miner := pilot.MinerAPI{
			Miner: id,
			API:   api,
		}
		body, err := json.Marshal(&miner)
		if err != nil {
			return err
		}

		url := fmt.Sprintf("http://%s/miner/add", cctx.String("connect"))
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			r, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("status: %s msg: %s", resp.Status, string(r))
		}
		return nil
	},
}

var minerRemoveCmd = &cli.Command{
	Name:  "remove",
	Usage: "remove miner",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner-id",
		},
	},
	Action: func(cctx *cli.Context) error {
		id := cctx.String("miner-id")
		if id == "" {
			return errors.New("miner-id is empty")
		}

		url := fmt.Sprintf("http://%s/miner/remove/%s", cctx.String("connect"), id)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			r, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("status: %s msg: %s", resp.Status, string(r))
		}
		return nil
	},
}

var minerListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all miner",
	Action: func(cctx *cli.Context) error {
		url := fmt.Sprintf("http://%s/miner/list", cctx.String("connect"))
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			r, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("status: %s msg: %s", resp.Status, string(r))
		}

		var miners []string
		err = json.NewDecoder(resp.Body).Decode(&miners)
		if err != nil {
			return err
		}

		fmt.Println(miners)

		return nil
	},
}

var minerWorkerCmd = &cli.Command{
	Name:  "worker",
	Usage: "list miner workers",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner-id",
		},
	},
	Action: func(cctx *cli.Context) error {
		id := cctx.String("miner-id")
		if id == "" {
			return errors.New("miner-id is empty")
		}

		url := fmt.Sprintf("http://%s/miner/worker/%s", cctx.String("connect"), id)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			r, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("status: %s msg: %s", resp.Status, string(r))
		}

		var wi map[uuid.UUID]pilot.WorkerInfo
		err = json.NewDecoder(resp.Body).Decode(&wi)
		if err != nil {
			return err
		}

		printWorkerInfo(wi)
		return nil
	},
}

func printWorkerInfo(wi map[uuid.UUID]pilot.WorkerInfo) {
	for _, w := range wi {
		fmt.Printf("WorkerID: %s\n", w.WorkerID)
		fmt.Printf("StorageID: %s\n", w.StorageID)
		fmt.Printf("Hostname: %s\n", w.Hostname)
		fmt.Printf("Runing: %+v\n", w.Runing)
		fmt.Printf("Prepared: %+v\n", w.Prepared)
		fmt.Printf("Assigned: %+v\n", w.Assigned)
		fmt.Printf("Sched: %+v\n", w.Sched)
		fmt.Printf("LastStart: %s\n", w.LastStart)
		fmt.Printf("Sectors: %s\n", reflect.ValueOf(w.Sectors).MapKeys())
		fmt.Printf("Tasks: %s\n", reflect.ValueOf(w.Tasks).MapKeys())
		fmt.Println()
	}
}

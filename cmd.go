package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gh-efforts/lotus-pilot/config"
	"github.com/gh-efforts/lotus-pilot/miner"
	"github.com/urfave/cli/v2"
)

var minerCmd = &cli.Command{
	Name:  "miner",
	Usage: "manage miner list",
	Subcommands: []*cli.Command{
		minerAddCmd,
		minerRemoveCmd,
		minerListCmd,
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
		miner := miner.MinerAPI{
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

		r, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status: %s msg: %s", resp.Status, string(r))
		}
		fmt.Println(string(r))

		return nil
	},
}

var switchCmd = &cli.Command{
	Name:  "switch",
	Usage: "manage switch",
	Subcommands: []*cli.Command{
		switchNewCmd,
		switchGetCmd,
		switchCancelCmd,
		switchRemoveCmd,
		switchListCmd,
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "connect",
			Value: "127.0.0.1:6788",
		},
	},
}

var switchNewCmd = &cli.Command{
	Name:  "new",
	Usage: "send new switch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "from",
		},
		&cli.StringFlag{
			Name: "to",
		},
		&cli.IntFlag{
			Name: "count",
		},
		&cli.BoolFlag{
			Name: "disableAP",
		},
		&cli.StringSliceFlag{
			Name: "worker",
		},
	},
	Action: func(cctx *cli.Context) error {
		req := miner.SwitchRequest{
			From:      cctx.String("from"),
			To:        cctx.String("to"),
			Count:     cctx.Int("count"),
			Worker:    cctx.StringSlice("worker"),
			DisableAP: cctx.Bool("disableAP"),
		}

		body, err := json.Marshal(&req)
		if err != nil {
			return err
		}

		url := fmt.Sprintf("http://%s/switch/new", cctx.String("connect"))
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

		var srsp miner.SwitchResponse
		err = json.NewDecoder(resp.Body).Decode(&srsp)
		if err != nil {
			return err
		}

		fmt.Printf("switchID: %s\n", srsp.ID)
		fmt.Println("worker:")
		for _, w := range srsp.Worker {
			fmt.Println(w.WorkerID, w.Hostname)
		}

		return nil
	},
}

var switchGetCmd = &cli.Command{
	Name:  "get",
	Usage: "get switch state",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "switch-id",
		},
	},
	Action: func(cctx *cli.Context) error {
		id := cctx.String("switch-id")
		if id == "" {
			return errors.New("switch-id is empty")
		}

		url := fmt.Sprintf("http://%s/switch/get/%s", cctx.String("connect"), id)
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

		var ss miner.SwitchState
		err = json.NewDecoder(resp.Body).Decode(&ss)
		if err != nil {
			return err
		}

		fmt.Printf("switchID: %s\n", ss.ID)
		fmt.Printf("state: %s\n", ss.State)
		if ss.ErrMsg != "" {
			fmt.Printf("errMsg: %s\n", ss.ErrMsg)
		}
		fmt.Printf("switch request %+v\n\n", ss.Req)

		for _, w := range ss.Worker {
			fmt.Printf("workerID: %s\n", w.WorkerID)
			fmt.Printf("hostname: %s\n", w.Hostname)
			fmt.Printf("state: %s\n", w.State)
			if w.ErrMsg != "" {
				fmt.Printf("errMsg: %s\n", w.ErrMsg)
			}
			fmt.Printf("try: %d\n\n", w.Try)
		}
		//fmt.Printf("%+v", ss)

		return nil
	},
}

var switchCancelCmd = &cli.Command{
	Name: "cancel",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "switch-id",
		},
	},
	Action: func(cctx *cli.Context) error {
		id := cctx.String("switch-id")
		if id == "" {
			return errors.New("switch-id is empty")
		}

		url := fmt.Sprintf("http://%s/switch/cancel/%s", cctx.String("connect"), id)
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

var switchRemoveCmd = &cli.Command{
	Name: "remove",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "switch-id",
		},
	},
	Action: func(cctx *cli.Context) error {
		id := cctx.String("switch-id")
		if id == "" {
			return errors.New("switch-id is empty")
		}

		url := fmt.Sprintf("http://%s/switch/remove/%s", cctx.String("connect"), id)
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

var switchListCmd = &cli.Command{
	Name:  "list",
	Usage: "get all switch id",
	Action: func(cctx *cli.Context) error {
		url := fmt.Sprintf("http://%s/switch/list", cctx.String("connect"))
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		r, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status: %s msg: %s", resp.Status, string(r))
		}
		fmt.Println(string(r))
		return nil
	},
}

var scriptCmd = &cli.Command{
	Name:  "script",
	Usage: "manage script",
	Subcommands: []*cli.Command{
		scriptCreateCmd,
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "connect",
			Value: "127.0.0.1:6788",
		},
	},
}

var scriptCreateCmd = &cli.Command{
	Name:  "create",
	Usage: "create script",
	Action: func(cctx *cli.Context) error {
		url := fmt.Sprintf("http://%s/script/create/%s", cctx.String("connect"), cctx.Args().First())
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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/filecoin-project/go-address"
	"github.com/gh-efforts/lotus-pilot/pilot"
	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
)

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
		from, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return err
		}
		to, err := address.NewFromString(cctx.String("to"))
		if err != nil {
			return err
		}
		worker := []uuid.UUID{}
		for _, w := range cctx.StringSlice("worker") {
			i, err := uuid.Parse(w)
			if err != nil {
				return err
			}
			worker = append(worker, i)
		}

		req := pilot.SwitchRequest{
			From:      from,
			To:        to,
			Count:     cctx.Int("count"),
			Worker:    worker,
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

		var ss pilot.SwitchState
		err = json.NewDecoder(resp.Body).Decode(&ss)
		if err != nil {
			return err
		}

		printSwitchState(ss)
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
		id, err := uuid.Parse(cctx.String("switch-id"))
		if err != nil {
			return err
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

		var ss pilot.SwitchState
		err = json.NewDecoder(resp.Body).Decode(&ss)
		if err != nil {
			return err
		}

		printSwitchState(ss)
		return nil
	},
}

var switchCancelCmd = &cli.Command{
	Name:  "cancel",
	Usage: "cancel a switch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "switch-id",
		},
	},
	Action: func(cctx *cli.Context) error {
		id, err := uuid.Parse(cctx.String("switch-id"))
		if err != nil {
			return err
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
	Name:  "remove",
	Usage: "remove switch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "switch-id",
		},
	},
	Action: func(cctx *cli.Context) error {
		id, err := uuid.Parse(cctx.String("switch-id"))
		if err != nil {
			return err
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

		if resp.StatusCode != http.StatusOK {
			r, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("status: %s msg: %s", resp.Status, string(r))
		}

		var ss []string
		err = json.NewDecoder(resp.Body).Decode(&ss)
		if err != nil {
			return err
		}

		fmt.Println(ss)

		return nil
	},
}

func printSwitchState(ss pilot.SwitchState) {
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
		if w.Try != 0 {
			fmt.Printf("try: %d\n\n", w.Try)
		}
	}
}

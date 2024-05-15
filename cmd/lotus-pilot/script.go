package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/urfave/cli/v2"
)

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
	Name:      "create",
	Usage:     "create worker start script",
	ArgsUsage: "[minerID/all]",
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

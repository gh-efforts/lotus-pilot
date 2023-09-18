package main

import (
	"net/http"
	"os"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	logging "github.com/ipfs/go-log/v2"

	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/gh-efforts/lotus-pilot/config"
	"github.com/gh-efforts/lotus-pilot/metrics"
	"github.com/gh-efforts/lotus-pilot/miner"
)

var (
	log = logging.Logger("pilot/main")
)

func main() {
	logging.SetLogLevel("*", "INFO")

	local := []*cli.Command{
		runCmd,
		minerCmd,
		switchCmd,
	}

	app := &cli.App{
		Name:     "lotus-pilot",
		Usage:    "lotus pilot server",
		Version:  UserVersion(),
		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorf("%+v", err)
	}
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "run pilot server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: "0.0.0.0:6788",
		},
		&cli.StringFlag{
			Name:  "config",
			Value: "./config.json",
			Usage: "specify config file path",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Bool("debug") {
			logging.SetLogLevelRegex("pilot/*", "DEBUG")
		}

		log.Info("starting lotus pilot...")

		ctx := cliutil.ReqContext(cctx)
		conf, err := config.LoadConfig(cctx.String("config"))
		if err != nil {
			return err
		}

		exporter, err := prometheus.NewExporter(prometheus.Options{
			Namespace: "lotuspilot",
		})
		if err != nil {
			return err
		}

		ctx, _ = tag.New(ctx,
			tag.Insert(metrics.Version, BuildVersion),
			tag.Insert(metrics.Commit, CurrentCommit),
		)
		if err := view.Register(
			metrics.Views...,
		); err != nil {
			return err
		}
		stats.Record(ctx, metrics.Info.M(1))

		miner, err := miner.NewMiner(ctx, conf)
		if err != nil {
			return err
		}

		listen := cctx.String("listen")
		log.Infow("pilot server", "listen", listen)

		http.Handle("/metrics", exporter)
		miner.Handle()
		server := &http.Server{
			Addr: listen,
		}

		go func() {
			<-ctx.Done()
			log.Info("shutdown pilot server")
			miner.Close()
			server.Shutdown(ctx)
		}()

		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}
		return nil
	},
}

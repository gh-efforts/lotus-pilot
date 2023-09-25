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
	"github.com/gh-efforts/lotus-pilot/build"
	"github.com/gh-efforts/lotus-pilot/metrics"
	"github.com/gh-efforts/lotus-pilot/miner"
	"github.com/gh-efforts/lotus-pilot/repo"

	_ "net/http/pprof"
)

var (
	log = logging.Logger("pilot/main")
)

func main() {
	logging.SetLogLevel("*", "INFO")

	local := []*cli.Command{
		initCmd,
		runCmd,
		minerCmd,
		switchCmd,
		scriptCmd,
		pprofCmd,
	}

	app := &cli.App{
		Name:     "lotus-pilot",
		Usage:    "lotus pilot server",
		Version:  build.UserVersion(),
		Commands: local,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "repo",
				Value: "~/.lotuspilot",
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorf("%+v", err)
	}
}

var initCmd = &cli.Command{
	Name:  "init",
	Usage: "init repo",
	Action: func(cctx *cli.Context) error {
		r, err := repo.New(cctx.String("repo"))
		if err != nil {
			return nil
		}
		return r.Init()
	},
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "run pilot server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: "0.0.0.0:6788",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "test",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Bool("debug") {
			logging.SetLogLevelRegex("pilot/*", "DEBUG")
		}
		if cctx.Bool("tesy") {
			build.AnsibleTest = true
		}

		log.Info("starting lotus pilot...")

		ctx := cliutil.ReqContext(cctx)

		exporter, err := prometheus.NewExporter(prometheus.Options{
			Namespace: "lotuspilot",
		})
		if err != nil {
			return err
		}

		ctx, _ = tag.New(ctx,
			tag.Insert(metrics.Version, build.BuildVersion),
			tag.Insert(metrics.Commit, build.CurrentCommit),
		)
		if err := view.Register(
			metrics.Views...,
		); err != nil {
			return err
		}
		stats.Record(ctx, metrics.Info.M(1))

		r, err := repo.New(cctx.String("repo"))
		if err != nil {
			return nil
		}
		r.Init()
		if err != nil {
			return nil
		}

		miner, err := miner.NewMiner(ctx, r)
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

package main

import (
	"net/http"
	"os"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/gorilla/mux"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	logging "github.com/ipfs/go-log/v2"

	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/gh-efforts/lotus-pilot/config"
	"github.com/gh-efforts/lotus-pilot/metrics"
)

var (
	log = logging.Logger("pilot/main")
)

func main() {
	logging.SetLogLevel("*", "INFO")

	local := []*cli.Command{
		runCmd,
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
	Name: "run",
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
		_ = conf

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

		listen := cctx.String("listen")
		log.Infow("pilot server", "listen", listen)

		h, err := PilotHandler(exporter)
		if err != nil {
			return err
		}
		server := &http.Server{
			Addr:    listen,
			Handler: h,
		}

		go func() {
			<-ctx.Done()
			time.Sleep(time.Millisecond * 100)
			log.Info("shutdown pilot server")
			server.Shutdown(ctx)
		}()

		return server.ListenAndServe()
	},
}

func PilotHandler(exporter *prometheus.Exporter) (http.Handler, error) {
	m := mux.NewRouter()

	m.Handle("/metrics", exporter)

	return m, nil
}

package miner

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gh-efforts/lotus-pilot/metrics"
	"go.opencensus.io/tag"
)

func middlewareTimer(handler http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.Endpoint, endpoint(request.URL.Path)))
		stop := metrics.Timer(ctx, metrics.APIRequestDuration)
		defer stop()

		handler(writer, request)
	}
}

func endpoint(path string) string {
	ss := strings.Split(path, "/")
	if len(ss) < 3 {
		return path
	}
	return fmt.Sprintf("/%s/%s", ss[1], ss[2])
}

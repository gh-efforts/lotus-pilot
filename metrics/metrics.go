package metrics

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// Distribution
var defaultMillisecondsDistribution = view.Distribution(0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 3000, 4000, 5000, 7500, 10000, 20000, 50000, 100_000, 250_000, 500_000, 1000_000)

// Tags
var (
	Version, _ = tag.NewKey("version")
	Commit, _  = tag.NewKey("commit")

	ErrorType, _  = tag.NewKey("error_type")
	RecordType, _ = tag.NewKey("record_type")
)

// Measures
var (
	Info               = stats.Int64("info", "Arbitrary counter to tag monitor info to", stats.UnitDimensionless)
	SelfError          = stats.Int64("self/error", "couter for monitor error", stats.UnitDimensionless)
	SelfRecordDuration = stats.Float64("self/record", "duration of every record", stats.UnitMilliseconds)
)

// Views
var (
	InfoView = &view.View{
		Name:        "info",
		Description: "Pilot information",
		Measure:     Info,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Version, Commit},
	}
	SelfErrorView = &view.View{
		Measure:     SelfError,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{ErrorType},
	}
	SelfRecordDurationView = &view.View{
		Measure:     SelfRecordDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{RecordType},
	}
)

var Views = []*view.View{
	InfoView,
	SelfErrorView,
	SelfRecordDurationView,
}

// SinceInMilliseconds returns the duration of time since the provide time as a float64.
func SinceInMilliseconds(startTime time.Time) float64 {
	return float64(time.Since(startTime).Nanoseconds()) / 1e6
}

// Timer is a function stopwatch, calling it starts the timer,
// calling the returned function will record the duration.
func Timer(ctx context.Context, recordType string) func() time.Duration {
	ctx, _ = tag.New(ctx,
		tag.Upsert(RecordType, recordType),
	)
	start := time.Now()
	return func() time.Duration {
		stats.Record(ctx, SelfRecordDuration.M(SinceInMilliseconds(start)))
		return time.Since(start)
	}
}

func RecordError(ctx context.Context, errType string) {
	ctx, _ = tag.New(ctx,
		tag.Upsert(ErrorType, errType),
	)

	stats.Record(ctx, SelfError.M(1))
}

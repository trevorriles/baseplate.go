package metricsbp

import (
	"context"
	"strings"
	"time"

	"github.com/reddit/baseplate.go/log"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/influxstatsd"
)

// ReporterTickerInterval is the interval the reporter sends data to statsd
// server. Default is one minute.
var ReporterTickerInterval = time.Minute

// M is short for "Metrics".
//
// This is the global Statsd to use.
// It's pre-initialized with one that does not send metrics anywhere,
// so it won't cause panic even if you don't initialize it before using it
// (for example, it's safe to be used in test code).
//
// But in production code you should still properly initialize it to actually
// send your metrics to your statsd collector,
// usually early in your main function:
//
//     func main() {
//       flag.Parse()
//       ctx, cancel := context.WithCancel(context.Background())
//       defer cancel()
//       metricsbp.M = metricsbp.NewStatsd{
//         ctx,
//         metricsbp.StatsdConfig{
//           ...
//         },
//       }
//       metricsbp.M.RunSysStats()
//       ...
//     }
//
//     func someOtherFunction() {
//       ...
//       metricsbp.M.Counter("my-counter").Add(1)
//       ...
//     }
var M = NewStatsd(context.Background(), StatsdConfig{})

// Statsd defines a statsd reporter (with influx extension) and the root of the
// metrics.
//
// It can be used to create metrics,
// and also maintains the background reporting goroutine,
//
// Please use NewStatsd to initialize it.
//
// When a *Statsd is nil,
// any function calls to it will fallback to use M instead,
// so they are safe to use (unless M was explicitly overridden as nil),
// but accessing the fields will still cause panic.
// For example:
//
//     st := (*metricsbp.Statsd)(nil)
//     st.Counter("my-counter").Add(1) // does not panic unless metricsbp.M is nil
//     st.Statsd.NewCounter("my-counter", 0.5).Add(1) // panics
type Statsd struct {
	Statsd *influxstatsd.Influxstatsd

	ctx        context.Context
	sampleRate float64
}

// StatsdConfig is the configs used in NewStatsd.
type StatsdConfig struct {
	// Prefix is the common metrics path prefix shared by all metrics managed by
	// (created from) this Metrics object.
	//
	// If it's not ending with a period ("."), a period will be added.
	Prefix string

	// DefaultSampleRate is the default reporting sample rate used when creating
	// metrics.
	DefaultSampleRate float64

	// Address is the UDP address (in "host:port" format) of the statsd service.
	//
	// It could be empty string, in which case we won't start the background
	// reporting goroutine.
	//
	// When Address is the empty string,
	// the Statsd object and the metrics created under it will not be reported
	// anywhere,
	// so it can be used in lieu of discarded metrics in test code.
	// But the metrics are still stored in memory,
	// so it shouldn't be used in lieu of discarded metrics in prod code.
	Address string

	// The log level used by the reporting goroutine.
	LogLevel log.Level

	// Labels are the labels/tags to be attached to every metrics created
	// from this Statsd object. For labels/tags only needed by some metrics,
	// use Counter/Gauge/Timing.With() instead.
	Labels map[string]string
}

// NewStatsd creates a Statsd object.
//
// It also starts a background reporting goroutine when Address is not empty.
// The goroutine will be stopped when the passed in context is canceled.
//
// NewStatsd never returns nil.
func NewStatsd(ctx context.Context, cfg StatsdConfig) *Statsd {
	prefix := cfg.Prefix
	if prefix != "" && !strings.HasSuffix(prefix, ".") {
		prefix = prefix + "."
	}
	labels := make([]string, 0, len(cfg.Labels)*2)
	for k, v := range cfg.Labels {
		labels = append(labels, k, v)
	}
	st := &Statsd{
		Statsd:     influxstatsd.New(prefix, log.KitLogger(cfg.LogLevel), labels...),
		ctx:        ctx,
		sampleRate: cfg.DefaultSampleRate,
	}

	if cfg.Address != "" {
		go func() {
			ticker := time.NewTicker(ReporterTickerInterval)
			defer ticker.Stop()

			st.Statsd.SendLoop(ctx, ticker.C, "udp", cfg.Address)
		}()
	}

	return st
}

// Counter returns a counter metrics to the name.
//
// It uses the DefaultSampleRate used to create Statsd object.
// If you need a different sample rate,
// you could use st.Statsd.NewCounter instead.
func (st *Statsd) Counter(name string) metrics.Counter {
	st = st.fallback()
	return st.Statsd.NewCounter(name, st.sampleRate)
}

// Histogram returns a histogram metrics to the name.
//
// It uses the DefaultSampleRate used to create Statsd object.
// If you need a different sample rate,
// you could use st.Statsd.NewTiming instead.
func (st *Statsd) Histogram(name string) metrics.Histogram {
	st = st.fallback()
	return st.Statsd.NewTiming(name, st.sampleRate)
}

// Gauge returns a gauge metrics to the name.
//
// It's a shortcut to st.Statsd.NewGauge(name).
func (st *Statsd) Gauge(name string) metrics.Gauge {
	st = st.fallback()
	return st.Statsd.NewGauge(name)
}

func (st *Statsd) fallback() *Statsd {
	if st == nil {
		return M
	}
	return st
}

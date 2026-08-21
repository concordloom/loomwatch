package metrics

import "github.com/prometheus/client_golang/prometheus"

// Fork change: transitional dual publication of series during the brand
// rename.
//
// The series used to be named onwatch_*, and Grafana rules and the
// accumulated Prometheus history depend on them. Renaming in one step would
// have silenced alerts and cut the graphs off, so for a while the exporter
// publishes both names: loomwatch_* as primary and onwatch_* as deprecated.
// The removal order is: move the rules to the new name, confirm they
// evaluate, and only then delete the deprecated series from here.
//
// Past history stays under the old name - that is the unavoidable price of
// the rename, not a defect: there is nothing to restore it under the new
// name from.

// dualGauge writes a value to both series at once.
type dualGauge struct {
	primary prometheus.Gauge
	legacy  prometheus.Gauge
}

func (d dualGauge) Set(v float64) {
	d.primary.Set(v)
	if d.legacy != nil {
		d.legacy.Set(v)
	}
}

// dualCounter increments both series at once.
type dualCounter struct {
	primary prometheus.Counter
	legacy  prometheus.Counter
}

func (d dualCounter) Inc() {
	d.primary.Inc()
	if d.legacy != nil {
		d.legacy.Inc()
	}
}

func (d dualCounter) Add(v float64) {
	d.primary.Add(v)
	if d.legacy != nil {
		d.legacy.Add(v)
	}
}

// dualGaugeVec is a pair of vectors with identical labels under different names.
type dualGaugeVec struct {
	primary *prometheus.GaugeVec
	legacy  *prometheus.GaugeVec
}

func newDualGaugeVec(name, legacyName, help string, labels []string) *dualGaugeVec {
	return &dualGaugeVec{
		primary: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels),
		legacy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: legacyName,
			Help: help + " (deprecated name, use " + name + ")",
		}, labels),
	}
}

func (v *dualGaugeVec) With(labels prometheus.Labels) dualGauge {
	return dualGauge{primary: v.primary.With(labels), legacy: v.legacy.With(labels)}
}

func (v *dualGaugeVec) WithLabelValues(vals ...string) dualGauge {
	return dualGauge{
		primary: v.primary.WithLabelValues(vals...),
		legacy:  v.legacy.WithLabelValues(vals...),
	}
}

func (v *dualGaugeVec) Reset() {
	v.primary.Reset()
	v.legacy.Reset()
}

func (v *dualGaugeVec) collectors() []prometheus.Collector {
	return []prometheus.Collector{v.primary, v.legacy}
}

// dualCounterVec is the same for counters.
type dualCounterVec struct {
	primary *prometheus.CounterVec
	legacy  *prometheus.CounterVec
}

func newDualCounterVec(name, legacyName, help string, labels []string) *dualCounterVec {
	return &dualCounterVec{
		primary: prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels),
		legacy: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: legacyName,
			Help: help + " (deprecated name, use " + name + ")",
		}, labels),
	}
}

func (v *dualCounterVec) WithLabelValues(vals ...string) dualCounter {
	return dualCounter{
		primary: v.primary.WithLabelValues(vals...),
		legacy:  v.legacy.WithLabelValues(vals...),
	}
}

func (v *dualCounterVec) collectors() []prometheus.Collector {
	return []prometheus.Collector{v.primary, v.legacy}
}

package metrics

import "github.com/prometheus/client_golang/prometheus"

// Fork change: переходная двойная публикация рядов при переименовании бренда.
//
// Ряды назывались onwatch_*, и на них завязаны правила Grafana и накопленная
// история Prometheus. Одномоментное переименование погасило бы алерты и
// оборвало графики, поэтому экспортёр какое-то время публикует оба имени:
// loomwatch_* как основное и onwatch_* как устаревшее. Порядок снятия:
// перевести правила на новое имя, убедиться, что они считают, и только потом
// удалить устаревший ряд отсюда.
//
// История за прошлое остаётся под старым именем — это неизбежная цена
// переименования, а не дефект: восстановить её под новым именем нечем.

// dualGauge пишет значение в оба ряда сразу.
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

// dualCounter увеличивает оба ряда сразу.
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

// dualGaugeVec — пара векторов с одинаковыми метками под разными именами.
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

// dualCounterVec — то же для счётчиков.
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

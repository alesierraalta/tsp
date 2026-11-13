package metering

import (
	"fmt"

	"example.com/billing/internal/store"
)

// Quota is what a subscription is allowed to use in a cycle and what it used.
type Quota struct {
	Plan   store.Plan
	Used   int64
	Metric string
}

// Validate checks the quota.
func (q Quota) Validate() error {
	if q.Metric == "" {
		return fmt.Errorf("metering: a quota needs a metric")
	}
	if q.Used < 0 {
		return fmt.Errorf("metering: %d units used is not a quantity", q.Used)
	}
	return nil
}

// Included returns the quantity the plan carries for the metric.
func (q Quota) Included() int64 { return q.Plan.IncludedQuantity }

// Remaining returns how much of the included quantity is left. A subscription that used more than its
// plan includes has nothing left, never a negative remainder: the overage is a bill, not a debt of
// days.
func (q Quota) Remaining() int64 {
	rest := q.Included() - q.Used
	if rest < 0 {
		return 0
	}
	return rest
}

// Exceeded reports whether the subscription used more than its plan includes.
func (q Quota) Exceeded() bool { return q.Used > q.Included() }

// Fraction returns the share of the included quantity used, as a whole percentage. A plan that
// includes nothing is at 100% as soon as it is used.
func (q Quota) Fraction() int {
	included := q.Included()
	if included <= 0 {
		if q.Used > 0 {
			return 100
		}
		return 0
	}
	return int(q.Used * 100 / included)
}

// Alert is the state a quota is in for the console and the operator's report.
type Alert struct {
	Metric    string
	Used      int64
	Included  int64
	Remaining int64
	Exceeded  bool
	Fraction  int
}

// Alerts returns one alert per metric of the events given, in metric order.
func Alerts(plan store.Plan, totals map[string]int64) []Alert {
	names := Metrics(totals)
	out := make([]Alert, 0, len(names))
	for _, metric := range names {
		quota := Quota{Plan: plan, Used: totals[metric], Metric: metric}
		out = append(out, Alert{
			Metric:    metric,
			Used:      quota.Used,
			Included:  quota.Included(),
			Remaining: quota.Remaining(),
			Exceeded:  quota.Exceeded(),
			Fraction:  quota.Fraction(),
		})
	}
	return out
}

// ExceededMetrics lists the metrics a subscription is over its plan on.
func ExceededMetrics(plan store.Plan, totals map[string]int64) []string {
	var out []string
	for _, metric := range Metrics(totals) {
		if totals[metric] > plan.IncludedQuantity {
			out = append(out, metric)
		}
	}
	return out
}

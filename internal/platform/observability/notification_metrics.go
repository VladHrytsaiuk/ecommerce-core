package observability

import (
	"github.com/prometheus/client_golang/prometheus"

	notificationsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/application"
)

// deadNotificationJobs is how many messages never went out.
//
// Retention deletes these after its window, so without this gauge the cleanup
// would quietly erase the evidence that a store has been failing to send mail —
// the table empties and nothing ever said why. It is sampled from the table on
// each retention pass rather than incremented at the transition, so it survives
// a restart and reports what is actually there.
var deadNotificationJobs = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "notification_jobs_dead",
	Help: "Notification jobs that exhausted their retries and were never delivered.",
})

func init() { prometheus.MustRegister(deadNotificationJobs) }

// NotificationMetrics is the Prometheus implementation of the notification
// retention worker's reporting port.
type NotificationMetrics struct{}

func NewNotificationMetrics() NotificationMetrics { return NotificationMetrics{} }

func (NotificationMetrics) DeadNotificationJobs(count int) {
	deadNotificationJobs.Set(float64(count))
}

var _ notificationsApp.DeadJobRecorder = NotificationMetrics{}

package metrics

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/prometheus/client_golang/prometheus"
)

func nowUnix() int64 { return time.Now().Unix() }

// countedCollections are the row counts worth a series. Deliberately not every
// collection: ticket_events and ticket_comments grow without bound and say
// nothing an operator acts on, and time_sessions is at most one row per agent.
var countedCollections = []string{
	"tickets",
	"customers",
	"staff",
	"users",
	"things",
	"locations",
	"projects",
	"visits",
	"time_entries",
	"maintenance_plans",
}

// ticketStatuses is the full select, listed so a status with no rows publishes
// a 0 rather than vanishing. A gap and a zero mean different things to an
// alert, and only one of them is true.
var ticketStatuses = []string{"open", "in_progress", "waiting", "resolved", "closed"}

// dbCollector reads the database at scrape time.
//
// It is a custom collector rather than a set of gauges on a timer because a
// row count is cheap and a scrape is the only moment anyone wants it; a
// background refresher would add a second cadence to reason about and would
// report numbers from whenever it last ran.
type dbCollector struct {
	app core.App

	records  *prometheus.Desc
	tickets  *prometheus.Desc
	dbSize   *prometheus.Desc
	errCount *prometheus.Desc
}

func newDBCollector(app core.App) *dbCollector {
	return &dbCollector{
		app: app,
		records: prometheus.NewDesc(
			Namespace+"_records", "Rows in a helpdesk collection.",
			[]string{"collection"}, nil),
		tickets: prometheus.NewDesc(
			Namespace+"_tickets", "Tickets by status. No customer label: see the package comment.",
			[]string{"status"}, nil),
		dbSize: prometheus.NewDesc(
			Namespace+"_database_size_bytes",
			"data.db plus its WAL and shared-memory files. NOT uploaded attachments, which live in pb_data/storage and are in every backup; watch that directory with host disk metrics.",
			nil, nil),
		errCount: prometheus.NewDesc(
			Namespace+"_collector_errors", "Collectors that failed during this scrape.",
			nil, nil),
	}
}

func (c *dbCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.records
	ch <- c.tickets
	ch <- c.dbSize
	ch <- c.errCount
}

func (c *dbCollector) Collect(ch chan<- prometheus.Metric) {
	var errs int

	for _, name := range countedCollections {
		n, err := c.count("SELECT COUNT(*) FROM {{" + name + "}}")
		if err != nil {
			// Nothing emitted for this collection. See the package comment.
			slog.Debug("metrics: collection count failed", "collection", name, "err", err)
			errs++
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.records, prometheus.GaugeValue, float64(n), name)
	}

	for _, status := range ticketStatuses {
		n, err := c.count("SELECT COUNT(*) FROM {{tickets}} WHERE status = '" + status + "'")
		if err != nil {
			slog.Debug("metrics: ticket status count failed", "status", status, "err", err)
			errs++
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.tickets, prometheus.GaugeValue, float64(n), status)
	}

	if size, err := c.databaseSize(); err != nil {
		slog.Debug("metrics: database size failed", "err", err)
		errs++
	} else {
		ch <- prometheus.MustNewConstMetric(c.dbSize, prometheus.GaugeValue, float64(size))
	}

	ch <- prometheus.MustNewConstMetric(c.errCount, prometheus.GaugeValue, float64(errs))
}

func (c *dbCollector) count(query string) (int, error) {
	var n int
	if err := c.app.DB().NewQuery(query).Row(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// databaseSize sums data.db and its sidecars. The WAL is included because it
// is real disk the process is using and can dwarf the main file between
// checkpoints, which is exactly when someone is asking.
func (c *dbCollector) databaseSize() (int64, error) {
	base := filepath.Join(c.app.DataDir(), "data.db")
	var total int64
	var firstErr error
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(base + suffix)
		if err != nil {
			// A missing WAL or SHM is normal, not an error. A missing data.db
			// is not, and is the only one that fails the collector.
			if suffix == "" && firstErr == nil {
				firstErr = err
			}
			continue
		}
		total += info.Size()
	}
	if firstErr != nil {
		return 0, firstErr
	}
	return total, nil
}

// RegisterDB adds the database collector to the set.
func (s *Set) RegisterDB(app core.App) {
	s.Registry.MustRegister(newDBCollector(app))
}

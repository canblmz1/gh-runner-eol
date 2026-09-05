// Package eol turns GitHub's runner end-of-life schedule into a risk
// assessment. It is deliberately free of I/O so the policy is unit-testable
// and reusable by every output format.
package eol

import (
	"math"
	"time"
)

// Schedule is the normalized response of
// GET /.../actions/runners/deprecations/{version}.
//
// Observed API behaviour (Sep 2026), which this model encodes:
//   - runtime_deprecates_at is null for the current release;
//   - registration_deprecates_at is frequently absent from the payload;
//   - versions GitHub no longer tracks return 404 (Found == false).
type Schedule struct {
	Version                  string
	Found                    bool
	RuntimeDeprecatesAt      *time.Time
	RegistrationDeprecatesAt *time.Time
}

// Status is the risk bucket for a runner version.
type Status string

const (
	// StatusCurrent: GitHub has not scheduled an end-of-life date yet.
	StatusCurrent Status = "current"
	// StatusHealthy: EOL is scheduled but comfortably in the future.
	StatusHealthy Status = "healthy"
	// StatusWarning: EOL is within the warning window.
	StatusWarning Status = "warning"
	// StatusOverdue: runtime support has ended; GitHub will not queue jobs.
	StatusOverdue Status = "overdue"
	// StatusUnknown: version missing on the runner or unknown to the API.
	StatusUnknown Status = "unknown"
)

// Severity orders statuses for sorting and exit-code decisions.
func (s Status) Severity() int {
	switch s {
	case StatusOverdue:
		return 3
	case StatusWarning:
		return 2
	case StatusUnknown:
		return 1
	default:
		return 0
	}
}

// Label is the fixed-width marker used by the table renderer.
func (s Status) Label() string {
	switch s {
	case StatusOverdue:
		return "OVERDUE"
	case StatusWarning:
		return "WARNING"
	case StatusUnknown:
		return "UNKNOWN"
	case StatusHealthy:
		return "OK"
	case StatusCurrent:
		return "OK"
	}
	return string(s)
}

// Assessment is the evaluated risk for one runner version at a point in time.
type Assessment struct {
	Version                  string     `json:"version"`
	Status                   Status     `json:"status"`
	RuntimeDeprecatesAt      *time.Time `json:"runtime_deprecates_at,omitempty"`
	RegistrationDeprecatesAt *time.Time `json:"registration_deprecates_at,omitempty"`
	// DaysLeft until runtime support ends. Negative when overdue. Nil when
	// no date is known.
	DaysLeft *int `json:"days_left,omitempty"`
	// RegistrationBlocked is true when new runners on this version can no
	// longer register (config.sh is rejected).
	RegistrationBlocked bool   `json:"registration_blocked"`
	Reason              string `json:"reason"`
}

// Assess applies the risk policy to a schedule.
func Assess(s Schedule, now time.Time, warnDays int) Assessment {
	a := Assessment{
		Version:                  s.Version,
		RuntimeDeprecatesAt:      s.RuntimeDeprecatesAt,
		RegistrationDeprecatesAt: s.RegistrationDeprecatesAt,
	}

	if s.Version == "" {
		a.Status = StatusUnknown
		a.Reason = "runner did not report a version"
		return a
	}
	if !s.Found {
		a.Status = StatusUnknown
		a.Reason = "version is not in GitHub's deprecation schedule (too old or not yet published)"
		return a
	}

	if s.RegistrationDeprecatesAt != nil && !now.Before(*s.RegistrationDeprecatesAt) {
		a.RegistrationBlocked = true
	}

	if s.RuntimeDeprecatesAt == nil {
		a.Status = StatusCurrent
		a.Reason = "no end-of-life scheduled"
		return a
	}

	days := daysBetween(now, *s.RuntimeDeprecatesAt)
	a.DaysLeft = &days

	switch {
	case days < 0:
		a.Status = StatusOverdue
		a.Reason = "runtime support ended; GitHub will not queue jobs to this version"
	case days <= warnDays:
		a.Status = StatusWarning
		a.Reason = "runtime support ends soon"
	default:
		a.Status = StatusHealthy
		a.Reason = "runtime support scheduled to end"
	}
	return a
}

// daysBetween returns whole days from now until t, rounding away from zero so
// that "ends later today" reads as 1 day left and "ended earlier today" as -1.
func daysBetween(now, t time.Time) int {
	h := t.Sub(now).Hours()
	if h >= 0 {
		return int(math.Ceil(h / 24))
	}
	return -int(math.Ceil(-h / 24))
}

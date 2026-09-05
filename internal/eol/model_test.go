package eol

import (
	"testing"
	"time"
)

func ts(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestAssess(t *testing.T) {
	now := *ts("2026-09-05T10:00:00Z")

	tests := []struct {
		name     string
		sched    Schedule
		want     Status
		wantDays *int
		wantReg  bool
	}{
		{
			name:  "current release has no date",
			sched: Schedule{Version: "2.337.0", Found: true},
			want:  StatusCurrent,
		},
		{
			name:     "healthy when far away",
			sched:    Schedule{Version: "2.336.0", Found: true, RuntimeDeprecatesAt: ts("2026-11-05T08:04:55Z")},
			want:     StatusHealthy,
			wantDays: ptr(61),
		},
		{
			name:     "warning inside window",
			sched:    Schedule{Version: "2.336.0", Found: true, RuntimeDeprecatesAt: ts("2026-09-15T10:00:00Z")},
			want:     StatusWarning,
			wantDays: ptr(10),
		},
		{
			name:     "warning on boundary day",
			sched:    Schedule{Version: "x", Found: true, RuntimeDeprecatesAt: ts("2026-09-19T10:00:00Z")},
			want:     StatusWarning,
			wantDays: ptr(14),
		},
		{
			name:     "overdue",
			sched:    Schedule{Version: "2.334.0", Found: true, RuntimeDeprecatesAt: ts("2026-08-10T17:08:55Z")},
			want:     StatusOverdue,
			wantDays: ptr(-26),
		},
		{
			name:     "ended earlier today counts as overdue",
			sched:    Schedule{Version: "x", Found: true, RuntimeDeprecatesAt: ts("2026-09-05T09:00:00Z")},
			want:     StatusOverdue,
			wantDays: ptr(-1),
		},
		{
			name:  "unknown when API 404",
			sched: Schedule{Version: "2.320.0", Found: false},
			want:  StatusUnknown,
		},
		{
			name:  "unknown when runner has no version",
			sched: Schedule{Version: "", Found: false},
			want:  StatusUnknown,
		},
		{
			name: "registration blocked flag",
			sched: Schedule{
				Version:                  "2.328.0",
				Found:                    true,
				RuntimeDeprecatesAt:      ts("2025-12-16T17:40:26Z"),
				RegistrationDeprecatesAt: ts("2026-03-16T00:00:00Z"),
			},
			want:    StatusOverdue,
			wantReg: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Assess(tc.sched, now, 14)
			if got.Status != tc.want {
				t.Fatalf("status = %s, want %s", got.Status, tc.want)
			}
			if tc.wantDays != nil {
				if got.DaysLeft == nil {
					t.Fatalf("DaysLeft = nil, want %d", *tc.wantDays)
				}
				if *got.DaysLeft != *tc.wantDays {
					t.Fatalf("DaysLeft = %d, want %d", *got.DaysLeft, *tc.wantDays)
				}
			}
			if got.RegistrationBlocked != tc.wantReg {
				t.Fatalf("RegistrationBlocked = %v, want %v", got.RegistrationBlocked, tc.wantReg)
			}
		})
	}
}

func TestSeverityOrdering(t *testing.T) {
	if !(StatusOverdue.Severity() > StatusWarning.Severity() &&
		StatusWarning.Severity() > StatusUnknown.Severity() &&
		StatusUnknown.Severity() > StatusHealthy.Severity() &&
		StatusHealthy.Severity() == StatusCurrent.Severity()) {
		t.Fatal("severity ordering broken")
	}
}

func ptr(i int) *int { return &i }

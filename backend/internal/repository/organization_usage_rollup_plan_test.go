package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func shanghaiTime(day string, clock string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", day+" "+clock, organizationUsageRollupLocation)
	if err != nil {
		panic(err)
	}
	return t
}

func shanghaiDay(day string) *time.Time {
	t := shanghaiTime(day, "00:00")
	return &t
}

func utcInstant(day, clock string) string {
	return formatOrganizationUsageInstant(shanghaiTime(day, clock))
}

func TestOrganizationUsageRollupPlan_WithoutWatermarkReadsWholeRangeRaw(t *testing.T) {
	start := shanghaiTime("2026-10-01", "00:00")
	end := shanghaiTime("2026-10-23", "15:00")
	plan := buildOrganizationUsageRollupPlan(start, start, end, nil, nil)
	require.Equal(t, "2026-10-01", plan.rollupStart)
	require.Equal(t, "2026-10-01", plan.rollupEnd)
	require.Equal(t, []string{utcInstant("2026-10-01", "00:00")}, plan.rawStarts)
	require.Equal(t, []string{utcInstant("2026-10-23", "15:00")}, plan.rawEnds)
	require.NotNil(t, plan.dirtyDates)
}

func TestOrganizationUsageRollupPlan_ReadsPublishedDaysAndRawTail(t *testing.T) {
	start := shanghaiTime("2026-10-01", "00:00")
	end := shanghaiTime("2026-10-23", "15:00")

	current := buildOrganizationUsageRollupPlan(start, start, end, shanghaiDay("2026-10-23"), nil)
	require.Equal(t, "2026-10-23", current.rollupEnd)
	require.Equal(t, []string{utcInstant("2026-10-23", "00:00")}, current.rawStarts)
	require.Equal(t, []string{utcInstant("2026-10-23", "15:00")}, current.rawEnds)

	lagging := buildOrganizationUsageRollupPlan(start, start, end, shanghaiDay("2026-10-20"), nil)
	require.Equal(t, "2026-10-20", lagging.rollupEnd)
	require.Equal(t, []string{utcInstant("2026-10-20", "00:00")}, lagging.rawStarts)
}

func TestOrganizationUsageRollupPlan_ReadsDirtyDaysRawAndMergesAdjacentRanges(t *testing.T) {
	start := shanghaiTime("2026-10-01", "00:00")
	end := shanghaiTime("2026-10-23", "15:00")
	dirty := []time.Time{
		*shanghaiDay("2026-10-22"), *shanghaiDay("2026-10-05"), *shanghaiDay("2026-10-06"),
		*shanghaiDay("2026-09-30"), *shanghaiDay("2026-10-25"),
	}
	plan := buildOrganizationUsageRollupPlan(start, start, end, shanghaiDay("2026-10-23"), dirty)
	require.Equal(t, []string{"2026-10-05", "2026-10-06", "2026-10-22"}, plan.dirtyDates)
	require.Equal(t, []string{utcInstant("2026-10-05", "00:00"), utcInstant("2026-10-22", "00:00")}, plan.rawStarts)
	require.Equal(t, []string{utcInstant("2026-10-07", "00:00"), utcInstant("2026-10-23", "15:00")}, plan.rawEnds)
}

func TestOrganizationUsageRollupPlan_HistoricalRangeNeedsNoRaw(t *testing.T) {
	start := shanghaiTime("2026-09-01", "00:00")
	end := shanghaiTime("2026-10-01", "00:00")
	plan := buildOrganizationUsageRollupPlan(start, start, end, shanghaiDay("2026-10-23"), nil)
	require.Equal(t, "2026-10-01", plan.rollupEnd)
	require.Empty(t, plan.rawStarts)
	require.Empty(t, plan.rawEnds)
}

func TestOrganizationUsageRollupPlan_AsOfBeforeRangeIsEmpty(t *testing.T) {
	start := shanghaiTime("2026-10-01", "00:00")
	plan := buildOrganizationUsageRollupPlan(start, start, start, shanghaiDay("2026-10-23"), nil)
	require.Equal(t, plan.rollupStart, plan.rollupEnd)
	require.Empty(t, plan.rawStarts)
}

func TestOrganizationUsageRollupPlan_BindsSlotsAsParameters(t *testing.T) {
	plan := organizationUsageRollupPlan{rollupStart: "2026-10-01", rollupEnd: "2026-10-23", dirtyDates: []string{}, rawStarts: []string{}, rawEnds: []string{}}
	query, args := plan.bind(`WITH `+organizationUsageDaysCTE("= ANY($1::bigint[])", "TRUE")+` SELECT 1`, []any{"first"})
	require.NotContains(t, query, "{{")
	require.True(t, strings.Contains(query, ">= $2::date") && strings.Contains(query, "< $3::date"))
	require.Contains(t, query, "<> ALL($4::date[])")
	require.Contains(t, query, "unnest($5::timestamptz[], $6::timestamptz[])")
	require.Equal(t, []any{"first", "2026-10-01", "2026-10-23", pq.Array([]string{}), pq.Array([]string{}), pq.Array([]string{})}, args)

	var nilPlan *organizationUsageRollupPlan
	unchanged, unchangedArgs := nilPlan.bind("SELECT 1", nil)
	require.Equal(t, "SELECT 1", unchanged)
	require.Empty(t, unchangedArgs)
}

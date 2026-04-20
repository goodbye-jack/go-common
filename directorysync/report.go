package directorysync

import (
	"fmt"
	"strings"
	"time"
)

func newReport(options SyncOptions) *SyncReport {
	status := "running"
	if options.DryRun {
		status = "dry_run"
	}
	return &SyncReport{
		BatchNo:    strings.TrimSpace(options.BatchNo),
		DryRun:     options.DryRun,
		StartedAt:  time.Now(),
		TenantCode: strings.TrimSpace(options.TenantCode),
		Status:     status,
		Details:    make([]SyncDetail, 0, 32),
	}
}

func (report *SyncReport) addDetail(detail SyncDetail) {
	if report == nil {
		return
	}
	report.Details = append(report.Details, detail)
	switch detail.Status {
	case DetailStatusSuccess:
		report.SuccessCount++
	case DetailStatusFailed:
		report.FailedCount++
	case DetailStatusWarning:
		report.WarningCount++
	}
}

func (report *SyncReport) finish(status string) {
	if report == nil {
		return
	}
	report.FinishedAt = time.Now()
	report.Status = status
}

// ValidationError 表示同步前校验未通过。
type ValidationError struct {
	Messages []string
}

func (errorValidation ValidationError) Error() string {
	if len(errorValidation.Messages) == 0 {
		return "directorysync validation failed"
	}
	return fmt.Sprintf("directorysync validation failed: %s", strings.Join(errorValidation.Messages, "; "))
}

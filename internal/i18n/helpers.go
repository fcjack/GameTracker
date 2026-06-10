package i18n

import (
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func ImportJobSummary(job *models.ImportJob, locale string) string {
	if job == nil {
		return ""
	}
	switch job.Status {
	case "completed":
		if job.ImportedCount == 0 && job.SkippedCount == 0 {
			return T(locale, "import.no_games_found")
		}
		key := "import.imported"
		if job.ImportedCount != 1 {
			key = "import.imported_plural"
		}
		msg := T(locale, key, job.ImportedCount)
		if job.SkippedCount > 0 {
			msg += T(locale, "import.skipped_suffix", job.SkippedCount)
		}
		return msg
	case "failed":
		if job.ErrorMessage != "" {
			return job.ErrorMessage
		}
		return T(locale, "import.failed")
	default:
		return ""
	}
}

func GroupYearLabel(locale, label string) string {
	if label == "Active" {
		return T(locale, "library.group_active")
	}
	return label
}

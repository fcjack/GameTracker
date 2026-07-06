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
			switch job.Provider {
			case "xbox":
				return T(locale, "import.no_games_found_xbox")
			case "epic":
				return T(locale, "import.no_games_found_epic")
			default:
				return T(locale, "import.no_games_found")
			}
		}
		key := importedSummaryKey(job.Provider, job.ImportedCount != 1)
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

func importedSummaryKey(provider string, plural bool) string {
	switch provider {
	case "xbox":
		if plural {
			return "import.imported_xbox_plural"
		}
		return "import.imported_xbox"
	case "epic":
		if plural {
			return "import.imported_epic_plural"
		}
		return "import.imported_epic"
	default:
		if plural {
			return "import.imported_plural"
		}
		return "import.imported"
	}
}

func GroupYearLabel(locale, label string) string {
	switch label {
	case "Active":
		return T(locale, "library.group_active")
	case "Unknown":
		return T(locale, "library.group_unknown_year")
	default:
		return label
	}
}

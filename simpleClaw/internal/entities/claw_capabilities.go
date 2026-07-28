package entities

type CapabilityID string

const (
	CapabilityWebSearch      CapabilityID = "web_search"
	CapabilityFilesImages    CapabilityID = "files_images"
	CapabilityMemory         CapabilityID = "memory"
	CapabilityGmail          CapabilityID = "gmail"
	CapabilityGoogleCalendar CapabilityID = "google_calendar"
	CapabilityNotion         CapabilityID = "notion"
	CapabilityGitHub         CapabilityID = "github"
	CapabilitySheets         CapabilityID = "sheets"
	CapabilityLinear         CapabilityID = "linear"
	CapabilityTrello         CapabilityID = "trello"
)

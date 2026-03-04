package channels

type DmPolicy string

const (
	DmPairing   DmPolicy = "pairing"
	DmAllowList DmPolicy = "allowlist"
	DmOpen      DmPolicy = "open"
	DmDisabled  DmPolicy = "disabled"
)

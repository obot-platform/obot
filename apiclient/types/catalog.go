package types

import "time"

type Catalog struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	LastSyncTime time.Time `json:"lastSyncTime,omitempty"`
}

type CatalogList List[Catalog]

package types

import "time"

type Monitor struct {
	ID        uint
	CreatedAt time.Time
	Username  string
	Path      string
}

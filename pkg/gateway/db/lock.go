package db

import (
	"gorm.io/gorm"
)

// The values must not change, so replicas running different versions during an upgrade still take the same lock.
const (
	MigrationLock       LockID = 0x6f626f7467776d   // "obotgwm"
	UserCreationLock    LockID = 0x6f626f7455736572 // "obotUser"
	DeviceCreationLock  LockID = 0x6f626f7444657669 // "obotDevi"
	HostedResourcesLock LockID = 0x6f626f744d435043 // "obotMCPC"
)

// LockID identifies a lock that's held for the rest of a transaction.
type LockID int64

// Lock takes the lock for the rest of tx. On Postgres it's an advisory lock, so it holds across replicas. SQLite
// runs a single replica, so there's nothing to take.
func Lock(tx *gorm.DB, id LockID) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(id)).Error
}

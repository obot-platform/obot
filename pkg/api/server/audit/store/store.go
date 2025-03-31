package store

import (
	"fmt"
	"strings"
	"time"
)

type Store interface {
	Persist([]byte) error
}

func filename(host string) string {
	return fmt.Sprintf("%s-%s.log.gz", strings.ReplaceAll(host, ".", "_"), time.Now().Format(time.RFC3339))
}

package store

import (
	"fmt"
	"strings"
	"time"
)

type Store interface {
	Persist([]byte) error
}

func filename(host string, compress bool) string {
	suffix := ".log"
	if compress {
		suffix += ".gz"
	}
	return fmt.Sprintf("%s-%s%s", strings.ReplaceAll(host, ".", "_"), time.Now().Format(time.RFC3339), suffix)
}

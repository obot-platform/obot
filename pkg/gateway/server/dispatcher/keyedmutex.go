package dispatcher

import "sync"

type keyedMutex struct {
	lock  sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: map[string]*sync.Mutex{}}
}

func (k *keyedMutex) Lock(key string) func() {
	k.lock.Lock()
	mutex := k.locks[key]
	if mutex == nil {
		mutex = &sync.Mutex{}
		k.locks[key] = mutex
	}
	k.lock.Unlock()

	mutex.Lock()
	return mutex.Unlock
}

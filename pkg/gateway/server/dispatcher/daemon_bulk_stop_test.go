package dispatcher

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBulkStopProviderDaemons(t *testing.T) {
	dispatcher := &Dispatcher{ports: newPorts()}
	var authStops atomic.Int32
	var modelStops atomic.Int32
	dispatcher.ports.daemonPorts["auth-provider/default/external"] = 10240
	dispatcher.ports.daemonsRunning["auth-provider/default/external"] = func() { authStops.Add(1) }
	dispatcher.ports.usedPorts[10240] = struct{}{}
	dispatcher.ports.daemonPorts["model-provider/default/openai"] = 10241
	dispatcher.ports.daemonsRunning["model-provider/default/openai"] = func() { modelStops.Add(1) }
	dispatcher.ports.usedPorts[10241] = struct{}{}

	dispatcher.StopAllAuthProviderDaemons()
	assert.Equal(t, int32(1), authStops.Load())
	assert.Equal(t, int32(0), modelStops.Load())
	assert.NotContains(t, dispatcher.ports.daemonPorts, "auth-provider/default/external")
	assert.Contains(t, dispatcher.ports.daemonPorts, "model-provider/default/openai")

	dispatcher.StopAllModelProviderDaemons()
	assert.Equal(t, int32(1), modelStops.Load())
	assert.Empty(t, dispatcher.ports.daemonPorts)
	assert.Empty(t, dispatcher.ports.daemonsRunning)
	assert.Empty(t, dispatcher.ports.usedPorts)
}

func TestBulkStopIsSafeWithConcurrentDaemonAccess(t *testing.T) {
	dispatcher := &Dispatcher{ports: newPorts()}
	dispatcher.ports.daemonPorts["model-provider/default/openai"] = 10240
	dispatcher.ports.daemonsRunning["model-provider/default/openai"] = func() {}
	dispatcher.ports.usedPorts[10240] = struct{}{}

	done := make(chan struct{})
	go func() {
		for range 1000 {
			dispatcher.ports.daemonLock.RLock()
			_ = dispatcher.ports.daemonPorts["model-provider/default/openai"]
			dispatcher.ports.daemonLock.RUnlock()
		}
		close(done)
	}()
	dispatcher.StopAllModelProviderDaemons()
	<-done
	assert.Empty(t, dispatcher.ports.daemonPorts)
}

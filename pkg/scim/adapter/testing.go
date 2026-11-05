package adapter

import (
	"testing"
)

// RegisterForTest registers a for the named auth provider and under its type until the test ends, and then restores
// whatever it replaced. Tests of shared code use it for rules that no registered adapter has, such as those of
// adaptertest.Adapter. Tests that register an adapter must not run in parallel with tests that look adapters up.
func RegisterForTest(tb testing.TB, authProviderName string, a Adapter) {
	tb.Helper()

	previousByName, hadName := byAuthProvider[authProviderName]
	previousByType, hadType := byType[a.Type()]
	byAuthProvider[authProviderName] = a
	byType[a.Type()] = a
	tb.Cleanup(func() {
		if hadName {
			byAuthProvider[authProviderName] = previousByName
		} else {
			delete(byAuthProvider, authProviderName)
		}
		if hadType {
			byType[a.Type()] = previousByType
		} else {
			delete(byType, a.Type())
		}
	})
}

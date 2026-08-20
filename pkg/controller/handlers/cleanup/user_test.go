package cleanup

import (
	"reflect"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestSavedPoolIDs(t *testing.T) {
	userDelete := &v1.UserDelete{
		Annotations: map[string]string{
			hostedAgentPoolCleanupAnnotation: `["pool-a","pool-b"]`,
		},
	}
	got, err := savedPoolIDs(userDelete)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"pool-a", "pool-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pool IDs = %#v, want %#v", got, want)
	}
}

func TestSavedPoolIDsRejectsInvalidCheckpoint(t *testing.T) {
	userDelete := &v1.UserDelete{
		Annotations: map[string]string{
			hostedAgentPoolCleanupAnnotation: "not-json",
		},
	}
	if _, err := savedPoolIDs(userDelete); err == nil {
		t.Fatal("expected invalid cleanup checkpoint to fail")
	}
}

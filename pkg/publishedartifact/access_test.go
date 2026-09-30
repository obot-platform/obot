package publishedartifact

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestSubjectsContainUserObotGroup(t *testing.T) {
	subjects := []types.Subject{{Type: types.SubjectTypeObotGroup, ID: "admin"}}
	admin := &user.DefaultInfo{Extra: map[string][]string{"obot_groups": {"admin"}}}
	idp := &user.DefaultInfo{Extra: map[string][]string{"auth_provider_groups": {"admin"}}}

	if !SubjectsContainUser(subjects, admin) {
		t.Fatal("obot group did not match")
	}
	if SubjectsContainUser(subjects, idp) {
		t.Fatal("auth provider group matched obot group")
	}
}

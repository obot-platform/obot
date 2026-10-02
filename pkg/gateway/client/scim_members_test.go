package client

import (
	"crypto/aes"
	"slices"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	encryptaes "k8s.io/apiserver/pkg/storage/value/encrypt/aes"
)

func TestSCIMGroupMembersDecryptTheirUserNames(t *testing.T) {
	c := newLifecycleTestClient(t)
	block, err := aes.NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := encryptaes.NewGCMTransformer(block)
	if err != nil {
		t.Fatal(err)
	}
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			userGroupResource: transformer,
		},
	}

	conn, _ := createTestSCIMConnection(t, c, false)
	alice := provisionTestSCIMUser(t, c, conn, "00u-alice", "alice@example.com")
	bob := provisionTestSCIMUser(t, c, conn, "00u-bob", "bob@example.com")
	group, err := c.CreateSCIMGroup(t.Context(), conn, SCIMGroupInput{
		DisplayName: "Team",
		MemberIDs:   []string{alice.ID, bob.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stored types.SCIMUserBinding
	if err := c.db.WithContext(t.Context()).Where("id = ?", alice.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Encrypted || stored.UserName == "alice@example.com" {
		t.Fatalf("the binding's userName is stored as %q, want it encrypted", stored.UserName)
	}

	want := []SCIMGroupMember{
		{
			ID:       alice.ID,
			UserName: "alice@example.com",
		},
		{
			ID:       bob.ID,
			UserName: "bob@example.com",
		},
	}
	got, err := c.GetSCIMGroup(t.Context(), conn.ID, group.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Members, want) {
		t.Fatalf("members = %+v, want %+v", got.Members, want)
	}

	listed, _, err := c.ListSCIMGroups(t.Context(), conn.ID, SCIMGroupFilter{}, SCIMPage{
		Limit: 10,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || !slices.Equal(listed[0].Members, want) {
		t.Fatalf("listed groups = %+v, want one with members %+v", listed, want)
	}
}

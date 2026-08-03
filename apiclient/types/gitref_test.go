package types

import "testing"

// The ref reaches git as an argument, so the validator is what stands between a
// user-supplied string and git's option parser.
func TestValidateGitRef(t *testing.T) {
	for _, tt := range []struct {
		ref  string
		want bool
	}{
		{"", true},
		{"main", true},
		{"v1.2.3", true},
		{"release/2026-01", true},
		{"9fceb02d0ae598e95dc970b74767f19372d61af8", true},
		{"feature_x+y", true},
		// An argument beginning with "-" is read by git as an option.
		{"--upload-pack=touch /tmp/pwned", false},
		{"-oProxyCommand=x", false},
		// ".." turns a ref into a revision range.
		{"main..evil", false},
		{"refs/heads/main/", false},
		{"/refs/heads/main", false},
		{"main branch", false},
		{"main;rm -rf /", false},
		{"main$(id)", false},
		{"main\nsecond", false},
	} {
		t.Run(tt.ref, func(t *testing.T) {
			err := ValidateGitRef(tt.ref)
			if (err == nil) != tt.want {
				t.Fatalf("ValidateGitRef(%q) error = %v, want valid=%v", tt.ref, err, tt.want)
			}
		})
	}
}

// A ref without a repository names a revision of nothing.
func TestGitRefRequiresRepo(t *testing.T) {
	agent := HostedAgentManifest{Name: "a", HarnessID: "h", GitRef: "main"}
	if err := agent.Validate(); err == nil {
		t.Error("expected an agent with a ref but no repo to be rejected")
	}

	instance := HostedAgentInstanceManifest{Name: "i", GitRef: "main"}
	if err := instance.Validate(); err == nil {
		t.Error("expected an instance with a ref but no repo to be rejected")
	}
}

// AllowUserGitRepo gates the ref as well: without it a user could repoint the
// agent's repository at another revision without supplying a repository.
func TestUserRefNeedsAllowUserGitRepo(t *testing.T) {
	agent := HostedAgentManifest{Name: "a", HarnessID: "h", GitRepo: "https://example.com/a.git"}
	instance := HostedAgentInstanceManifest{Name: "i", GitRepo: "https://example.com/b.git", GitRef: "main"}

	if err := instance.ValidateAgainstAgent(agent); err == nil {
		t.Fatal("expected a user ref to be rejected when the agent forbids user repositories")
	}

	agent.AllowUserGitRepo = true
	if err := instance.ValidateAgainstAgent(agent); err != nil {
		t.Fatalf("expected a user ref to be accepted when allowed: %v", err)
	}
}

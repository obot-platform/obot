package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostedAgentQuestionValidate(t *testing.T) {
	tests := []struct {
		name     string
		question HostedAgentQuestion
		wantErr  string
	}{
		{
			name:     "key required",
			question: HostedAgentQuestion{},
			wantErr:  "question key is required",
		},
		{
			name:     "bare key defaults to string",
			question: HostedAgentQuestion{Key: "greeting"},
		},
		{
			name:     "select needs options",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSelect},
			wantErr:  "select requires at least one option",
		},
		{
			name:     "select with options",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSelect, Options: []string{"a", "b"}},
		},
		{
			name:     "select rejects duplicate options",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSelect, Options: []string{"a", "a"}},
			wantErr:  "duplicate option a",
		},
		{
			name:     "select rejects empty option",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSelect, Options: []string{""}},
			wantErr:  "select options cannot be empty",
		},
		{
			name:     "options rejected for non-select",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeString, Options: []string{"a"}},
			wantErr:  "options are only valid for select",
		},
		{
			name:     "unknown type",
			question: HostedAgentQuestion{Key: "k", Type: "wat"},
			wantErr:  "invalid type wat",
		},
		{
			name:     "default must satisfy the type",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeNumber, Default: "abc"},
			wantErr:  "invalid default",
		},
		{
			name:     "valid default",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeNumber, Default: "42"},
		},
		{
			name:     "default outside select options",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSelect, Options: []string{"a"}, Default: "z"},
			wantErr:  "invalid default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.question.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestHostedAgentQuestionValidateAnswer(t *testing.T) {
	tests := []struct {
		name     string
		question HostedAgentQuestion
		answer   string
		wantErr  string
	}{
		{name: "empty is deferred to required check", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeNumber}, answer: ""},
		{name: "number accepts int", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeNumber}, answer: "10"},
		{name: "number accepts float", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeNumber}, answer: "10.5"},
		{name: "number rejects text", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeNumber}, answer: "ten", wantErr: "must be a number"},
		{name: "boolean accepts true", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeBoolean}, answer: "true"},
		{name: "boolean rejects yes", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeBoolean}, answer: "yes", wantErr: "must be true or false"},
		{
			name:     "select accepts listed option",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSelect, Options: []string{"a", "b"}},
			answer:   "b",
		},
		{
			name:     "select rejects unlisted option",
			question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSelect, Options: []string{"a", "b"}},
			answer:   "c",
			wantErr:  "must be one of: a, b",
		},
		{name: "schedule accepts 5 fields", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSchedule}, answer: "0 3 * * *"},
		{name: "schedule accepts 6 fields", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSchedule}, answer: "0 0 3 * * *"},
		{name: "schedule rejects too few fields", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeSchedule}, answer: "0 3", wantErr: "valid cron expression"},
		{name: "string accepts anything", question: HostedAgentQuestion{Key: "k", Type: HostedAgentQuestionTypeString}, answer: "whatever"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.question.ValidateAnswer(tt.answer)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestHostedAgentManifestValidateAnswers(t *testing.T) {
	manifest := HostedAgentManifest{
		Name:      "a",
		HarnessID: "hrn1x",
		Questions: []HostedAgentQuestion{
			{Key: "required_one", Required: true},
			{Key: "optional_one"},
			{Key: "with_default", Required: true, Default: "d"},
			{Key: "count", Type: HostedAgentQuestionTypeNumber},
		},
	}

	t.Run("missing required answer", func(t *testing.T) {
		err := manifest.ValidateAnswers(map[string]string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "answer for required_one is required")
	})

	t.Run("required satisfied by default", func(t *testing.T) {
		err := manifest.ValidateAnswers(map[string]string{"required_one": "x"})
		require.NoError(t, err)
	})

	t.Run("unknown answer rejected", func(t *testing.T) {
		err := manifest.ValidateAnswers(map[string]string{"required_one": "x", "bogus": "y"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown answer: bogus")
	})

	t.Run("typed answer validated", func(t *testing.T) {
		err := manifest.ValidateAnswers(map[string]string{"required_one": "x", "count": "abc"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "answer for count")
	})

	t.Run("no questions rejects any answer", func(t *testing.T) {
		empty := HostedAgentManifest{Name: "a", HarnessID: "hrn1x"}
		err := empty.ValidateAnswers(map[string]string{"anything": "x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown answer: anything")
	})
}

func TestHostedAgentManifestApplyAnswerDefaults(t *testing.T) {
	manifest := HostedAgentManifest{
		Questions: []HostedAgentQuestion{
			{Key: "a", Default: "default-a"},
			{Key: "b", Default: "default-b"},
			{Key: "c"},
		},
	}

	got := manifest.ApplyAnswerDefaults(map[string]string{"b": "user-b"})

	assert.Equal(t, "default-a", got["a"], "unanswered question should take its default")
	assert.Equal(t, "user-b", got["b"], "user answer must win over the default")
	assert.NotContains(t, got, "c", "question without a default should stay absent")
}

func TestHostedAgentManifestValidate(t *testing.T) {
	t.Run("negative max instances rejected", func(t *testing.T) {
		m := HostedAgentManifest{Name: "a", HarnessID: "hrn1x", MaxInstancesPerUser: -1}
		err := m.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maxInstancesPerUser must be greater than or equal to 0")
	})

	t.Run("duplicate question keys", func(t *testing.T) {
		m := HostedAgentManifest{
			Name: "a", HarnessID: "hrn1x",
			Questions: []HostedAgentQuestion{{Key: "k"}, {Key: "k"}},
		}
		err := m.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate question key: k")
	})

	t.Run("valid agent", func(t *testing.T) {
		m := HostedAgentManifest{
			Name: "a", HarnessID: "hrn1x", MaxInstancesPerUser: 2,
			Questions:       []HostedAgentQuestion{{Key: "schedule", Type: HostedAgentQuestionTypeSchedule, Default: "0 3 * * *"}},
			AllowUserSkills: true,
		}
		require.NoError(t, m.Validate())
	})
}

func TestHostedAgentInstanceManifestValidateAgainstAgent(t *testing.T) {
	agent := HostedAgentManifest{
		Name: "a", HarnessID: "hrn1x",
		Questions: []HostedAgentQuestion{{Key: "k", Required: true}},
	}

	t.Run("user resources rejected when not allowed", func(t *testing.T) {
		instance := HostedAgentInstanceManifest{
			Name:    "n",
			Answers: map[string]string{"k": "v"},
			Skills:  []string{"sk1abc"},
		}
		err := instance.ValidateAgainstAgent(agent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not allow user-defined skills")
	})

	t.Run("user resources accepted when allowed", func(t *testing.T) {
		allowing := agent
		allowing.AllowUserSkills = true
		instance := HostedAgentInstanceManifest{
			Name:    "n",
			Answers: map[string]string{"k": "v"},
			Skills:  []string{"sk1abc"},
		}
		require.NoError(t, instance.ValidateAgainstAgent(allowing))
	})

	t.Run("each kind gated independently", func(t *testing.T) {
		allowing := agent
		allowing.AllowUserSkills = true
		instance := HostedAgentInstanceManifest{
			Name:    "n",
			Answers: map[string]string{"k": "v"},
			Models:  []string{"m1abc"},
		}
		err := instance.ValidateAgainstAgent(allowing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not allow user-defined models")
	})

	t.Run("missing required answer surfaces", func(t *testing.T) {
		instance := HostedAgentInstanceManifest{Name: "n"}
		err := instance.ValidateAgainstAgent(agent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "answer for k is required")
	})
}

func TestHostedAgentManifestScheduleAnswers(t *testing.T) {
	manifest := HostedAgentManifest{
		Questions: []HostedAgentQuestion{
			{Key: "cron", Type: HostedAgentQuestionTypeSchedule},
			{Key: "other", Type: HostedAgentQuestionTypeString},
			{Key: "blank_cron", Type: HostedAgentQuestionTypeSchedule},
		},
	}

	got := manifest.ScheduleAnswers(map[string]string{"cron": "0 3 * * *", "other": "0 3 * * *"})

	assert.Equal(t, map[string]string{"cron": "0 3 * * *"}, got,
		"only non-empty schedule answers should be returned for cron parsing")
}

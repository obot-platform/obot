package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestListSkillsEncodesQueryParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/skills", r.URL.Path)
		require.Equal(t, "github review", r.URL.Query().Get("q"))
		require.Equal(t, "25", r.URL.Query().Get("limit"))
		require.NoError(t, json.NewEncoder(w).Encode(types.SkillList{
			Items: []types.Skill{{
				Metadata:      types.Metadata{ID: "sk1"},
				SkillManifest: types.SkillManifest{Name: "github-review"},
			}},
		}))
	}))
	defer server.Close()

	result, err := (&Client{BaseURL: server.URL}).ListSkills(context.Background(), "github review", 25)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "sk1", result.Items[0].ID)
	require.Equal(t, "github-review", result.Items[0].Name)
}

func TestListSkillsOmitsEmptyParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "", r.URL.RawQuery)
		require.NoError(t, json.NewEncoder(w).Encode(types.SkillList{}))
	}))
	defer server.Close()

	_, err := (&Client{BaseURL: server.URL}).ListSkills(context.Background(), "", 0)
	require.NoError(t, err)
}

func TestGetSkillEscapesIDAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/skills/skill%2Fwith%20space", r.URL.EscapedPath())
		require.NoError(t, json.NewEncoder(w).Encode(types.Skill{
			Metadata:      types.Metadata{ID: "skill/with space"},
			SkillManifest: types.SkillManifest{Name: "reviewer"},
		}))
	}))
	defer server.Close()

	skill, err := (&Client{BaseURL: server.URL}).GetSkill(context.Background(), "skill/with space")
	require.NoError(t, err)
	require.Equal(t, "skill/with space", skill.ID)
	require.Equal(t, "reviewer", skill.Name)
}

func TestDownloadSkillReturnsRawBytes(t *testing.T) {
	want := []byte("zip bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/skills/sk1/download", r.URL.Path)
		w.Header().Set("Content-Type", "application/zip")
		_, err := w.Write(want)
		require.NoError(t, err)
	}))
	defer server.Close()

	got, err := (&Client{BaseURL: server.URL}).DownloadSkill(context.Background(), "sk1")
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDownloadSkillRejectsOversizedContentLength(t *testing.T) {
	oldMax := maxSkillDownloadBytes
	maxSkillDownloadBytes = 8
	t.Cleanup(func() { maxSkillDownloadBytes = oldMax })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(maxSkillDownloadBytes+1))
	}))
	defer server.Close()

	_, err := (&Client{BaseURL: server.URL}).DownloadSkill(context.Background(), "sk1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "skill download exceeds maximum size")
}

func TestDownloadSkillRejectsOversizedBody(t *testing.T) {
	oldMax := maxSkillDownloadBytes
	maxSkillDownloadBytes = 8
	t.Cleanup(func() { maxSkillDownloadBytes = oldMax })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, err := io.Copy(w, io.LimitReader(repeatingReader('x'), maxSkillDownloadBytes+1))
		require.NoError(t, err)
	}))
	defer server.Close()

	_, err := (&Client{BaseURL: server.URL}).DownloadSkill(context.Background(), "sk1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "skill download exceeds maximum size")
}

type repeatingReader byte

func (r repeatingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

func TestSkillHelpersPropagateHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL}
	_, err := client.ListSkills(context.Background(), "anything", 10)
	require.Error(t, err)

	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusTeapot, httpErr.Code)
}

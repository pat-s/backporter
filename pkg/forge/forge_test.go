package forge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		forgeType string
		token     string
		wantError bool
		wantName  string
	}{
		{
			name:      "github forge",
			forgeType: "github",
			token:     "test-token",
			wantError: false,
			wantName:  "github",
		},
		{
			name:      "github forge without token",
			forgeType: "github",
			token:     "",
			wantError: false,
			wantName:  "github",
		},
		{
			name:      "forgejo forge without URL",
			forgeType: "forgejo",
			token:     "test-token",
			wantError: true,
			wantName:  "",
		},
		{
			name:      "unknown forge type",
			forgeType: "gitlab",
			token:     "test-token",
			wantError: true,
			wantName:  "",
		},
		{
			name:      "empty forge type",
			forgeType: "",
			token:     "",
			wantError: true,
			wantName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forge, err := New(tt.forgeType, tt.token)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, forge)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, forge)
				assert.Equal(t, tt.wantName, forge.Name())
			}
		})
	}
}

func TestPRInfoIsSquashMerge(t *testing.T) {
	tests := []struct {
		name     string
		prInfo   *PRInfo
		expected bool
	}{
		{
			name: "squash merge",
			prInfo: &PRInfo{
				Squashed: true,
			},
			expected: true,
		},
		{
			name: "not squash merge",
			prInfo: &PRInfo{
				Squashed: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.prInfo.IsSquashMerge()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubName(t *testing.T) {
	gh, err := NewGitHub("test-token")
	require.NoError(t, err)
	assert.Equal(t, "github", gh.Name())
}

func TestForgejoName(t *testing.T) {
	fg := NewForgejo("https://codeberg.org", "test-token")
	assert.Equal(t, "forgejo", fg.Name())
}

func TestPRInfoHasBackportLabel(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected bool
	}{
		{
			name:     "exact backport label",
			labels:   []string{"backport"},
			expected: true,
		},
		{
			name:     "backport with prefix",
			labels:   []string{"needs-backport"},
			expected: true,
		},
		{
			name:     "backport with suffix",
			labels:   []string{"backport-needed"},
			expected: true,
		},
		{
			name:     "backport uppercase",
			labels:   []string{"BACKPORT"},
			expected: true,
		},
		{
			name:     "backport mixed case",
			labels:   []string{"BackPort"},
			expected: true,
		},
		{
			name:     "backport among other labels",
			labels:   []string{"bug", "backport", "priority:high"},
			expected: true,
		},
		{
			name:     "no backport label",
			labels:   []string{"bug", "enhancement"},
			expected: false,
		},
		{
			name:     "empty labels",
			labels:   []string{},
			expected: false,
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PRInfo{Labels: tt.labels}
			result := pr.HasBackportLabel()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestForgejoCommentOnPR(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var payload struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotBody = payload.Body
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	f := NewForgejo(srv.URL, "secret")
	err := f.CommentOnPR(t.Context(), "owner", "repo", 42, "hello")
	require.NoError(t, err)
	assert.Equal(t, "POST /api/v1/repos/owner/repo/issues/42/comments", gotPath)
	assert.Equal(t, "token secret", gotAuth)
	assert.Equal(t, "hello", gotBody)
}

func TestForgejoCommentOnPRError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"token lacks scope"}`))
	}))
	defer srv.Close()

	err := NewForgejo(srv.URL, "").CommentOnPR(t.Context(), "owner", "repo", 42, "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token lacks scope")
}

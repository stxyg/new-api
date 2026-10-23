package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogOtherScopesAndMerges(t *testing.T) {
	var other LogOther

	assert.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	other.MergePublic(map[string]any{
		"zero": 0,
	})
	assert.True(t, other.SetAdmin("use_channel", []string{"channel-a"}))
	other.MergeAdmin(map[string]any{
		"rejected": false,
	})
	assert.True(t, other.SetRoot("upstream_request_id", "upstream-private"))
	other.MergeRoot(map[string]any{
		"generation": 0,
	})
	assert.True(t, other.SetAudit("method", "POST"))
	other.MergeAudit(map[string]any{
		"success": false,
	})

	require.JSONEq(t, `{
		"request_path": "/v1/chat/completions",
		"zero": 0,
		"admin_info": {
			"use_channel": ["channel-a"],
			"rejected": false
		},
		"root_info": {
			"upstream_request_id": "upstream-private",
			"generation": 0
		},
		"audit_info": {
			"method": "POST",
			"success": false
		}
	}`, other.JSONString())
}

func TestLogOtherRejectsSensitivePublicFields(t *testing.T) {
	other := NewLogOther()

	for _, key := range []string{
		"admin_info",
		"root_info",
		"audit_info",
		"channel_id",
		"channel_name",
		"channel_type",
		"reject_reason",
	} {
		assert.False(t, other.SetPublic(key, "must-not-leak"), key)
	}
	// Upstream-model diagnostics are allowed in the stored public scope; the
	// user-facing projection, not the writer, hides them from log owners.
	for _, key := range []string{
		"upstream_model_name",
		"response_model",
		"is_model_mapped",
	} {
		assert.True(t, other.SetPublic(key, "operator-only"), key)
	}
	other.MergePublic(map[string]any{
		"request_path": "/v1/responses",
		"channel_name": "still-must-not-leak",
		"admin_info":   map[string]any{"secret": true},
	})

	require.JSONEq(t, `{
		"request_path": "/v1/responses",
		"upstream_model_name": "operator-only",
		"response_model": "operator-only",
		"is_model_mapped": "operator-only"
	}`, other.JSONString())
	require.JSONEq(t, `{}`, NewLogOther().JSONString())
}

func TestLogOtherUserProjectionStripsUpstreamModelMetadata(t *testing.T) {
	// Historical logs (and any future stray writer) keep these keys in the
	// public top level of the stored JSON. The user-facing projection must
	// remove them so log owners cannot see the channel's upstream model.
	stored := `{
		"request_path": "/v1/chat/completions",
		"upstream_model_name": "upstream-secret",
		"is_model_mapped": true,
		"response_model": {"requested_model": "m", "upstream_model": "upstream-secret", "returned_model": "upstream-secret"},
		"admin_info": {"use_channel": ["c1"]}
	}`

	userView := formatLogOtherJSON(stored, logOtherVisibilityUser)
	require.JSONEq(t, `{"request_path": "/v1/chat/completions"}`, userView)

	// Admins keep the diagnostics: admin_info survives and legacy top-level
	// keys are left untouched for backward compatibility with stored rows.
	adminView := formatLogOtherJSON(stored, logOtherVisibilityAdmin)
	assert.Contains(t, adminView, "upstream-secret")
	assert.Contains(t, adminView, "use_channel")
}

func TestLogOtherJSONStringDoesNotMutateReceiver(t *testing.T) {
	other := NewLogOther()
	require.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	require.True(t, other.SetAdmin("rejected", false))

	before := other.Snapshot()
	first := other.JSONString()
	after := other.Snapshot()
	second := other.JSONString()

	require.Equal(t, before, after)
	require.Equal(t, first, second)
}

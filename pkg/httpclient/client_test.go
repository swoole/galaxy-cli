package httpclient

import (
	"galaxy/protoc"
	"testing"
)

func TestNormalizeScopeParameters(t *testing.T) {
	data, ok := normalizeScopeParameters(&protoc.ProjectProfileReq{
		OrgId: 1, GroupId: 2, ProjectId: 3,
	}).(map[string]interface{})
	if !ok {
		t.Fatalf("expected a parameter map, got %T", data)
	}
	for key, expected := range map[string]uint32{"org": 1, "group": 2, "project": 3} {
		if value, exists := data[key]; !exists || value != expected {
			t.Fatalf("expected %s=%v, got %#v", key, expected, data)
		}
	}
	for _, key := range []string{"org_id", "group_id", "project_id"} {
		if _, exists := data[key]; exists {
			t.Fatalf("database-style scope parameter %s leaked into request: %#v", key, data)
		}
	}
}

func TestRedactRequestForLog(t *testing.T) {
	data, ok := redactRequestForLog(map[string]interface{}{
		"org": 1, "content": "SECRET=1", "files": map[string]string{"secret:key": "value"},
	}).(map[string]interface{})
	if !ok {
		t.Fatalf("expected a parameter map, got %T", data)
	}
	if data["content"] != "[REDACTED]" || data["files"] != "[REDACTED]" {
		t.Fatalf("sensitive migration data leaked into debug log payload: %#v", data)
	}
	if data["org"] != 1 {
		t.Fatalf("non-sensitive fields should be preserved: %#v", data)
	}
}

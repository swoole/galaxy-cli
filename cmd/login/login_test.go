package login

import (
	"galaxy/pkg/httpclient"
	"testing"
)

func TestUnSecret(t *testing.T) {
	const key = "test-host-id"
	secret, err := httpclient.Secret("user@example.com", "test-password", key)
	if err != nil {
		t.Fatal(err)
	}
	username, password, err := httpclient.UnSecret(secret, key)
	if err != nil {
		t.Fatal(err)
	}
	if username != "user@example.com" || password != "test-password" {
		t.Fatalf("unexpected credentials: username=%q password=%q", username, password)
	}
}

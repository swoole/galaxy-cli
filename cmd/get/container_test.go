package get

import (
	containerservice "galaxy/service/container"
	"galaxy/service/instance"
	"testing"
)

func TestSelectContainerRuntime(t *testing.T) {
	runtimes := []*instance.Runtime{{ID: 5, Name: "web", ServiceName: "cg-1-9-1-web", RuntimeRef: "service-id"}}
	for _, value := range []string{"web", "cg-1-9-1-web", "service-id", "5"} {
		selected, err := selectContainerRuntime(runtimes, value)
		if err != nil || selected != runtimes[0] {
			t.Fatalf("selector %q did not resolve runtime: selected=%#v err=%v", value, selected, err)
		}
	}
}

func TestContainerRuntime(t *testing.T) {
	runtime := &instance.Runtime{ID: 5, Name: "web", ServiceName: "cg-1-9-1-web", RuntimeRef: "service-id"}
	cases := []*containerservice.Container{
		{ServiceID: "service-id"},
		{ServiceName: "cg-1-9-1-web"},
		{Name: "cg-1-9-1-web.1.task"},
	}
	for _, item := range cases {
		if got := containerRuntime(item, []*instance.Runtime{runtime}); got != runtime {
			t.Fatalf("container %#v did not resolve runtime", item)
		}
	}
}

func TestShortContainerID(t *testing.T) {
	if got := shortContainerID("1234567890abcdef"); got != "1234567890ab" {
		t.Fatalf("unexpected short id %q", got)
	}
}

func TestImageWithoutDigest(t *testing.T) {
	tests := map[string]string{
		"mysql:8.0@sha256:7dcddc01f13b":                    "mysql:8.0",
		"registry.example.com/team/app:v1@sha256:deadbeef": "registry.example.com/team/app:v1",
		"redis:8.8": "redis:8.8",
	}
	for input, expected := range tests {
		if got := imageWithoutDigest(input); got != expected {
			t.Fatalf("imageWithoutDigest(%q) = %q, want %q", input, got, expected)
		}
	}
}

package get

import "testing"

func TestClusterStatus(t *testing.T) {
	cases := map[int]string{0: "创建中", 1: "初始化中", 2: "待连接", 3: "就绪", 4: "离线", 9: "异常", 99: "未知"}
	for status, expected := range cases {
		if got := clusterStatus(status); got != expected {
			t.Fatalf("status %d: expected %q, got %q", status, expected, got)
		}
	}
}

func TestClusterConnection(t *testing.T) {
	cases := map[string]string{
		"agent-ssh://cluster-1":       "Agent SSH",
		"agent-local://cluster-1":     "Agent 本地",
		"agent://cluster-1":           "Agent 本地",
		"https://127.0.0.1:2376":      "HTTPS/TLS",
		"tcp://127.0.0.1:2375":        "TCP",
		"http://127.0.0.1:2375":       "TCP",
		"unix:///var/run/docker.sock": "Unix Socket",
		"":                            "-",
	}
	for endpoint, expected := range cases {
		if got := clusterConnection(endpoint); got != expected {
			t.Fatalf("endpoint %q: expected %q, got %q", endpoint, expected, got)
		}
	}
}

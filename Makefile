# Galaxy CLI 与 Galaxy Agent 构建入口，用法见 README.md「构建环境」。

CGO_ENABLED ?= 0
VERSION     ?= dev
GOARCH      ?= $(shell go env GOARCH)

# AgentImage 会通过 -ldflags 写入二进制，成为 `galaxy agent install` 的默认镜像。
# 发布时传入实际仓库地址，例如：
#   make build AgentImage=registry.example.com/galaxy/galaxy-agent:1.0.3
AgentImage ?= galaxy-agent:$(VERSION)

# Agent 镜像内的 Go Module 下载与校验地址，可使用企业内部代理覆盖。
GoProxy ?= https://goproxy.cn,direct
GoSumDB ?= sum.golang.google.cn

LDFLAGS = -s -w \
	-X galaxy/pkg/buildVariable.BuildVersion=$(VERSION) \
	-X galaxy/pkg/buildVariable.AgentImage=$(AgentImage)

.PHONY: build agent agent-image test clean

# 用户 CLI：静态可执行文件，不依赖目标机器的 glibc、musl 等用户态动态库。
build:
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags="$(LDFLAGS)" -o galaxy .

# 节点 Agent 二进制（Linux）。
agent:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=$(GOARCH) \
		go build -trimpath -ldflags="$(LDFLAGS)" -o bin/galaxy-agent-linux-$(GOARCH) ./cmd/galaxy-agent

# Agent 容器镜像：默认构建当前 Docker 主机的平台，不要求安装 Buildx。
agent-image:
	docker build -f Dockerfile.agent \
		--build-arg VERSION=$(VERSION) \
		--build-arg AGENT_IMAGE=$(AgentImage) \
		--build-arg GOPROXY=$(GoProxy) \
		--build-arg GOSUMDB=$(GoSumDB) \
		-t $(AgentImage) .

test:
	go test ./...

clean:
	rm -f galaxy bin/galaxy-agent-linux-*

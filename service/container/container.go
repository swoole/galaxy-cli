package container

import (
	"encoding/base64"
	"fmt"
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
	terminal "galaxy/pkg/utils/term"
	"github.com/AlecAivazis/survey/v2"
	"github.com/gorilla/websocket"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *httpclient.HttpClient
}
type Container struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	ServiceID   string `json:"service_id"`
	ServiceName string `json:"service_name"`
	TaskID      string `json:"task_id"`
	NodeID      string `json:"node_id"`
	State       string `json:"state"`
	Status      string `json:"status"`
	Health      string `json:"health"`
	CreatedAt   int64  `json:"created_at"`
}
type ContainerList struct {
	Containers []*Container `json:"containers"`
}
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}
type File struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}
type FileList struct {
	Pwd   string  `json:"pwd"`
	Files []*File `json:"files"`
}
type FileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	Size     int64  `json:"size"`
}
type Ticket struct {
	Ticket        string `json:"ticket"`
	WebsocketPath string `json:"websocket_path"`
}

func NewService(flags *galaxycfg.ConfigFlags) *Service {
	return &Service{cfgFlags: flags, client: httpclient.NewHttpClient(flags)}
}
func scope(orgID, groupID, projectID, clusterID uint32) map[string]interface{} {
	return map[string]interface{}{"org": orgID, "group": groupID, "project": projectID, "cluster_id": clusterID}
}

func (s *Service) Containers(orgID, groupID, projectID, clusterID uint32) ([]*Container, error) {
	var rsp *ContainerList
	if err := s.client.Get("/cluster/swarm/containers", scope(orgID, groupID, projectID, clusterID), &rsp); err != nil {
		return nil, err
	}
	return rsp.Containers, nil
}

// ServiceContainers 返回一个 Swarm Service 在全部节点上的容器。
func (s *Service) ServiceContainers(orgID, groupID, projectID, clusterID uint32, serviceID string) ([]*Container, error) {
	params := scope(orgID, groupID, projectID, clusterID)
	params["service_id"] = serviceID
	var rsp *ContainerList
	if err := s.client.Get("/cluster/swarm/service-containers", params, &rsp); err != nil {
		return nil, err
	}
	return rsp.Containers, nil
}

func (s *Service) Select(orgID, groupID, projectID, clusterID uint32, preferred string) (*Container, error) {
	containers, err := s.Containers(orgID, groupID, projectID, clusterID)
	if err != nil {
		return nil, err
	}
	var running []*Container
	for _, item := range containers {
		if item.State != "running" {
			continue
		}
		if preferred != "" && (item.Name == preferred || item.ID == preferred || strings.HasPrefix(item.ID, preferred)) {
			return item, nil
		}
		running = append(running, item)
	}
	if preferred != "" {
		return nil, fmt.Errorf("未找到运行中的容器 %q", preferred)
	}
	if len(running) == 0 {
		return nil, fmt.Errorf("当前项目在所选集群中没有运行中的容器")
	}
	if len(running) == 1 {
		return running[0], nil
	}
	titles := make([]string, len(running))
	for i, item := range running {
		titles[i] = fmt.Sprintf("%s [%s]", item.Name, shortID(item.ID))
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: "请选择容器", Options: titles, Default: titles[0]}, &selected); err != nil {
		return nil, err
	}
	return running[selected], nil
}
func (s *Service) Exec(orgID, groupID, projectID, clusterID uint32, containerID string, command []string, workingDir string) (*ExecResult, error) {
	params := scope(orgID, groupID, projectID, clusterID)
	params["container_id"] = containerID
	params["command"] = command
	params["working_dir"] = workingDir
	var rsp *ExecResult
	if err := s.client.Post("/cluster/swarm/container-exec", params, &rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}
func (s *Service) ListFiles(orgID, groupID, projectID, clusterID uint32, containerID, path string) (*FileList, error) {
	params := scope(orgID, groupID, projectID, clusterID)
	params["container_id"] = containerID
	params["path"] = path
	var rsp *FileList
	if err := s.client.Get("/cluster/swarm/container-list-files", params, &rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}
func (s *Service) ReadFile(orgID, groupID, projectID, clusterID uint32, containerID, path string) ([]byte, error) {
	params := scope(orgID, groupID, projectID, clusterID)
	params["container_id"] = containerID
	params["path"] = path
	var rsp *FileContent
	if err := s.client.Get("/cluster/swarm/container-read-file", params, &rsp); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(rsp.Content))
}
func (s *Service) WriteFile(orgID, groupID, projectID, clusterID uint32, containerID, path string, content []byte) error {
	if len(content) == 0 {
		result, err := s.Exec(
			orgID,
			groupID,
			projectID,
			clusterID,
			containerID,
			[]string{"sh", "-c", `: > "$1"`, "galaxy-sync", path},
			"",
		)
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("写入空文件失败: %s", result.Stderr)
		}
		return nil
	}
	params := scope(orgID, groupID, projectID, clusterID)
	params["container_id"] = containerID
	params["path"] = path
	params["content"] = base64.StdEncoding.EncodeToString(content)
	params["encoding"] = "base64"
	return s.client.Post("/cluster/swarm/container-write-file", params, nil)
}
func (s *Service) Interactive(orgID, groupID, projectID, clusterID uint32, containerID string, command []string, streams galaxycfg.IOStreams) error {
	params := scope(orgID, groupID, projectID, clusterID)
	params["container_id"] = containerID
	var ticket *Ticket
	if err := s.client.Post("/cluster/swarm/terminal-ticket", params, &ticket); err != nil {
		return err
	}
	base, err := url.Parse(s.cfgFlags.GetAPIServer())
	if err != nil {
		return err
	}
	if base.Scheme == "https" {
		base.Scheme = "wss"
	} else {
		base.Scheme = "ws"
	}
	base.Path = "/" + strings.TrimLeft(ticket.WebsocketPath, "/")
	query := base.Query()
	query.Set("ticket", ticket.Ticket)
	base.RawQuery = query.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(base.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	if len(command) > 0 && !isShell(command) {
		parts := make([]string, len(command))
		for i, arg := range command {
			parts[i] = strconv.Quote(arg)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte(strings.Join(parts, " ")+"; exit\n")); err != nil {
			return err
		}
	}
	tty := terminal.TTY{In: streams.In, Out: streams.Out, Raw: true, TryDev: true}
	return tty.Safe(func() error {
		done := make(chan error, 1)
		go func() {
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					done <- err
					return
				}
				if _, err := streams.Out.Write(data); err != nil {
					done <- err
					return
				}
			}
		}()
		go func() {
			buffer := make([]byte, 4096)
			for {
				n, err := streams.In.Read(buffer)
				if n > 0 {
					if writeErr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()
		err := <-done
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || err == io.EOF {
			return nil
		}
		return err
	})
}
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
func isShell(command []string) bool {
	if len(command) != 1 {
		return false
	}
	value := command[0]
	if slash := strings.LastIndex(value, "/"); slash >= 0 {
		value = value[slash+1:]
	}
	return value == "sh" || value == "bash" || value == "ash" || value == "csh" || value == "dash"
}

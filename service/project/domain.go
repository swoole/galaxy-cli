package project

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
)

type RouteList struct {
	Data []*Route `json:"data"`
}
type Route struct {
	ID            uint32    `json:"id"`
	Hostname      string    `json:"hostname"`
	PathPrefix    string    `json:"path_prefix"`
	TargetService string    `json:"target_service"`
	TargetPort    uint32    `json:"target_port"`
	TLSEnabled    bool      `json:"tls_enabled"`
	HTTPSRedirect bool      `json:"https_redirect"`
	CertificateID uint32    `json:"certificate_id"`
	Status        string    `json:"status"`
	Source        string    `json:"source"`
	Cluster       *RouteRef `json:"cluster"`
	CreatedAt     int64     `json:"created_at"`
}

func (that *Service) UpdateRoute(orgID, groupID, projectID, clusterID, routeID uint32, source string, updates map[string]interface{}) error {
	if source == "" {
		source = "project"
	}
	params := map[string]interface{}{"org": orgID, "group": groupID, "project": projectID, "cluster_id": clusterID, "vhost_id": routeID, "source": source}
	for key, value := range updates {
		params[key] = value
	}
	return that.client.Put("/cluster/swarm/web-gateway/vhosts", params, nil)
}

type RouteRef struct {
	ID    uint32 `json:"id"`
	Title string `json:"title"`
}

func (that *Service) Routes(orgID, groupID, projectID uint32) ([]*Route, error) {
	var rsp *RouteList
	err := that.client.Get("/deploy/route", map[string]interface{}{"org": orgID, "group": groupID, "project": projectID}, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.Data, nil
}

func (that *Service) CreateRoute(orgID, groupID, projectID, clusterID uint32, hostname, pathPrefix, targetService string, targetPort, certificateID uint32, tlsEnabled, httpsRedirect bool) (*Route, error) {
	var rsp struct {
		Vhost *Route `json:"vhost"`
	}
	err := that.client.Post("/cluster/swarm/web-gateway/vhosts", map[string]interface{}{
		"org": orgID, "group": groupID, "project": projectID, "cluster_id": clusterID,
		"hostname": hostname, "path_prefix": pathPrefix, "path_match": "prefix",
		"target_service": targetService, "target_port": targetPort, "upstream_scheme": "http",
		"tls_enabled": tlsEnabled, "certificate_id": certificateID, "https_redirect": httpsRedirect, "enabled": true,
	}, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.Vhost, nil
}

func (that *Service) DeleteRoute(orgID, groupID, projectID, clusterID, routeID uint32, source string) error {
	if source == "" {
		source = "project"
	}
	return that.client.Delete("/cluster/swarm/web-gateway/vhosts", map[string]interface{}{
		"org": orgID, "group": groupID, "project": projectID, "cluster_id": clusterID,
		"vhost_id": routeID, "source": source,
	}, nil)
}

func (that *Service) SelectRoute(msg string, routes []*Route, hostname, path string) (*Route, error) {
	var matches []*Route
	for _, route := range routes {
		if hostname != "" && route.Hostname != hostname {
			continue
		}
		if path != "" && route.PathPrefix == path {
			return route, nil
		}
		matches = append(matches, route)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("未找到匹配的项目路由")
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	titles := make([]string, len(matches))
	for i, route := range matches {
		titles[i] = fmt.Sprintf("%s%s -> %s:%d", route.Hostname, route.PathPrefix, route.TargetService, route.TargetPort)
	}
	selected := 0
	if err := survey.AskOne(&survey.Select{Message: msg, Options: titles, Default: titles[0]}, &selected); err != nil {
		return nil, err
	}
	return matches[selected], nil
}

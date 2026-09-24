package types

import (
	"bytes"
	"fmt"
	"io"
	"net"
)

const disabledPrefix = "# "

type Profile struct {
	Name   string
	Status Status
	IPList []string
	Routes map[string]*Route
}

func (p *Profile) appendIP(ip string) {
	for _, current := range p.IPList {
		if current == ip {
			return
		}
	}
	p.IPList = append(p.IPList, ip)
}

func (p *Profile) AddRoute(route *Route) {
	p.AddRoutes([]*Route{route})
}

func (p *Profile) AddRoutes(routes []*Route) {
	if p.Routes == nil {
		p.Routes = map[string]*Route{}
	}
	for _, route := range routes {
		ip := route.IP.String()
		if p.Routes[ip] == nil {
			p.appendIP(ip)
			p.Routes[ip] = &Route{IP: net.ParseIP(ip), HostNames: uniqueStrings(route.HostNames)}
			continue
		}
		p.Routes[ip].HostNames = uniqueStrings(append(p.Routes[ip].HostNames, route.HostNames...))
	}
}

func (p *Profile) Render(writer io.StringWriter) error {
	buffer := bytes.NewBuffer(nil)
	if _, err := fmt.Fprintf(buffer, "\n# profile.%s %s\n", p.Status, p.Name); err != nil {
		return err
	}
	for _, ip := range p.IPList {
		route := p.Routes[ip]
		for _, host := range route.HostNames {
			prefix := ""
			if p.Status == Disabled {
				prefix = disabledPrefix
			}
			if _, err := fmt.Fprintf(buffer, "%s%s %s\n", prefix, ip, host); err != nil {
				return err
			}
		}
	}
	if _, err := buffer.WriteString("# end\n"); err != nil {
		return err
	}
	_, err := writer.WriteString(buffer.String())
	return err
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

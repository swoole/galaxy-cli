package hostctl

import (
	"galaxy/pkg/hostctl/file"
	"galaxy/pkg/hostctl/types"
)

func CreateRoute(projectName, domain, resolve string) error {
	h, err := file.NewFile(GetDefaultHostFile())
	if err != nil {
		return err
	}
	pf, err := h.GetProfile(projectName)
	if err == nil {
		pf.AddRoute(types.NewRoute(resolve, domain))
	} else if err == types.ErrUnknownProfile {
		err = h.AddRoute(projectName, types.NewRoute(resolve, domain))
		if err != nil {
			return err
		}
	}
	err = h.Flush()
	if err != nil {
		return err
	}
	return nil
}

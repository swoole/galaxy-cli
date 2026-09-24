package file

import (
	"galaxy/pkg/hostctl/types"
)

// MergeProfiles joins new profiles with existing content.
func (f *File) MergeProfiles(profiles []*types.Profile) {
	for _, newP := range profiles {
		newName := newP.Name

		_, ok := f.data.Profiles[newName]
		if !ok {
			f.data.ProfileNames = append(f.data.ProfileNames, newName)
			f.data.Profiles[newName] = newP

			continue
		}

		baseP := f.data.Profiles[newName]

		var routes []*types.Route
		for _, r := range newP.Routes {
			routes = append(routes, r)
		}

		baseP.AddRoutes(routes)

		f.data.Profiles[newName] = baseP
	}
}

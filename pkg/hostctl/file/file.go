package file

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/spf13/afero"

	"galaxy/pkg/hostctl/parser"
	"galaxy/pkg/hostctl/types"
)

// File manages the Galaxy profiles stored in a hosts file.
type File struct {
	fs    afero.Fs
	path  string
	data  *types.Content
	mutex sync.Mutex
}

func NewFile(path string) (*File, error) {
	return NewWithFs(path, afero.NewOsFs())
}

func NewWithFs(path string, fs afero.Fs) (*File, error) {
	if fs == nil {
		fs = afero.NewOsFs()
	}
	source, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer source.Close()

	data, err := parser.Parse(source)
	if err != nil {
		return nil, err
	}
	return &File{fs: fs, path: path, data: data}, nil
}

func (f *File) GetProfile(name string) (*types.Profile, error) {
	profile, ok := f.data.Profiles[name]
	if !ok {
		return nil, types.ErrUnknownProfile
	}
	return profile, nil
}

func (f *File) AddRoute(name string, route *types.Route) error {
	return f.AddRoutes(name, []*types.Route{route})
}

func (f *File) AddRoutes(name string, routes []*types.Route) error {
	profile, err := f.GetProfile(name)
	if err != nil && !errors.Is(err, types.ErrUnknownProfile) {
		return err
	}
	if profile == nil {
		profile = &types.Profile{Name: name, Status: types.Enabled, Routes: map[string]*types.Route{}}
		profile.AddRoutes(routes)
		return f.AddProfile(profile)
	}
	profile.AddRoutes(routes)
	return nil
}

func (f *File) Flush() error {
	destination, err := f.fs.OpenFile(f.path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer destination.Close()
	return f.writeToFile(destination)
}

func (f *File) writeToFile(destination afero.File) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.data == nil {
		return types.ErrNoContent
	}
	if err := destination.Truncate(0); err != nil {
		return err
	}
	if _, err := destination.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := f.data.DefaultProfile.Render(destination); err != nil {
		return err
	}
	for _, name := range f.data.ProfileNames {
		if name == types.Default {
			continue
		}
		if err := f.data.Profiles[name].Render(destination); err != nil {
			return err
		}
	}
	return nil
}

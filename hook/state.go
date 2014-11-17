package hook

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"launchpad.net/errgo/errors"
)

// TODO should we add a CleanUp or Remove method on
// this type to remove all the state?

// PersistentState is used to save persistent charm state
// to disk. It is defined as an interface so that it can
// be defined differently for tests. The customary implementation
// is DiskState.
type PersistentState interface {
	// Save saves the given state data with the given name.
	Save(name string, data []byte) error

	// Load loads the state data from the given name.
	// If the data has not previously been saved, this
	// should return (nil, nil).
	Load(name string) ([]byte, error)
}

// diskState is an implementation of PersistentState that
// stores the state in the filesystem.
type diskState struct {
	dir string
}

// NewDiskState returns an implementation of
// PersistentState that stores state in the given directory.
func NewDiskState(dir string) PersistentState {
	return &diskState{dir}
}

// Save implements PersistentState.Save.
func (s *diskState) Save(name string, data []byte) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return errors.Wrap(err)
	}
	if err := ioutil.WriteFile(s.path(name), data, 0600); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

// Load implements PersistentState.Load
func (s *diskState) Load(name string) ([]byte, error) {
	data, err := ioutil.ReadFile(s.path(name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return data, nil
}

func (s *diskState) path(name string) string {
	return filepath.Join(s.dir, name+".json")
}

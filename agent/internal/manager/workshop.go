package manager

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"clutchbase/internal/protocol"
)

// WorkshopMaps reads only the managed instance's installed map directory.
func (m *Manager) WorkshopMaps(name string) ([]string, error) {
	dir, err := m.Dir(name)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Join(dir, "server"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer root.Close()
	const base = "game/csgo/maps/workshop"
	for _, part := range []string{"game", "game/csgo", "game/csgo/maps", base} {
		info, err := root.Lstat(part)
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return nil, errors.New("Workshop map directory must be a real directory")
		}
	}
	f, err := root.Open(base)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	dirs, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	maps := []string{}
	for _, d := range dirs {
		if !d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			continue
		}
		folder, err := root.Open(base + "/" + d.Name())
		if err != nil {
			return nil, err
		}
		files, err := folder.ReadDir(-1)
		folder.Close()
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".bsp") {
				continue
			}
			id := "workshop/" + d.Name() + "/" + strings.TrimSuffix(file.Name(), ".bsp")
			if protocol.ValidWorkshopMap(id) {
				maps = append(maps, id)
				if !protocol.ValidWorkshopMaps(maps) {
					return nil, errors.New("Workshop discovery exceeds report limits")
				}
			}
		}
	}
	sort.Strings(maps)
	return maps, nil
}

// CheckInstalledMap independently checks the managed installation before use.
func (m *Manager) CheckInstalledMap(name, mapName string) error {
	if protocol.ValidMap(mapName) {
		return nil
	}
	if !protocol.ValidWorkshopMap(mapName) {
		return errors.New("unsupported map")
	}
	maps, err := m.WorkshopMaps(name)
	if err != nil {
		return err
	}
	for _, installed := range maps {
		if installed == mapName {
			return nil
		}
	}
	return errors.New("Workshop map is not installed in this instance")
}

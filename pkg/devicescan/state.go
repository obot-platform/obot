package devicescan

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
)

// maxFileBytes is the per-file content cap. Files over this are recorded
// with Oversized=true and no Content.
const maxFileBytes int64 = 1 << 20 // 1 MiB

// artifactSkipDirs are dependency / build directories we never descend
// into when walking inside a skill or plugin directory.
var artifactSkipDirs = map[string]bool{
	"node_modules": true,
	".venv":        true,
	"venv":         true,
	"vendor":       true,
	"dist":         true,
	".tox":         true,
	".git":         true,
	"__pycache__":  true,
}

// scanState owns the dedup-by-key state shared across phases — the file
// table (keyed by absolute path) and the client table (keyed by name).
// Observation slices (mcps, skills, plugins) are now returned by phase
// functions and accumulated in Scan, not threaded through here.
type scanState struct {
	fsys    fs.FS
	homeAbs string

	files   map[string]types.DeviceScanFile
	clients map[string]types.DeviceScanClient
}

func newScanState(fsys fs.FS, homeAbs string) *scanState {
	return &scanState{
		fsys:    fsys,
		homeAbs: homeAbs,
		files:   map[string]types.DeviceScanFile{},
		clients: map[string]types.DeviceScanClient{},
	}
}

// abs converts an fs.FS-relative path to the absolute path used in wire
// output.
func (s *scanState) abs(rel string) string {
	return filepath.Join(s.homeAbs, filepath.FromSlash(rel))
}

// addFile reads and records the file at rel (relative to s.fsys). Returns
// the absolute path observations should reference. Idempotent. Files
// larger than maxFileBytes are recorded with Oversized=true and no Content.
func (s *scanState) addFile(rel string) (string, error) {
	abs := s.abs(rel)
	if _, ok := s.files[abs]; ok {
		return abs, nil
	}

	info, err := fs.Stat(s.fsys, rel)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("addFile: path is a directory")
	}

	f := types.DeviceScanFile{
		Path:      abs,
		SizeBytes: info.Size(),
	}

	if info.Size() > maxFileBytes {
		f.Oversized = true
		s.files[abs] = f
		return abs, nil
	}

	data, err := fs.ReadFile(s.fsys, rel)
	if err != nil {
		// Unreadable (perms / IO). Record path + size only; leave
		// Oversized=false so the UI doesn't mislabel it.
		s.files[abs] = f
		return abs, nil
	}

	if utf8.Valid(data) {
		f.Content = string(data)
	}
	s.files[abs] = f
	return abs, nil
}

// addFileOrAbs is a convenience wrapper that returns the absolute path
// regardless of whether the file could be read. Many phases want "the abs
// path to record on an observation, even if the file itself is missing."
func (s *scanState) addFileOrAbs(rel string) string {
	if abs, err := s.addFile(rel); err == nil && abs != "" {
		return abs
	}
	return s.abs(rel)
}

// addClient upserts a presence-detected client record. Version and the
// path fields take the first non-empty value when called more than once
// for the same name.
func (s *scanState) addClient(c types.DeviceScanClient) {
	if c.Name == "" {
		return
	}
	existing, ok := s.clients[c.Name]
	if !ok {
		s.clients[c.Name] = c
		return
	}
	if existing.Version == "" {
		existing.Version = c.Version
	}
	if existing.BinaryPath == "" {
		existing.BinaryPath = c.BinaryPath
	}
	if existing.InstallPath == "" {
		existing.InstallPath = c.InstallPath
	}
	if existing.ConfigPath == "" {
		existing.ConfigPath = c.ConfigPath
	}
	s.clients[c.Name] = existing
}

// listArtifactPaths walks dirRel and returns absolute paths for every
// file whose extension is in allowedExts. Files are NOT registered in
// s.files — only their paths are listed. Used by skills/plugins where
// we want a manifest of related files but don't upload their contents
// alongside the SKILL.md / manifest.
func (s *scanState) listArtifactPaths(dirRel string, allowedExts map[string]bool) []string {
	var paths []string
	_ = fs.WalkDir(s.fsys, dirRel, func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if rel != dirRel && artifactSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !allowedExts[path.Ext(rel)] {
			return nil
		}
		paths = append(paths, s.abs(rel))
		return nil
	})
	return paths
}

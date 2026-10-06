package mcp

import (
	"archive/tar"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestInitFilesArchive(t *testing.T) {
	files := map[string]string{
		"config": "first\nEOF\nrm -rf /\nlast",
		"large":  strings.Repeat("a", 512*1024),
	}

	archive, err := initFilesArchive(files)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	tr := tar.NewReader(archive)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		got[hdr.Name] = string(data)
	}

	if len(got) != len(files) {
		t.Fatalf("archive has %d files, want %d", len(got), len(files))
	}
	for name, content := range files {
		if got["init-files/"+name] != content+"\n" {
			t.Errorf("init-files/%s does not match the input content followed by a newline", name)
		}
	}
}

func TestInitFilesArchiveRejectsPathNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../escape", "dir/file"} {
		if _, err := initFilesArchive(map[string]string{name: "x"}); err == nil {
			t.Errorf("expected file name %q to be rejected", name)
		}
	}
}

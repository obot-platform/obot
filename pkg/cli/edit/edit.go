package edit

import (
	"bytes"
	"errors"
	"os"
	"strings"

	"k8s.io/kubectl/pkg/cmd/util/editor"
)

var (
	envs = []string{
		"ACORN_EDITOR",
		"EDITOR",
	}
)

func stripComments(buf []byte) []byte {
	var (
		result = bytes.Buffer{}
		start  = true
	)
	for _, line := range strings.Split(string(buf), "\n") {
		if start {
			if strings.HasPrefix(line, "# ") {
				continue
			}
			start = false
		}
		result.WriteString(line)
		result.WriteString("\n")
	}
	return result.Bytes()
}

func commentError(err error, buf []byte) []byte {
	var header bytes.Buffer
	for _, line := range strings.Split(strings.ReplaceAll(err.Error(), "\r", ""), "\n") {
		header.WriteString("# ")
		header.WriteString(line)
		header.WriteString("\n")
	}
	return append(header.Bytes(), buf...)
}

var ErrEditAborted = errors.New("edit aborted")

func Edit(content []byte, suffix string, save func([]byte) error) error {
	editor := editor.NewDefaultEditor(envs)
	for {
		buf, file, err := editor.LaunchTempFile("otto", suffix, bytes.NewReader(content))
		if file != "" {
			_ = os.Remove(file)
		}
		if err != nil {
			return err
		}

		if bytes.Equal(buf, content) {
			return ErrEditAborted
		}

		buf = stripComments(buf)

		if err := save(buf); err != nil {
			content = commentError(err, buf)
			continue
		}

		return nil
	}
}

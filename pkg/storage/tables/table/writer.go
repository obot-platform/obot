package table

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"text/template"

	"github.com/liggitt/tabwriter"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	yaml2 "sigs.k8s.io/yaml"
)

type Writer interface {
	Write(obj kclient.Object)
	// WriteFormatted will write a newly formatted object that follows the default
	// pattern passed to the NewWriter function.  If the format has been overwritten
	// by the use the formatted object will not be passed to the custom format, but
	// instead the kclient.Object if not nil.  This gives consistency in that users custom
	// formatting always applies to the source kclient.Object, not the intermediate formatted
	// object that was created.
	//
	// sourceObj may be nil
	WriteFormatted(formattedObj any, sourceObj kclient.Object)
	Close() error
	Err() error
	Flush() error
	AddFormatFunc(name string, f FormatFunc)
}

type writer struct {
	closed        bool
	HeaderFormat  string
	ValueFormat   string
	errs          []error
	headerPrinted bool
	rawWriter     io.Writer
	Writer        *tabwriter.Writer
	buffered      []buffered
	customFormat  bool
	dataFormat    bool
	funcMap       map[string]any
}

type buffered struct {
	formatted any
	obj       runtime.Object
}

type FormatFunc any

func NewWriter(values [][]string, quiet bool, format string) Writer {
	t := &writer{
		//nolint:govet
		funcMap: maps.Clone(FuncMap),
	}

	t.rawWriter = os.Stdout
	t.Writer = tabwriter.NewWriter(os.Stdout, 10, 1, 3, ' ', tabwriter.RememberWidths)

	t.HeaderFormat, t.ValueFormat = SimpleFormat(values)

	if quiet {
		t.HeaderFormat = ""
		t.ValueFormat = "{{.Obj | fullID }}\n"
		for _, row := range values {
			if len(row) > 1 && row[0] == "Name" {
				_, t.ValueFormat = SimpleFormat([][]string{row})
			}
		}
	}

	switch {
	case isDataFormat(format):
		t.HeaderFormat = ""
		t.ValueFormat = format
		t.dataFormat = true
	case format == "" || format == "table":
	default:
		t.HeaderFormat = ""
		t.ValueFormat = format + "\n"
		t.customFormat = true
	}

	return t
}

func (t *writer) AddFormatFunc(name string, f FormatFunc) {
	t.funcMap[name] = f
}

func (t *writer) Err() error {
	return t.Close()
}

func (t *writer) writeHeader() {
	if t.HeaderFormat != "" && !t.headerPrinted {
		t.headerPrinted = true
		err := t.printTemplate(t.Writer, t.HeaderFormat, struct{}{})
		_ = t.saveErr(err)
	}
}

func (t *writer) Write(obj kclient.Object) {
	t.WriteFormatted(nil, obj)
}

func (t *writer) WriteFormatted(formatted any, obj kclient.Object) {
	if len(t.errs) > 0 || (obj == nil && formatted == nil) {
		return
	}

	if obj != nil {
		obj = obj.DeepCopyObject().(kclient.Object)
		obj.SetManagedFields(nil)
	}

	t.writeHeader()
	if len(t.errs) > 0 {
		return
	}

	switch {
	case t.dataFormat:
		// write later
		t.buffered = append(t.buffered, buffered{
			formatted: formatted,
			obj:       obj,
		})
	case t.customFormat:
		// for a custom format prefer the kclient.Object over the formatted object
		t.writeObject(obj, formatted)
	default:
		// for default format prefer the formatted object over the kclient.Object
		t.writeObject(formatted, obj)
	}
}

// writeObject will write the first not nil entry with the table writer
func (t *writer) writeObject(objs ...any) {
	for _, obj := range objs {
		if obj == nil {
			continue
		}
		err := t.printTemplate(t.Writer, t.ValueFormat, obj)
		_ = t.saveErr(err)
		break
	}
}

func (t *writer) Flush() error {
	return t.flush(false)
}

func (t *writer) saveErr(err error) bool {
	if err != nil {
		t.errs = append(t.errs, err)
	}
	return err != nil
}

func (t *writer) writeDataObject(objs ...any) error {
	for i, obj := range objs {
		switch t.ValueFormat {
		case "json":
			if i > 0 {
				_, err := t.rawWriter.Write([]byte("\n"))
				if t.saveErr(err) {
					continue
				}
			}
			content, err := FormatJSON(obj)
			if t.saveErr(err) {
				continue
			}
			_, err = t.rawWriter.Write([]byte(content + "\n"))
			if t.saveErr(err) {
				continue
			}
		case "jsoncompact":
			if i > 0 {
				_, err := t.rawWriter.Write([]byte("\n"))
				if t.saveErr(err) {
					continue
				}
			}
			content, err := FormatJSONCompact(obj)
			if t.saveErr(err) {
				continue
			}
			_, err = t.rawWriter.Write([]byte(content))
			if t.saveErr(err) {
				continue
			}
		case "yaml":
			content, err := FormatJSON(obj)
			if t.saveErr(err) {
				continue
			}

			converted, err := yaml2.JSONToYAML([]byte(content))
			if t.saveErr(err) {
				continue
			}

			_, err = t.rawWriter.Write([]byte("---\n"))
			if t.saveErr(err) {
				continue
			}
			_, err = t.rawWriter.Write(append(converted, []byte("\n")...))
			if t.saveErr(err) {
				continue
			}
		default:
			t.saveErr(fmt.Errorf("invalid format for writing data %s", t.ValueFormat))
		}
	}

	return errors.Join(t.errs...)
}

type objectList struct {
	Items []any `json:"items"`
}

func (t *writer) flush(closing bool) error {
	if len(t.errs) > 0 {
		return errors.Join(t.errs...)
	}

	defer func() {
		t.buffered = nil
	}()

	// buffered object only exist for data formats
	if len(t.buffered) > 0 {
		var (
			objs []any
			seen = map[any]struct{}{}
		)
		for _, buffered := range t.buffered {
			objToWrite := any(buffered.obj)
			if objToWrite == nil {
				objToWrite = buffered.formatted
			}
			if objToWrite != nil {
				if _, ok := seen[reflect.ValueOf(objToWrite)]; ok {
					continue
				}
				seen[reflect.ValueOf(objToWrite)] = struct{}{}
				objs = append(objs, objToWrite)
			}
		}

		if closing {
			// if we are closing, then we want to print objects its in a proper list
			// data structure
			err := t.writeDataObject(objectList{Items: objs})
			if err != nil {
				return err
			}
		} else {
			// if we are not closing, then we want to print objects in a stream fashion, which is
			// just objects at the top level
			err := t.writeDataObject(objs...)
			if err != nil {
				return err
			}
		}
	}

	return t.Writer.Flush()
}

func (t *writer) Close() error {
	if t.closed {
		return errors.Join(t.errs...)
	}
	if len(t.errs) > 0 {
		return errors.Join(t.errs...)
	}

	defer func() {
		t.closed = true
	}()
	t.writeHeader()
	if len(t.errs) > 0 {
		return errors.Join(t.errs...)
	}
	return t.flush(true)
}

func (t *writer) printTemplate(out io.Writer, templateContent string, obj any) error {
	tmpl, err := template.New("").Funcs(t.funcMap).Parse(templateContent)
	if err != nil {
		return err
	}

	return tmpl.Execute(out, obj)
}

func isDataFormat(format string) bool {
	switch format {
	case "aml", "json", "jsoncompact", "yaml":
		return true
	default:
		return false
	}
}

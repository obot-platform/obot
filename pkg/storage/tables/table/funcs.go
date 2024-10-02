package table

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	FuncMap = map[string]any{
		"ago":          FormatCreated,
		"agoptr":       FormatTimePointer,
		"until":        FormatUntil,
		"json":         FormatJSON,
		"jsoncompact":  FormatJSONCompact,
		"yaml":         FormatYAML,
		"boolToStar":   BoolToStar,
		"array":        ToArray,
		"arrayFirst":   ToArrayFirst,
		"arrayNoSpace": ToArrayNoSpace,
		"graph":        Graph,
		"pointer":      Pointer,
		"fullID":       FormatID,
		"alias":        Noop,
		"ownerName":    OwnerReferenceName,
	}
)

func Noop(any) string {
	return ""
}

func ToArray(s []string) (string, error) {
	return strings.Join(s, ", "), nil
}

func ToArrayNoSpace(s []string) (string, error) {
	return strings.Join(s, ","), nil
}

func ToArrayFirst(s []string) (string, error) {
	if len(s) > 0 {
		return s[0], nil
	}
	return "", nil
}

func Graph(value int) (string, error) {
	bars := int(float64(value) / 100.0 * 30)
	builder := &strings.Builder{}
	for i := 0; i < bars; i++ {
		if i == bars-1 {
			builder.WriteString(fmt.Sprintf("> %v", value))
			break
		}
		builder.WriteString("=")
	}
	return builder.String(), nil
}

func Pointer(data any) string {
	if reflect.ValueOf(data).IsNil() {
		return ""
	}
	return fmt.Sprint(data)
}

func FormatID(obj kclient.Object) (string, error) {
	return obj.GetName(), nil
}

func FormatTimePointer(data *metav1.Time) string {
	if data == nil {
		return ""
	}
	return FormatCreated(*data)
}

func FormatCreated(data metav1.Time) string {
	return duration.HumanDuration(time.Now().UTC().Sub(data.Time)) + " ago"
}

func FormatUntil(data metav1.Time) string {
	return duration.HumanDuration(time.Until(data.Time.UTC())) + " from now"
}

func FormatJSON(data any) (string, error) {
	bytes, err := json.MarshalIndent(cleanFields(data), "", "    ")
	return string(bytes) + "\n", err
}

func FormatJSONCompact(data any) (string, error) {
	bytes, err := json.Marshal(cleanFields(data))
	return string(bytes) + "\n", err
}

func toKObject(obj any) (kclient.Object, bool) {
	ro, ok := obj.(kclient.Object)
	if !ok {
		newObj := reflect.New(reflect.TypeOf(obj))
		newObj.Elem().Set(reflect.ValueOf(obj))
		ro, ok = newObj.Interface().(kclient.Object)
	}
	return ro, ok
}

func cleanFields(obj any) any {
	if ol, ok := obj.(objectList); ok {
		for i, o := range ol.Items {
			ol.Items[i] = cleanFields(o)
		}
		return ol
	}

	ro, ok := toKObject(obj)
	if ok {
		ro.SetManagedFields(nil)
		ro.SetUID("")
		ro.SetGenerateName("")
		ro.SetResourceVersion("")
		labels := ro.GetLabels()
		for k := range labels {
			if strings.Contains(k, "gptscript.io/") {
				delete(labels, k)
			}
		}
		ro.SetLabels(labels)

		annotations := ro.GetAnnotations()
		for k := range annotations {
			if strings.Contains(k, "gptscript.io/") {
				delete(annotations, k)
			}
		}
		ro.SetAnnotations(annotations)
		return ro
	}
	return obj
}

func FormatYAML(data any) (string, error) {
	bytes, err := yaml.Marshal(cleanFields(data))
	return string(bytes) + "\n", err
}

func BoolToStar(obj any) (string, error) {
	if b, ok := obj.(bool); ok && b {
		return "*", nil
	}
	if b, ok := obj.(*bool); ok && b != nil && *b {
		return "*", nil
	}
	return "", nil
}

func OwnerReferenceName(obj metav1.Object) string {
	owners := obj.GetOwnerReferences()
	if len(owners) == 0 {
		return ""
	}

	return owners[0].Name
}

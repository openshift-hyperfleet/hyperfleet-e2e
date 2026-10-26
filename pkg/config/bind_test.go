package config

import (
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// TestEnvVarReachesEveryField sets the env var of every leaf Config field and
// checks that viper.Unmarshal puts its value in the struct, with no config file
// to name the key. This relies on viper's ExperimentalBindStruct option, so it
// also guards a viper upgrade. Each round starts from viper.Reset, which clears
// the option, as the tests that load config do.
func TestEnvVarReachesEveryField(t *testing.T) {
	t.Cleanup(viper.Reset)
	want := map[string]any{}
	fields := leafFields(reflect.ValueOf(&Config{}).Elem(), "")
	for i, path := range slices.Sorted(maps.Keys(fields)) {
		field, n := fields[path], i+1
		var value string
		switch {
		case field.Type() == reflect.TypeFor[time.Duration]():
			value, want[path] = (time.Duration(n) * time.Second).String(), time.Duration(n)*time.Second
		case field.Kind() == reflect.String:
			value, want[path] = "value-"+strconv.Itoa(n), "value-"+strconv.Itoa(n)
		case field.Kind() == reflect.Bool:
			value, want[path] = "true", true
		case field.Kind() == reflect.Int64:
			value, want[path] = strconv.Itoa(n), int64(n)
		default:
			t.Fatalf("add a case for %s of type %s", path, field.Type())
		}
		t.Setenv(EnvVar(strings.ToUpper(strings.ReplaceAll(path, ".", "_"))), value)
	}

	for round := range 20 {
		viper.Reset()
		if err := BindEnvVars(); err != nil {
			t.Fatalf("BindEnvVars: %v", err)
		}
		var cfg Config
		if err := viper.Unmarshal(&cfg); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		got := leafFields(reflect.ValueOf(&cfg).Elem(), "")
		for path, value := range want {
			if field := got[path]; !reflect.DeepEqual(field.Interface(), value) {
				t.Fatalf("round %d: %s = %v, want %v", round, path, field.Interface(), value)
			}
		}
	}
}

// leafFields returns the leaf fields under v by config path. A field tagged "-",
// such as Entities, has no path.
func leafFields(v reflect.Value, prefix string) map[string]reflect.Value {
	fields := map[string]reflect.Value{}
	for fieldType, field := range v.Fields() {
		name, _, _ := strings.Cut(fieldType.Tag.Get(TagMapstructure), ",")
		if name == "" || name == "-" {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if field.Kind() == reflect.Struct {
			maps.Copy(fields, leafFields(field, path))
		} else {
			fields[path] = field
		}
	}
	return fields
}

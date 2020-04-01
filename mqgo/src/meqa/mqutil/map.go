package mqutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-openapi/swag"
	"gopkg.in/yaml.v2"
)

func InterfaceToJsonString(i interface{}) string {
	b, _ := json.Marshal(i)
	if b[0] == '"' {
		return string(b[1 : len(b)-1]) // remove the ""
	}
	return string(b)
}

// MapInterfaceToMapString converts the params map (all primitive types with exception of array)
// before passing to resty.
func MapInterfaceToMapString(src map[string]interface{}) map[string]string {
	dst := make(map[string]string)
	for k, v := range src {
		if ar, ok := v.([]interface{}); ok {
			str := ""
			for _, entry := range ar {
				str += fmt.Sprintf("%v,", InterfaceToJsonString(entry))
			}
			str = strings.TrimRight(str, ",")
			dst[k] = str
		} else {
			dst[k] = InterfaceToJsonString(v)
		}
	}
	return dst
}

// MapIsCompatible checks if the first map has every key in the second.
func MapIsCompatible(big map[string]interface{}, small map[string]interface{}) bool {
	for k, _ := range small {
		if _, ok := big[k]; !ok {
			return false
		}
	}
	return true
}

func TimeCompare(v1 interface{}, v2 interface{}) bool {
	s1, ok := v1.(string)
	if !ok {
		return false
	}
	s2, ok := v2.(string)
	if !ok {
		return false
	}
	var t time.Time
	var s string
	var b1, b2 bool
	t1, err := time.Parse(time.RFC3339, s1)
	if err == nil {
		t = t1
		s = s2
		b1 = true
	}
	t2, err := time.Parse(time.RFC3339, s2)
	if err == nil {
		t = t2
		s = s1
		b2 = true
	}
	if b1 && b2 {
		return t1 == t2
	}
	if !b1 && !b2 {
		return false
	}
	// One of b1 and b2 is true, now t point to time and s point to a potential time string
	// that's not RFC3339 format. We make a guess buy searching for a few key elements.
	return strings.Contains(s, fmt.Sprintf("%d", t.Second())) && strings.Contains(s, fmt.Sprintf("%d", t.Minute()))
}

// MapCombine combines two map together. If there is any overlap the dst will be overwritten.
func MapCombine(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(dst) == 0 {
		return MapCopy(src)
	}
	if len(src) == 0 {
		return dst
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// Just like MapCombine but keep the original dst value if there is an overlap.
func MapAdd(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(dst) == 0 {
		return MapCopy(src)
	}
	if len(src) == 0 {
		return dst
	}
	for k, v := range src {
		if _, exist := dst[k]; !exist {
			dst[k] = v
		}
	}
	return dst
}

// MapReplace replaces the values in dst with the ones in src with the matching keys.
func MapReplace(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return dst
	}
	for k := range dst {
		if v, ok := src[k]; ok {
			dst[k] = v
		}
	}
	return dst
}

func MapCopy(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{})
	for k, v := range src {
		if m, ok := v.(map[string]interface{}); ok {
			v = MapCopy(m)
		}
		if a, ok := v.([]interface{}); ok {
			v = ArrayCopy(a)
		}
		dst[k] = v
	}
	return dst
}

func ArrayCopy(src []interface{}) (dst []interface{}) {
	if len(src) == 0 {
		return nil
	}
	for _, v := range src {
		if m, ok := v.(map[string]interface{}); ok {
			v = MapCopy(m)
		}
		if a, ok := v.([]interface{}); ok {
			v = ArrayCopy(a)
		}
		dst = append(dst, v)
	}
	return dst
}

func InterfacePrint(m interface{}, printToConsole bool) {
	yamlBytes, _ := yaml.Marshal(m)
	Logger.Print(string(yamlBytes))
	if printToConsole {
		fmt.Println(string(yamlBytes))
	}
}

// Check if existing matches criteria. When criteria is a map, we check whether
// everything in criteria can be found and equals a field in existing.
func InterfaceEquals(criteria interface{}, existing interface{}) bool {
	if criteria == nil {
		if existing == nil {
			return true
		} else {
			existingKind := reflect.TypeOf(existing).Kind()
			if existingKind == reflect.Map || existingKind == reflect.Array || existingKind == reflect.Slice {
				return true
			}
			return false
		}
	} else {
		if existing == nil {
			return false
		}
	}
	cType := reflect.TypeOf(criteria)
	eType := reflect.TypeOf(existing)
	if cType == eType && cType.Comparable() {
		if criteria == existing {
			return true
		}
		// The only exception is time, where the format may be different on both ends.
		return TimeCompare(criteria, existing)
	}

	cKind := cType.Kind()
	eKind := eType.Kind()
	if cKind == reflect.Array || cKind == reflect.Slice {
		if eKind == reflect.Array || eKind == reflect.Slice {
			// We don't compare arrays
			return true
		}
		return false
	}
	if cKind == reflect.Map {
		if eKind != reflect.Map {
			return false
		}
		cm, ok := criteria.(map[string]interface{})
		if !ok {
			return false
		}
		em, ok := existing.(map[string]interface{})
		if !ok {
			return false
		}
		for k, v := range cm {
			if !InterfaceEquals(v, em[k]) {
				return false
			}
		}
		return true
	}
	if eKind == reflect.String && (cKind == reflect.Int || cKind == reflect.Float32 || cKind == reflect.Float64) {
		return reflect.TypeOf(existing).String() == "json.Number"
	}

	cJson, _ := json.Marshal(criteria)
	eJson, _ := json.Marshal(existing)

	return string(cJson) == string(eJson)
}

func MarshalJsonIndentNoEscape(i interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "    ")
	err := enc.Encode(i)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Given a yaml stream, output a json stream.
func YamlToJson(in []byte) (json.RawMessage, error) {
	var unmarshaled interface{}
	err := yaml.Unmarshal(in, &unmarshaled)
	if err != nil {
		return nil, err
	}
	return swag.YAMLToJSON(unmarshaled)
}

func JsonToYaml(in []byte) ([]byte, error) {
	var out interface{}
	err := json.Unmarshal(in, &out)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(out)
}

func YamlObjToJsonObj(in interface{}) (interface{}, error) {
	jsonRaw, err := swag.YAMLToJSON(in)
	if err != nil {
		return nil, err
	}
	var out interface{}
	err = json.Unmarshal(jsonRaw, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type FieldIterFunc func(key string, value interface{}) error
type MapIterFunc func(m map[string]interface{}) error

// Iterate all the leaf level fields. For maps iterate all the fields. For arrays we will go through all the entries and
// see if any of them is a map. The iteration will be done in a width first order, so deeply buried fields will be iterated last.
// The maps should be map[string]interface{}.
func IterateMapsInInterface(in interface{}, callback MapIterFunc) error {
	if inMap, _ := in.(map[string]interface{}); inMap != nil {
		err := callback(inMap)
		if err != nil {
			return err
		}
		for _, v := range inMap {
			err := IterateMapsInInterface(v, callback)
			if err != nil {
				return err
			}
		}
		return nil
	}
	if inArray, _ := in.([]interface{}); inArray != nil {
		for _, v := range inArray {
			err := IterateMapsInInterface(v, callback)
			if err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

func IterateFieldsInInterface(in interface{}, callback FieldIterFunc) error {
	mapCallback := func(m map[string]interface{}) error {
		for k, v := range m {
			err := callback(k, v)
			if err != nil {
				return err
			}
		}
		return nil
	}

	return IterateMapsInInterface(in, mapCallback)
}

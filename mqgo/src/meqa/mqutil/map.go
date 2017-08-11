package mqutil

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MapInterfaceToMapString converts the params map (all primitive types with exception of array)
// before passing to resty.
func MapInterfaceToMapString(src map[string]interface{}) map[string]string {
	dst := make(map[string]string)
	for k, v := range src {
		if ar, ok := v.([]interface{}); ok {
			str := ""
			for _, entry := range ar {
				str += fmt.Sprintf("%v,", entry)
			}
			str = strings.TrimRight(str, ",")
			dst[k] = str
		} else {
			dst[k] = fmt.Sprint(v)
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
		return src
	}
	if len(src) == 0 {
		return dst
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func MapEquals(big map[string]interface{}, small map[string]interface{}, strict bool) bool {
	if strict && len(big) != len(small) {
		return false
	}
	for k, v := range small {
		if big[k] != v && fmt.Sprint(big[k]) != fmt.Sprint(v) && !TimeCompare(big[k], v) {
			fmt.Printf("key %v: %v %v mismatch", k, big[k], v)
			return false
		}
	}
	return true
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

func InterfacePrint(m interface{}, prefix string) {
	jsonBytes, _ := json.Marshal(m)
	Logger.Printf("%s%s", prefix, string(jsonBytes))
}

// InterfaceToArray converts interface type to []map[string]interface{}.
func InterfaceToArray(obj interface{}) []map[string]interface{} {
	var objarray []map[string]interface{}
	if a, ok := obj.([]interface{}); ok {
		if len(a) > 0 {
			if _, ok := a[0].(map[string]interface{}); ok {
				objarray = obj.([]map[string]interface{})
			}
		}
	} else if o, ok := obj.(map[string]interface{}); ok {
		objarray = []map[string]interface{}{o}
	}
	return objarray
}

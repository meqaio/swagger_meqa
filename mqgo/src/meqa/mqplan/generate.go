package mqplan

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"time"

	"gopkg.in/resty.v0"

	"meqa/mqswag"
	"meqa/mqutil"

	"github.com/go-openapi/spec"
	"github.com/lucasjones/reggen"
	"github.com/xeipuuv/gojsonschema"
)

func getOperationByMethod(item *spec.PathItem, method string) *spec.Operation {
	switch method {
	case resty.MethodGet:
		return item.Get
	case resty.MethodPost:
		return item.Post
	case resty.MethodPut:
		return item.Put
	case resty.MethodDelete:
		return item.Delete
	case resty.MethodPatch:
		return item.Patch
	case resty.MethodHead:
		return item.Head
	case resty.MethodOptions:
		return item.Options
	}
	return nil
}

// Generate paramter value based on the spec.
func GenerateParameter(paramSpec *spec.Parameter, swagger *mqswag.Swagger, db mqswag.DB) (interface{}, error) {
	if paramSpec.Schema != nil {
		return GenerateSchema(paramSpec.Name, paramSpec.Schema, swagger, db)
	}
	if len(paramSpec.Enum) != 0 {
		return generateEnum(paramSpec.Enum)
	}
	if len(paramSpec.Type) == 0 {
		return nil, mqutil.NewError(mqutil.ErrInvalid, "Parameter doesn't have type")
	}

	var schema *spec.Schema
	if paramSpec.Schema != nil {
		schema = paramSpec.Schema
	} else {
		// construct a full schema from simple ones
		schema = createSchemaFromSimple(&paramSpec.SimpleSchema, &paramSpec.CommonValidations)
	}
	if paramSpec.Type == gojsonschema.TYPE_OBJECT {
		return generateObject("param_", schema, swagger, db)
	}
	if paramSpec.Type == gojsonschema.TYPE_ARRAY {
		return generateArray("param_", schema, swagger, db)
	}

	return generateByType(createSchemaFromSimple(&paramSpec.SimpleSchema, &paramSpec.CommonValidations), paramSpec.Name+"_")
}

func generateByType(s *spec.Schema, prefix string) (interface{}, error) {
	if len(s.Type) != 0 {
		switch s.Type[0] {
		case gojsonschema.TYPE_BOOLEAN:
			return generateBool(s)
		case gojsonschema.TYPE_INTEGER:
			return generateInt(s)
		case gojsonschema.TYPE_NUMBER:
			return generateFloat(s)
		case gojsonschema.TYPE_STRING:
			return generateString(s, prefix)
		}
	}

	return nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("unrecognized type: %s", s.Type))
}

// RandomTime generate a random time in the range of [t - r, t).
func RandomTime(t time.Time, r time.Duration) time.Time {
	return t.Add(-time.Duration(float64(r) * rand.Float64()))
}

// TODO we need to make it context aware. Based on different contexts we should generate different
// date ranges. Prefix is a prefix to use when generating strings. It's only used when there is
// no specified pattern in the swagger.json
func generateString(s *spec.Schema, prefix string) (string, error) {
	if s.Format == "date-time" {
		t := RandomTime(time.Now(), time.Hour*24*30)
		return t.Format(time.RFC3339), nil
	}
	if s.Format == "date" {
		t := RandomTime(time.Now(), time.Hour*24*30)
		return t.Format("2006-01-02"), nil
	}

	// If no pattern is specified, we use the field name + some numbers as pattern
	var pattern string
	length := 0
	if len(s.Pattern) != 0 {
		pattern = s.Pattern
		length = len(s.Pattern) * 2
	} else {
		pattern = prefix + "\\d+"
		length = len(prefix) + 5
	}
	str, err := reggen.Generate(pattern, length)
	if err != nil {
		return "", mqutil.NewError(mqutil.ErrInvalid, err.Error())
	}

	if len(s.Format) == 0 || s.Format == "password" {
		return str, nil
	}
	if s.Format == "byte" {
		return base64.StdEncoding.EncodeToString([]byte(str)), nil
	}
	if s.Format == "binary" {
		return hex.EncodeToString([]byte(str)), nil
	}
	return "", mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Invalid format string: %s", s.Format))
}

func generateBool(s *spec.Schema) (interface{}, error) {
	return rand.Intn(2) == 0, nil
}

func generateFloat(s *spec.Schema) (float64, error) {
	var realmin float64
	if s.Minimum != nil {
		realmin = *s.Minimum
		if s.ExclusiveMinimum {
			realmin += 0.01
		}
	}
	var realmax float64
	if s.Maximum != nil {
		realmax = *s.Maximum
		if s.ExclusiveMaximum {
			realmax -= 0.01
		}
	}
	if realmin >= realmax {
		if s.Minimum == nil && s.Maximum == nil {
			realmin = -1.0
			realmax = 1.0
		} else if s.Minimum != nil {
			realmax = realmin + math.Abs(realmin)
		} else if s.Maximum != nil {
			realmin = realmax - math.Abs(realmax)
		} else {
			// both are present but conflicting
			return 0, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("specified min value %v is bigger than max %v",
				*s.Minimum, *s.Maximum))
		}
	}
	return rand.Float64()*(realmax-realmin) + realmin, nil
}

func generateInt(s *spec.Schema) (int64, error) {
	// Give a default range if there isn't one
	if s.Maximum == nil && s.Minimum == nil {
		maxf := 10000.0
		s.Maximum = &maxf
	}
	f, err := generateFloat(s)
	if err != nil {
		return 0, err
	}
	i := int64(f)
	if s.Minimum != nil && i <= int64(*s.Minimum) {
		i++
	}
	return i, nil
}

func generateArray(name string, schema *spec.Schema, swagger *mqswag.Swagger, db mqswag.DB) (interface{}, error) {
	var numItems int
	if schema.MaxItems != nil || schema.MinItems != nil {
		var maxItems int
		if schema.MaxItems != nil {
			maxItems = int(*schema.MaxItems)
			if maxItems < 0 {
				maxItems = 0
			}
		}
		var minItems int
		if schema.MinItems != nil {
			minItems = int(*schema.MinItems)
			if minItems < 0 {
				minItems = 0
			}
		}
		maxDiff := maxItems - minItems
		if maxDiff <= 0 {
			maxDiff = 1
		}
		numItems = rand.Intn(int(maxDiff)) + minItems
	} else {
		numItems = rand.Intn(10)
	}

	var itemSchema *spec.Schema
	if len(schema.Items.Schemas) != 0 {
		itemSchema = &(schema.Items.Schemas[0])
	} else {
		itemSchema = schema.Items.Schema
	}

	var ar []interface{}
	for i := 0; i < numItems; i++ {
		entry, err := GenerateSchema(name, itemSchema, swagger, db)
		if err != nil {
			return nil, err
		}
		ar = append(ar, entry)
	}
	return ar, nil
}

func generateObject(name string, schema *spec.Schema, swagger *mqswag.Swagger, db mqswag.DB) (interface{}, error) {
	obj := make(map[string]interface{})
	for k, v := range schema.Properties {
		o, err := GenerateSchema(name+k+"_", &v, swagger, db)
		if err != nil {
			return nil, err
		}
		obj[k] = o
	}
	return obj, nil
}

func createSchemaFromSimple(s *spec.SimpleSchema, v *spec.CommonValidations) *spec.Schema {
	schema := spec.Schema{}
	schema.AddType(s.Type, s.Format)
	if s.Items != nil {
		schema.Items = &spec.SchemaOrArray{}
		schema.Items.Schema = createSchemaFromSimple(&s.Items.SimpleSchema, &s.Items.CommonValidations)
	}
	schema.Default = s.Default
	schema.Enum = v.Enum
	schema.ExclusiveMaximum = v.ExclusiveMaximum
	schema.ExclusiveMinimum = v.ExclusiveMinimum
	schema.Maximum = v.Maximum
	schema.Minimum = v.Minimum
	schema.MaxItems = v.MaxItems
	schema.MaxLength = v.MaxLength
	schema.MinItems = v.MinItems
	schema.MinLength = v.MinLength
	schema.MultipleOf = v.MultipleOf
	schema.Pattern = v.Pattern
	schema.UniqueItems = v.UniqueItems

	return &schema
}

func GenerateSchema(name string, schema *spec.Schema, swagger *mqswag.Swagger, db mqswag.DB) (interface{}, error) {
	// Deal with refs.
	tokens := schema.Ref.GetPointer().DecodedTokens()
	if len(tokens) != 0 {
		if len(tokens) != 2 {
			return nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Invalid reference: %s", schema.Ref.GetURL()))
		}
		if tokens[0] == "definitions" {
			referredSchema, ok := swagger.Definitions[tokens[1]]
			if !ok {
				return nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Reference object not found: %s", schema.Ref.GetURL()))
			}
			return GenerateSchema(name, &referredSchema, swagger, db)
		}
		return nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Invalid reference: %s", schema.Ref.GetURL()))
	}

	if len(schema.Enum) != 0 {
		return generateEnum(schema.Enum)
	}
	if len(schema.Type) == 0 {
		return nil, mqutil.NewError(mqutil.ErrInvalid, "Parameter doesn't have type")
	}
	if schema.Type[0] == gojsonschema.TYPE_OBJECT {
		return generateObject(name, schema, swagger, db)
	}
	if schema.Type[0] == gojsonschema.TYPE_ARRAY {
		return generateArray(name, schema, swagger, db)
	}

	return generateByType(schema, name)
}

func generateEnum(e []interface{}) (interface{}, error) {
	return e[rand.Intn(len(e))], nil
}

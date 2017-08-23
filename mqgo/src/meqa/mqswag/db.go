package mqswag

import (
	"errors"
	"fmt"
	"meqa/mqutil"
	"reflect"
	"sync"

	"github.com/go-openapi/spec"
	"github.com/xeipuuv/gojsonschema"
)

// This file implements the in-memory DB that holds all the entity objects.

// Schema is the swagger spec schema.
type Schema spec.Schema

func CreateSchemaFromSimple(s *spec.SimpleSchema, v *spec.CommonValidations) *Schema {
	schema := spec.Schema{}
	schema.AddType(s.Type, s.Format)
	if s.Items != nil {
		schema.Items = &spec.SchemaOrArray{}
		schema.Items.Schema = (*spec.Schema)(CreateSchemaFromSimple(&s.Items.SimpleSchema, &s.Items.CommonValidations))
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

	return (*Schema)(&schema)
}

// Matches checks if the Schema matches the input interface. In proper swagger.json
// Enums should have types as well. So we don't check for untyped enums.
// TODO check format, handle AllOf, AnyOf, OneOf
func (schema *Schema) Matches(object interface{}, swagger *Swagger) bool {
	if object == nil {
		return schema.Type.Contains(gojsonschema.TYPE_NULL)
	}

	_, referredSchema, err := swagger.GetReferredSchema(schema)
	if err != nil {
		mqutil.Logger.Print(err.Error())
		return false
	}
	if referredSchema != nil {
		return referredSchema.Matches(object, swagger)
	}

	// This check works right now because we are not too strict in MatchesMap function.
	if len(schema.AllOf) > 0 {
		for _, s := range schema.AllOf {
			if !((*Schema)(&s)).Matches(object, swagger) {
				return false
			}
		}
		return true
	}

	if len(schema.AnyOf) > 0 {
		for _, s := range schema.AnyOf {
			if !((*Schema)(&s)).Matches(object, swagger) {
				return true
			}
		}
		return false
	}

	k := reflect.TypeOf(object).Kind()
	if k == reflect.Bool {
		return schema.Type.Contains(gojsonschema.TYPE_BOOLEAN)
	} else if k >= reflect.Int && k <= reflect.Uint64 {
		return schema.Type.Contains(gojsonschema.TYPE_INTEGER) || schema.Type.Contains(gojsonschema.TYPE_NUMBER)
	} else if k == reflect.Float32 || k == reflect.Float64 {
		// After unmarshal, the map only holds floats. It doesn't differentiate int and float.
		return schema.Type.Contains(gojsonschema.TYPE_INTEGER) || schema.Type.Contains(gojsonschema.TYPE_NUMBER)
	} else if k == reflect.String {
		return schema.Type.Contains(gojsonschema.TYPE_STRING)
	} else if k == reflect.Array || k == reflect.Slice {
		if !schema.Type.Contains(gojsonschema.TYPE_ARRAY) {
			return false
		}
		// Check the array elements.
		itemsSchema := (*Schema)(schema.Items.Schema)
		if itemsSchema == nil && len(schema.Items.Schemas) > 0 {
			s := Schema(schema.Items.Schemas[0])
			itemsSchema = &s
		}
		if itemsSchema == nil {
			return false
		}
		ar := object.([]interface{})
		for _, item := range ar {
			if !itemsSchema.Matches(item, swagger) {
				return false
			}
		}
		return true
	} else if k == reflect.Map {
		if !schema.Type.Contains(gojsonschema.TYPE_OBJECT) {
			return false
		}
		// check the object content.
		return schema.MatchesMap(object.(map[string]interface{}), swagger)
	} else {
		mqutil.Logger.Printf("unknown type: %v", k)
	}
	return false
}

// MatchesMap checks if the Schema matches the input map.
func (schema *Schema) MatchesMap(obj map[string]interface{}, swagger *Swagger) bool {
	if obj == nil {
		return false
	}
	// check all required fields in Schema are present in the object.
	for _, requiredName := range schema.Required {
		if obj[requiredName] == nil {
			mqutil.Logger.Printf("required field not present: %s", requiredName)
			return false
		}
	}

	/* This check is too strict. Sometimes people have a schema with type "object" but no field to mean a generic object.
	// check all object's fields are in schema and the types match.
	for k, v := range obj {
		if p, ok := schema.Properties[k]; ok {
			if !((*Schema)(&p)).Matches(v, swagger) {
				mqutil.Logger.Printf("property type mismatch: %s %v", k, p)
				return false
			}
		}
	}
	*/
	return true
}

func (schema *Schema) Contains(name string, swagger *Swagger) bool {
	iterFunc := func(swagger *Swagger, schemaName string, schema *Schema, context interface{}) error {
		// The only way we have to abort is through an error.
		if schemaName == name {
			return errors.New("found")
		}
		return nil
	}

	err := schema.Iterate(iterFunc, nil, swagger)
	if err != nil && err.Error() == "found" {
		return true
	}
	return false
}

type SchemaIterator func(swagger *Swagger, schemaName string, schema *Schema, context interface{}) error

// IterateSchema descends down the starting schema and call the iterator function for all the child schemas.
// The iteration order is parent first then children. It will abort on error.
func (schema *Schema) Iterate(iterFunc SchemaIterator, context interface{}, swagger *Swagger) error {
	err := iterFunc(swagger, "", schema, context)
	if err != nil {
		return err
	}

	// Deal with refs.
	referenceName, referredSchema, err := swagger.GetReferredSchema(schema)
	if err != nil {
		return err
	}
	if referredSchema != nil {
		// We don't want to go down nested schemas.
		// XXX
		return iterFunc(swagger, referenceName, referredSchema, context)
		// return referredSchema.Iterate(iterFunc, context, swagger)
	}

	if schema.Type.Contains(gojsonschema.TYPE_OBJECT) {
		for _, v := range schema.Properties {
			err = (*Schema)(&v).Iterate(iterFunc, context, swagger)
			if err != nil {
				return err
			}
		}
	}
	if schema.Type.Contains(gojsonschema.TYPE_ARRAY) {
		var itemSchema *spec.Schema
		if len(schema.Items.Schemas) != 0 {
			itemSchema = &(schema.Items.Schemas[0])
		} else {
			itemSchema = schema.Items.Schema
		}
		err = (*Schema)(itemSchema).Iterate(iterFunc, context, swagger)
		if err != nil {
			return err
		}
	}
	return nil
}

// SchemaDB is our in-memory DB. It is organized around Schemas. Each schema maintains a list of objects that matches
// the schema. We don't build indexes and do linear search. This keeps the searching flexible for now.
type SchemaDB struct {
	Name    string
	Schema  *Schema
	Objects []interface{}
}

// Insert inserts an object into the schema's object list.
func (db *SchemaDB) Insert(obj interface{}) error {
	db.Objects = append(db.Objects, obj)
	return nil
}

// MatchFunc checks whether the input criteria and an input object matches.
type MatchFunc func(criteria interface{}, existing interface{}) bool

// An implementation of the MatchFunc that returns true if the existing object matches all the fields in the criteria obj.
func MatchAllFields(criteria interface{}, existing interface{}) bool {
	cm, ok := criteria.(map[string]interface{})
	if !ok {
		return false
	}
	em, ok := existing.(map[string]interface{})
	if !ok {
		return false
	}
	// We only do simple value comparision for now. We know that our search keys are simple types.
	for k, v := range cm {
		if em[k] != v {
			return false
		}
	}
	return true
}

func MatchAlways(criteria interface{}, existing interface{}) bool {
	return true
}

// Find finds the specified number of objects that match the input criteria.
func (db *SchemaDB) Find(criteria interface{}, matches MatchFunc, desiredCount int) []interface{} {
	var result []interface{}
	for _, obj := range db.Objects {
		if matches(criteria, obj) {
			result = append(result, obj)
			if len(result) >= desiredCount {
				return result
			}
		}
	}
	return result
}

// Delete deletes the specified number of elements that match the criteria. Input -1 for delete all.
// Returns the number of elements deleted.
func (db *SchemaDB) Delete(criteria interface{}, matches MatchFunc, desiredCount int) int {
	count := 0
	for i, obj := range db.Objects {
		if matches(criteria, obj) {
			db.Objects[i] = db.Objects[count]
			count++
			if count >= desiredCount {
				break
			}
		}
	}
	db.Objects = db.Objects[count:]
	return count
}

// Update finds the matching object, then update with the new one.
func (db *SchemaDB) Update(criteria interface{}, matches MatchFunc, newObj map[string]interface{}, desiredCount int, patch bool) int {
	count := 0
	for i, obj := range db.Objects {
		if matches(criteria, obj) {
			m, ok := obj.(map[string]interface{})
			if !ok {
				continue
			}
			if patch {
				mqutil.MapCombine(m, newObj)
			} else {
				db.Objects[i] = newObj
			}
			count++
			if count >= desiredCount {
				break
			}
		}
	}
	return count
}

type DB struct {
	schemas map[string](*SchemaDB)
	Swagger *Swagger
	mutex   sync.Mutex // We don't expect much contention, as such mutex will be fast
}

func (db *DB) Init(s *Swagger) {
	db.Swagger = s
	db.schemas = make(map[string](*SchemaDB))
	for schemaName, schema := range s.Definitions {
		if _, ok := db.schemas[schemaName]; ok {
			mqutil.Logger.Printf("warning - schema %s already exists", schemaName)
		}
		// Note that schema variable is reused in the loop
		schemaCopy := schema
		db.schemas[schemaName] = &SchemaDB{schemaName, (*Schema)(&schemaCopy), nil}
	}
}

func (db *DB) GetSchema(name string) *Schema {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	if db.schemas[name] == nil {
		return nil
	}
	return db.schemas[name].Schema
}

func (db *DB) Insert(name string, schema *spec.Schema, obj interface{}) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		// To be consistent with the other place, we will make a copy
		schemaCopy := *schema
		db.schemas[name] = &SchemaDB{name, (*Schema)(&schemaCopy), nil}
	}
	if !db.schemas[name].Schema.Matches(obj, db.Swagger) {
		return errors.New(fmt.Sprintf("object and schema doesn't match, name: %s obj type %v schema type %v",
			name, reflect.TypeOf(obj).Kind(), db.schemas[name].Schema.Type))
	}
	return db.schemas[name].Insert(obj)
}

func (db *DB) Find(name string, criteria interface{}, matches MatchFunc, desiredCount int) []interface{} {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		return nil
	}
	return db.schemas[name].Find(criteria, matches, desiredCount)
}

func (db *DB) Delete(name string, criteria interface{}, matches MatchFunc, desiredCount int) int {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		return 0
	}
	return db.schemas[name].Delete(criteria, matches, desiredCount)
}

func (db *DB) Update(name string, criteria interface{}, matches MatchFunc, newObj map[string]interface{}, desiredCount int, patch bool) int {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		return 0
	}
	return db.schemas[name].Update(criteria, matches, newObj, desiredCount, patch)
}

// FindMatchingSchema finds the schema that matches the obj.
func (db *DB) FindMatchingSchema(obj interface{}) (string, *spec.Schema) {
	for name, schemaDB := range db.schemas {
		schema := schemaDB.Schema
		if schema.Matches(obj, db.Swagger) {
			mqutil.Logger.Printf("found matching schema: %s", name)
			return name, (*spec.Schema)(schema)
		}
	}
	return "", nil
}

// DB holds schema name to Schema mapping.
var ObjDB DB

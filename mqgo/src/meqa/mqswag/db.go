package mqswag

import (
	"encoding/json"
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

// Prases the object against this schema. If the obj and schema doesn't match
// return an error. Otherwise parse all the objects identified by the schema
// into the map indexed by the object class name.
func (schema *Schema) Parses(name string, object interface{}, collection map[string][]interface{}, swagger *Swagger) error {
	raiseError := func() error {
		schemaBytes, _ := json.MarshalIndent((*spec.Schema)(schema), "", "    ")
		objectBytes, _ := json.MarshalIndent(object, "", "    ")
		return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf(
			"schema and object doesn't match. Schema:\n%s\nObject:\n%s\n",
			string(schemaBytes), string(objectBytes)))
	}
	if object == nil {
		return nil
	}
	refName, referredSchema, err := swagger.GetReferredSchema(schema)
	if err != nil {
		return err
	}
	if referredSchema != nil {
		return referredSchema.Parses(refName, object, collection, swagger)
	}

	if len(schema.AllOf) > 0 {
		for _, s := range schema.AllOf {
			// The name doesn't get passed down. The name is handled at the current level.
			err = ((*Schema)(&s)).Parses("", object, collection, swagger)
			if err != nil {
				return err
			}
		}

		// AllOf is satisfied. We can add the object to our collection
		if len(name) > 0 {
			collection[name] = append(collection[name], object)
		}
		return nil
	}

	k := reflect.TypeOf(object).Kind()
	if k == reflect.Bool {
		if !schema.Type.Contains(gojsonschema.TYPE_BOOLEAN) {
			return raiseError()
		}
	} else if k >= reflect.Int && k <= reflect.Uint64 {
		if !schema.Type.Contains(gojsonschema.TYPE_INTEGER) && !schema.Type.Contains(gojsonschema.TYPE_NUMBER) {
			return raiseError()
		}
	} else if k == reflect.Float32 || k == reflect.Float64 {
		// After unmarshal, the map only holds floats. It doesn't differentiate int and float.
		if !schema.Type.Contains(gojsonschema.TYPE_INTEGER) && !schema.Type.Contains(gojsonschema.TYPE_NUMBER) {
			return raiseError()
		}
	} else if k == reflect.String {
		if !schema.Type.Contains(gojsonschema.TYPE_STRING) {
			return raiseError()
		}
	} else if k == reflect.Map {
		objMap, objIsMap := object.(map[string]interface{})
		if !objIsMap || !schema.Type.Contains(gojsonschema.TYPE_OBJECT) {
			return raiseError()
		}
		for _, requiredName := range schema.Required {
			if _, exist := objMap[requiredName]; !exist {
				mqutil.Logger.Printf("required field not present: %s", requiredName)
				return raiseError()
			}
		}
		// descend into the object's properties. If all checks out we can add the object
		// to the collection under the current name.
		for propertyName, propertySchema := range schema.Properties {
			objProperty, exist := objMap[propertyName]
			if !exist {
				continue // we have checked for required fields already, so this is ok
			}
			err = ((*Schema)(&propertySchema)).Parses("", objProperty, collection, swagger)
			if err != nil {
				return err
			}
		}
		// all the properties are OK.
		if len(name) > 0 {
			collection[name] = append(collection[name], object)
		}
	} else if k == reflect.Array || k == reflect.Slice {
		if !schema.Type.Contains(gojsonschema.TYPE_ARRAY) {
			return raiseError()
		}
		// Check the array elements.
		itemsSchema := (*Schema)(schema.Items.Schema)
		if itemsSchema == nil && len(schema.Items.Schemas) > 0 {
			s := Schema(schema.Items.Schemas[0])
			itemsSchema = &s
		}
		if itemsSchema == nil {
			return raiseError()
		}
		ar := object.([]interface{})
		for _, item := range ar {
			err = itemsSchema.Parses("", item, collection, swagger)
			if err != nil {
				return err
			}
		}
	} else {
		mqutil.Logger.Printf("unknown type: %v", k)
		return raiseError()
	}
	return nil
}

// Matches checks if the Schema matches the input interface. In proper swagger.json
// Enums should have types as well. So we don't check for untyped enums.
// TODO check format, handle AllOf, AnyOf, OneOf
func (schema *Schema) Matches(object interface{}, swagger *Swagger) bool {
	err := schema.Parses("", object, make(map[string][]interface{}), swagger)
	return err == nil
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

	if len(schema.AllOf) > 0 {
		for _, s := range schema.AllOf {
			err = ((*Schema)(&s)).Iterate(iterFunc, context, swagger)
			if err != nil {
				return err
			}
		}
		return nil
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

type DBEntry struct {
	Data         map[string]interface{}            // The object itself.
	Associations map[string]map[string]interface{} // The objects associated with this object. Class to object map.
}

func (entry *DBEntry) Matches(criteria interface{}, associations map[string]map[string]interface{}, matches MatchFunc) bool {
	for className, classAssociation := range associations {
		if !mqutil.MatchAllFields(classAssociation, entry.Associations[className]) {
			return false
		}
	}
	return matches(criteria, entry.Data)
}

// SchemaDB is our in-memory DB. It is organized around Schemas. Each schema maintains a list of objects that matches
// the schema. We don't build indexes and do linear search. This keeps the searching flexible for now.
type SchemaDB struct {
	Name      string
	Schema    *Schema
	NoHistory bool
	Objects   []*DBEntry
}

// Insert inserts an object into the schema's object list.
func (db *SchemaDB) Insert(obj interface{}, associations map[string]map[string]interface{}) error {
	if !db.NoHistory {
		found := db.Find(obj, associations, mqutil.MatchAllFields, 1)
		if len(found) == 0 {
			dbentry := &DBEntry{obj.(map[string]interface{}), associations}
			db.Objects = append(db.Objects, dbentry)
		}
	}
	return nil
}

// MatchFunc checks whether the input criteria and an input object matches.
type MatchFunc func(criteria interface{}, existing interface{}) bool

func MatchAlways(criteria interface{}, existing interface{}) bool {
	return true
}

// Find finds the specified number of objects that match the input criteria.
func (db *SchemaDB) Find(criteria interface{}, associations map[string]map[string]interface{}, matches MatchFunc, desiredCount int) []interface{} {
	var result []interface{}
	for _, entry := range db.Objects {
		if entry.Matches(criteria, associations, matches) {
			result = append(result, entry.Data)
			if desiredCount >= 0 && len(result) >= desiredCount {
				return result
			}
		}
	}
	return result
}

// Delete deletes the specified number of elements that match the criteria. Input -1 for delete all.
// Returns the number of elements deleted.
func (db *SchemaDB) Delete(criteria interface{}, associations map[string]map[string]interface{}, matches MatchFunc, desiredCount int) int {
	count := 0
	for i, entry := range db.Objects {
		if entry.Matches(criteria, associations, matches) {
			db.Objects[i] = db.Objects[count]
			count++
			if desiredCount >= 0 && count >= desiredCount {
				break
			}
		}
	}
	db.Objects = db.Objects[count:]
	return count
}

// Update finds the matching object, then update with the new one.
func (db *SchemaDB) Update(criteria interface{}, associations map[string]map[string]interface{},
	matches MatchFunc, newObj map[string]interface{}, desiredCount int, patch bool) int {

	count := 0
	for _, entry := range db.Objects {
		if entry.Matches(criteria, associations, matches) {
			if patch {
				mqutil.MapCombine(entry.Data, newObj)
			} else {
				entry.Data = newObj
			}
			count++
			if desiredCount >= 0 && count >= desiredCount {
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

// TODO it seems that if an object is not being used as a parameter to any operation, we don't
// need to track it in DB. This will save some resources. We can do this by adding swagger to
// a dag, then iterate through all the objects, and find those that doesn't have any oepration
// as a child.
func (db *DB) Init(s *Swagger) {
	db.Swagger = s
	db.schemas = make(map[string](*SchemaDB))
	for schemaName, schema := range s.Definitions {
		if _, ok := db.schemas[schemaName]; ok {
			mqutil.Logger.Printf("warning - schema %s already exists", schemaName)
		}
		// Note that schema variable is reused in the loop
		schemaCopy := schema
		db.schemas[schemaName] = &SchemaDB{schemaName, (*Schema)(&schemaCopy), false, nil}
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

func CopyWithoutClass(src map[string]map[string]interface{}, className string) map[string]map[string]interface{} {
	dst := make(map[string]map[string]interface{})
	for k, v := range src {
		if k != className {
			dst[k] = v
		}
	}
	return dst
}

func (db *DB) Insert(name string, obj interface{}, associations map[string]map[string]interface{}) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		return mqutil.NewError(mqutil.ErrInternal, fmt.Sprintf("inserting into non-existing schema: %s", name))
	}
	return db.schemas[name].Insert(obj, CopyWithoutClass(associations, name))
}

func (db *DB) Find(name string, criteria interface{}, associations map[string]map[string]interface{},
	matches MatchFunc, desiredCount int) []interface{} {

	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		return nil
	}
	return db.schemas[name].Find(criteria, CopyWithoutClass(associations, name), matches, desiredCount)
}

func (db *DB) Delete(name string, criteria interface{}, associations map[string]map[string]interface{},
	matches MatchFunc, desiredCount int) int {

	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		return 0
	}
	return db.schemas[name].Delete(criteria, CopyWithoutClass(associations, name), matches, desiredCount)
}

func (db *DB) Update(name string, criteria interface{}, associations map[string]map[string]interface{},
	matches MatchFunc, newObj map[string]interface{}, desiredCount int, patch bool) int {

	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.schemas[name] == nil {
		return 0
	}
	return db.schemas[name].Update(criteria, CopyWithoutClass(associations, name), matches, newObj, desiredCount, patch)
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

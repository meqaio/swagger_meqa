package mqswag

const TYPE_OBJECT = "object"
const TYPE_BOOL = "boolean"
const TYPE_STRING = "string"

// This file implements the in-memory DB that holds all the entity objects.

type ProperInterface interface {
}

type EntityInterface interface {
	Type() string

	// PropertyType returns the type for the named property. Returns "" if not found.
	// For non-object typed entities, always return ""
	PropertyType(propertyName string) string

	// Whether the given object matches this entity - the object is the same type and has all the attributes.
	Matches(entity EntityInterface) bool
}

// Definition is the same as the swagger definition of type "object".
type ObjectDefinition struct {
	Name       string
	Properties map[string]EntityInterface
}

func (self *ObjectDefinition) Type() string {
	return "object"
}

func (self *ObjectDefinition) PropertyType(propertyName string) string {
	if property, ok := self.Properties[propertyName]; ok {
		return property.Type()
	}
	return ""
}

func (self *ObjectDefinition) Matches(entity EntityInterface) bool {
	if self.Type() != entity.Type() {
		return false
	}
	panic("not implemented")
	return true
}

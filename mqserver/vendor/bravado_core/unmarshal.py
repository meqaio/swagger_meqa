# -*- coding: utf-8 -*-
from six import iteritems

from bravado_core import formatter
from bravado_core import schema
from bravado_core.exception import SwaggerMappingError
from bravado_core.model import is_model
from bravado_core.model import MODEL_MARKER
from bravado_core.schema import collapsed_properties
from bravado_core.schema import get_spec_for_prop
from bravado_core.schema import handle_null_value
from bravado_core.schema import is_dict_like
from bravado_core.schema import is_list_like
from bravado_core.schema import SWAGGER_PRIMITIVES


def unmarshal_schema_object(swagger_spec, schema_object_spec, value):
    """Unmarshal the value using the given schema object specification.

    Unmarshaling includes:
    - transform the value according to 'format' if available
    - return the value in a form suitable for use. e.g. conversion to a Model
      type.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type schema_object_spec: dict
    :type value: int, float, long, string, unicode, boolean, list, dict, etc

    :return: unmarshaled value
    :rtype: int, float, long, string, unicode, boolean, list, dict, object (in
        the case of a 'format' conversion', or Model type
    """
    deref = swagger_spec.deref
    schema_object_spec = deref(schema_object_spec)

    obj_type = schema_object_spec.get('type')
    if not obj_type and 'allOf' in schema_object_spec:
        obj_type = 'object'

    if not obj_type:
        raise SwaggerMappingError(
            "The following schema object is missing a type field: {0}"
            .format(schema_object_spec.get('x-model', str(schema_object_spec))))

    if obj_type in SWAGGER_PRIMITIVES:
        return unmarshal_primitive(swagger_spec, schema_object_spec, value)

    if obj_type == 'array':
        return unmarshal_array(swagger_spec, schema_object_spec, value)

    if swagger_spec.config['use_models'] and \
            is_model(swagger_spec, schema_object_spec):
        # It is important that the 'model' check comes before 'object' check.
        # Model specs also have type 'object' but also have the additional
        # MODEL_MARKER key for identification.
        return unmarshal_model(swagger_spec, schema_object_spec, value)

    if obj_type == 'object':
        return unmarshal_object(swagger_spec, schema_object_spec, value)

    if obj_type == 'file':
        return value

    raise SwaggerMappingError(
        "Don't know how to unmarshal value {0} with a type of {1}"
        .format(value, obj_type))


def unmarshal_primitive(swagger_spec, primitive_spec, value):
    """Unmarshal a jsonschema primitive type into a python primitive.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type primitive_spec: dict
    :type value: int, long, float, boolean, string, unicode, etc

    :rtype: int, long, float, boolean, string, unicode, or an object
        based on 'format'
    :raises: SwaggerMappingError
    """
    if value is None:
        return handle_null_value(swagger_spec, primitive_spec)

    value = formatter.to_python(swagger_spec, primitive_spec, value)
    return value


def unmarshal_array(swagger_spec, array_spec, array_value):
    """Unmarshal a jsonschema type of 'array' into a python list.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type array_spec: dict
    :type array_value: list
    :rtype: list
    :raises: SwaggerMappingError
    """
    if array_value is None:
        return handle_null_value(swagger_spec, array_spec)

    if not is_list_like(array_value):
        raise SwaggerMappingError('Expected list like type for {0}:{1}'.format(
            type(array_value), array_value))

    item_spec = swagger_spec.deref(array_spec).get('items')
    return [
        unmarshal_schema_object(swagger_spec, item_spec, item)
        for item in array_value
    ]


def unmarshal_object(swagger_spec, object_spec, object_value):
    """Unmarshal a jsonschema type of 'object' into a python dict.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type object_spec: dict
    :type object_value: dict
    :rtype: dict
    :raises: SwaggerMappingError
    """
    deref = swagger_spec.deref

    if object_value is None:
        return handle_null_value(swagger_spec, object_spec)

    if not is_dict_like(object_value):
        raise SwaggerMappingError('Expected dict like type for {0}:{1}'.format(
            type(object_value), object_value))

    object_spec = deref(object_spec)
    required_fields = object_spec.get('required', [])

    result = {}
    for k, v in iteritems(object_value):
        prop_spec = get_spec_for_prop(
            swagger_spec, object_spec, object_value, k)
        if v is None and k not in required_fields and prop_spec:
            if schema.has_default(swagger_spec, prop_spec):
                result[k] = schema.get_default(swagger_spec, prop_spec)
            else:
                result[k] = None
        elif prop_spec:
            result[k] = unmarshal_schema_object(swagger_spec, prop_spec, v)
        else:
            # Don't marshal when a spec is not available - just pass through
            result[k] = v

    properties = collapsed_properties(deref(object_spec), swagger_spec)
    for prop_name, prop_spec in iteritems(properties):
        if prop_name not in result and swagger_spec.config['include_missing_properties']:
            result[prop_name] = None
            if schema.has_default(swagger_spec, prop_spec):
                result[prop_name] = schema.get_default(swagger_spec, prop_spec)

    return result


def unmarshal_model(swagger_spec, model_spec, model_value):
    """Unmarshal a dict into a Model instance.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type model_spec: dict
    :type model_value: dict
    :rtype: Model instance
    :raises: SwaggerMappingError
    """
    deref = swagger_spec.deref
    model_name = deref(model_spec).get(MODEL_MARKER)
    model_type = swagger_spec.definitions.get(model_name, None)

    if model_type is None:
        raise SwaggerMappingError(
            'Unknown model {0} when trying to unmarshal {1}'
            .format(model_name, model_value))

    if model_value is None:
        return handle_null_value(swagger_spec, model_spec)

    if not is_dict_like(model_value):
        raise SwaggerMappingError(
            "Expected type to be dict for value {0} to unmarshal to a {1}."
            "Was {2} instead."
            .format(model_value, model_type, type(model_value)))

    # Check if model is polymorphic
    discriminator = model_spec.get('discriminator')
    if discriminator is not None:
        child_model_name = model_value.get(discriminator, None)
        if child_model_name not in swagger_spec.definitions:
            raise SwaggerMappingError(
                'Unknown model {0} when trying to unmarshal {1}. '
                'Value of {2}\'s discriminator {3} did not match any definitions.'
                .format(child_model_name, model_value, model_name, discriminator)
            )
        model_type = swagger_spec.definitions.get(child_model_name)
        model_spec = model_type._model_spec

    model_as_dict = unmarshal_object(swagger_spec, model_spec, model_value)
    model_instance = model_type._from_dict(model_as_dict)
    return model_instance

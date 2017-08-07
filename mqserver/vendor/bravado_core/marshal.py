# -*- coding: utf-8 -*-
from six import iteritems

from bravado_core import formatter
from bravado_core import schema
from bravado_core.exception import SwaggerMappingError
from bravado_core.model import is_model
from bravado_core.model import is_object
from bravado_core.model import MODEL_MARKER
from bravado_core.schema import get_spec_for_prop
from bravado_core.schema import handle_null_value
from bravado_core.schema import is_dict_like
from bravado_core.schema import is_list_like
from bravado_core.schema import SWAGGER_PRIMITIVES


def marshal_schema_object(swagger_spec, schema_object_spec, value):
    """Marshal the value using the given schema object specification.

    Marshaling includes:
    - transform the value according to 'format' if available
    - return the value in a form suitable for 'on-the-wire' transmission

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type schema_object_spec: dict
    :type value: int, long, string, unicode, boolean, list, dict, Model type

    :return: marshaled value
    :rtype: int, long, string, unicode, boolean, list, dict
    :raises: SwaggerMappingError
    """
    deref = swagger_spec.deref
    schema_object_spec = deref(schema_object_spec)
    obj_type = schema_object_spec.get('type')

    if obj_type in SWAGGER_PRIMITIVES:
        return marshal_primitive(swagger_spec, schema_object_spec, value)

    if obj_type == 'array':
        return marshal_array(swagger_spec, schema_object_spec, value)

    if is_model(swagger_spec, schema_object_spec):

        # Allow models to be passed in as dicts for flexibility.
        if is_dict_like(value):
            return marshal_object(swagger_spec, schema_object_spec, value)

        # It is important that the 'model' check comes before 'object' check
        # below. Model specs are of type 'object' but also have a MODEL_MARKER
        # key for identification.
        return marshal_model(swagger_spec, schema_object_spec, value)

    if is_object(swagger_spec, schema_object_spec):
        return marshal_object(swagger_spec, schema_object_spec, value)

    if obj_type == 'file':
        return value

    raise SwaggerMappingError('Unknown type {0} for value {1}'.format(
        obj_type, value))


def marshal_primitive(swagger_spec, primitive_spec, value):
    """Marshal a python primitive type into a jsonschema primitive.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type primitive_spec: dict
    :type value: int, long, float, boolean, string, unicode, or an object
        based on 'format'

    :rtype: int, long, float, boolean, string, unicode, etc
    :raises: SwaggerMappingError
    """
    default_used = False

    if value is None and schema.has_default(swagger_spec, primitive_spec):
        default_used = True
        value = schema.get_default(swagger_spec, primitive_spec)

    if value is None:
        return handle_null_value(swagger_spec, primitive_spec)

    if not default_used:
        value = formatter.to_wire(swagger_spec, primitive_spec, value)

    return value


def marshal_array(swagger_spec, array_spec, array_value):
    """Marshal a jsonschema type of 'array' into a json-like list.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type array_spec: dict
    :type array_value: list
    :rtype: list
    :raises: SwaggerMappingError
    """
    if array_value is None:
        return handle_null_value(swagger_spec, array_spec)

    if not is_list_like(array_value):
        raise SwaggerMappingError('Expected list like type for {0}: {1}'
                                  .format(type(array_value), array_value))

    items_spec = swagger_spec.deref(array_spec).get('items')

    return [
        marshal_schema_object(
            swagger_spec,
            items_spec,
            element)
        for element in array_value
    ]


def marshal_object(swagger_spec, object_spec, object_value):
    """Marshal a python dict to json dict.

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

        if v is None and k not in required_fields:
            continue
        if prop_spec:
            result[k] = marshal_schema_object(swagger_spec, prop_spec, v)
        else:
            # Don't marshal when a spec is not available - just pass through
            result[k] = v

    return result


def marshal_model(swagger_spec, model_spec, model_value):
    """Marshal a Model instance into a json-like dict.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type model_spec: dict
    :type model_value: Model instance
    :rtype: dict
    :raises: SwaggerMappingError
    """
    deref = swagger_spec.deref
    model_name = deref(model_spec).get(MODEL_MARKER)
    model_type = swagger_spec.definitions.get(model_name, None)

    if model_type is None:
        raise SwaggerMappingError('Unknown model {0}'.format(model_name))

    if model_value is None:
        return handle_null_value(swagger_spec, model_spec)

    if not model_type._isinstance(model_value):
        raise SwaggerMappingError(
            'Expected model of type {0} for {1}:{2}'
            .format(model_name, type(model_value), model_value))

    # just convert the model to a dict and feed into `marshal_object` because
    # models are essentially 'type':'object' when marshaled
    object_value = model_value._as_dict()

    return marshal_object(swagger_spec, model_spec, object_value)

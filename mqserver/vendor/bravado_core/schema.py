# -*- coding: utf-8 -*-
import copy
from collections import Mapping

from six import iteritems

from bravado_core.exception import SwaggerMappingError


# 'object' and 'array' are omitted since this should really be read as
# "Swagger types that map to python primitives"
SWAGGER_PRIMITIVES = (
    'integer',
    'number',
    'string',
    'boolean',
    'null',
)


def has_default(swagger_spec, schema_object_spec):
    return 'default' in swagger_spec.deref(schema_object_spec)


def get_default(swagger_spec, schema_object_spec):
    return swagger_spec.deref(schema_object_spec).get('default')


def is_required(swagger_spec, schema_object_spec):
    return swagger_spec.deref(schema_object_spec).get('required', False)


def has_format(swagger_spec, schema_object_spec):
    return 'format' in swagger_spec.deref(schema_object_spec)


def get_format(swagger_spec, schema_object_spec):
    return swagger_spec.deref(schema_object_spec).get('format')


def is_param_spec(swagger_spec, schema_object_spec):
    return 'in' in swagger_spec.deref(schema_object_spec)


def is_prop_nullable(swagger_spec, schema_object_spec):
    return swagger_spec.deref(schema_object_spec).get('x-nullable', False)


def is_ref(spec):
    return is_dict_like(spec) and '$ref' in spec


def is_dict_like(spec):
    """
    :param spec: swagger object specification in dict form
    :rtype: boolean
    """
    return isinstance(spec, Mapping)


def is_list_like(spec):
    """
    :param spec: swagger object specification in dict form
    :rtype: boolean
    """
    return isinstance(spec, (list, tuple))


def get_spec_for_prop(swagger_spec, object_spec, object_value, prop_name):
    """Given a jsonschema object spec and value, retrieve the spec for the
     given property taking 'additionalProperties' into consideration.

    :param object_spec: spec for a jsonschema 'object' in dict form
    :param object_value: jsonschema object containing the given property. Only
        used in error message.
    :param prop_name: name of the property to retrieve the spec for

    :return: spec for the given property or None if no spec found
    :rtype: dict or None
    """
    deref = swagger_spec.deref

    properties = collapsed_properties(deref(object_spec), swagger_spec)
    prop_spec = properties.get(prop_name)

    if prop_spec is not None:
        result_spec = deref(prop_spec)
        # If the de-referenced specification is for a x-nullable property
        # then copy the spec and add the x-nullable property.
        # If in the future there are other attributes on the property that
        # modify a referenced schema, it can be done here (or rewrite
        # unmarshal to pass the unreferenced property spec as another arg).
        if 'x-nullable' in prop_spec and 'x-nullable' not in result_spec:
            result_spec = copy.deepcopy(result_spec)
            result_spec['x-nullable'] = prop_spec['x-nullable']
        return result_spec

    additional_props = deref(object_spec).get('additionalProperties', True)

    if isinstance(additional_props, bool):
        # no spec for additional properties to conform to - this is basically
        # a way to send pretty much anything across the wire as is.
        return None

    additional_props = deref(additional_props)
    if is_dict_like(additional_props):
        # spec that all additional props MUST conform to
        return additional_props

    raise SwaggerMappingError(
        "Don't know what to do with `additionalProperties` in spec {0} "
        "when inspecting value {1}".format(object_spec, object_value))


def handle_null_value(swagger_spec, schema_object_spec):
    """Handle a null value for the associated schema object spec. Checks the
     x-nullable attribute in the spec to see if it is allowed and returns None
     if so and raises an exception otherwise.

    :param swagger_spec: :class:`bravado_core.spec.Spec`
    :param schema_object_spec: dict
    :return: The default if there is a default value, None if the spec is nullable
    :raises: SwaggerMappingError if the spec is not nullable and no default exists
    """
    if has_default(swagger_spec, schema_object_spec):
        return get_default(swagger_spec, schema_object_spec)

    if is_prop_nullable(swagger_spec, schema_object_spec):
        return None

    raise SwaggerMappingError(
        'Spec {0} is a required value'.format(schema_object_spec))


def collapsed_properties(model_spec, swagger_spec):
    """Processes model spec and outputs dictionary with attributes
    as the keys and attribute spec as the value for the model.

    This handles traversing any polymorphic models and the hierarchy
    of properties properly.

    :param model_spec: model specification (must be dereferenced already)
    :type model_spec: dict
    :param swagger_spec: :class:`bravado_core.spec.Spec`
    :returns: dict
    """

    properties = {}

    # properties may or may not be present
    if 'properties' in model_spec:
        for attr, attr_spec in iteritems(model_spec['properties']):
            properties[attr] = attr_spec

    # allOf may or may not be present
    if 'allOf' in model_spec:
        deref = swagger_spec.deref
        for item_spec in model_spec['allOf']:
            item_spec = deref(item_spec)
            more_properties = collapsed_properties(item_spec, swagger_spec)
            properties.update(more_properties)

    return properties

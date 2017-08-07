# -*- coding: utf-8 -*-
"""
Support for the 'format' key in the swagger spec as outlined in
https://github.com/swagger-api/swagger-spec/blob/master/versions/2.0.md#dataTypeFormat
"""
import functools
from collections import namedtuple

import dateutil.parser
import pytz
import six

from bravado_core import schema

if six.PY3:
    long = int


def NO_OP(x):
    return None


def to_wire(swagger_spec, primitive_spec, value):
    """Converts a python primitive or object to a reasonable wire
    representation if it has an associated Swagger `format`.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :param primitive_spec: spec for a primitive type as a dict
    :param value: primitive to convert to wire representation
    :type value: int, long, float, boolean, string, unicode, object, etc
    :rtype: int, long, float, boolean, string, unicode, etc
    """
    if value is None or not schema.has_format(swagger_spec, primitive_spec):
        return value
    format_name = schema.get_format(swagger_spec, primitive_spec)
    formatter = swagger_spec.get_format(format_name)
    return formatter.to_wire(value) if formatter else value


def to_python(swagger_spec, primitive_spec, value):
    """Converts a value in wire format to its python representation if
     it has an associated Swagger `format`.

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :param primitive_spec: spec for a primitive type as a dict
    :type value: int, long, float, boolean, string, unicode, etc
    :rtype: int, long, float, boolean, string, object, etc
    """
    if value is None or not schema.has_format(swagger_spec, primitive_spec):
        return value
    format_name = schema.get_format(swagger_spec, primitive_spec)
    formatter = swagger_spec.get_format(format_name)
    return formatter.to_python(value) if formatter else value


class SwaggerFormat(namedtuple('SwaggerFormat',
                               'format to_python to_wire validate description')):
    """User-defined format which can be registered with a
    :class:`bravado_core.spec.Spec` to handle marshalling to wire format,
    unmarshalling to a python type, and format specific validation.

    :param format: Name for the user-defined format.
    :param to_python: function to unmarshal a value of this format.
        Eg. lambda val_str: base64.b64decode(val_str)
    :param to_wire: function to marshal a value of this format
        Eg. lambda val_py: base64.b64encode(val_py)
    :param validate: function to validate the correctness of the `wire` value.
        It should raise :class:`bravado_core.exception.SwaggerValidationError`
        if the value does not conform to the format.
    :param description: Short description of the format and conversion logic.
    """


def return_true_wrapper(validate_func):
    """Decorator for the SwaggerFormat.validate function to always return True.

    The contract for `SwaggerFormat.validate` is to raise an exception
    when validation fails. However, the contract for jsonschema's
    validate function is to raise an exception or return True. This wrapper
    bolts-on the `return True` part.

    :param validate_func: SwaggerFormat.validate function
    :return: wrapped callable
    """
    @functools.wraps(validate_func)
    def wrapper(validatable_primitive):
        validate_func(validatable_primitive)
        return True

    return wrapper


DEFAULT_FORMATS = {
    'byte': SwaggerFormat(
        format='byte',
        to_wire=lambda b: b if isinstance(b, str) else str(b),
        to_python=lambda s: s if isinstance(s, str) else str(s),
        validate=NO_OP,  # jsonschema validates string
        description='Converts [wire]string:byte <=> python byte'),
    'date': SwaggerFormat(
        format='date',
        to_wire=lambda d: d.isoformat(),
        to_python=lambda d: dateutil.parser.parse(d).date(),
        validate=NO_OP,  # jsonschema validates date
        description='Converts [wire]string:date <=> python datetime.date'),
    # Python has no double. float is C's double in CPython
    'double': SwaggerFormat(
        format='double',
        to_wire=lambda d: d if isinstance(d, float) else float(d),
        to_python=lambda d: d if isinstance(d, float) else float(d),
        validate=NO_OP,  # jsonschema validates number
        description='Converts [wire]number:double <=> python float'),
    'date-time': SwaggerFormat(
        format='date-time',
        to_wire=lambda dt: (dt if dt.tzinfo else pytz.utc.localize(dt)).isoformat(),
        to_python=lambda dt: dateutil.parser.parse(dt),
        validate=NO_OP,  # jsonschema validates date-time
        description=(
            'Converts string:date-time <=> python datetime.datetime')),
    'float': SwaggerFormat(
        format='float',
        to_wire=lambda f: f if isinstance(f, float) else float(f),
        to_python=lambda f: f if isinstance(f, float) else float(f),
        validate=NO_OP,  # jsonschema validates number
        description='Converts [wire]number:float <=> python float'),
    'int32': SwaggerFormat(
        format='int32',
        to_wire=lambda i: i if isinstance(i, int) else int(i),
        to_python=lambda i: i if isinstance(i, int) else int(i),
        validate=NO_OP,  # jsonschema validates integer
        description='Converts [wire]integer:int32 <=> python int'),
    'int64': SwaggerFormat(
        format='int64',
        to_wire=lambda i: i if isinstance(i, long) else long(i),
        to_python=lambda i: i if isinstance(i, long) else long(i),
        validate=NO_OP,  # jsonschema validates integer
        description='Converts [wire]integer:int64 <=> python long'),
}

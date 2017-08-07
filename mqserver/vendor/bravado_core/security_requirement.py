# -*- coding: utf-8 -*-
import logging

import six

from bravado_core.exception import SwaggerSchemaError

log = logging.getLogger(__name__)


class SecurityRequirement(object):
    """
    Wrapper of security requirement object (http://swagger.io/specification/#securityRequirementObject)

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type op: :class:`bravado_core.operation.Operation`
    :type security_requirement_spec: security requirement specification in dict form
    """

    def __init__(self, swagger_spec, security_requirement_spec):
        self.swagger_spec = swagger_spec
        self.security_requirement_spec = swagger_spec.deref(security_requirement_spec)
        for security_definition in six.iterkeys(security_requirement_spec):
            if security_definition not in self.swagger_spec.security_definitions:
                raise SwaggerSchemaError(
                    '{security} not defined in {swagger_path}'.format(
                        swagger_path='/securityDefinitions',
                        security=security_definition,
                    )
                )

    @property
    def security_definitions(self):
        return dict(
            (security_name, self.swagger_spec.security_definitions[security_name])
            for security_name in six.iterkeys(self.security_requirement_spec)
        )

    @property
    def security_scopes(self):
        return dict(
            (security_name, self.security_requirement_spec[security_name])
            for security_name in six.iterkeys(self.security_requirement_spec)
        )

    @property
    def parameters_representation_dict(self):
        return [
            definition.parameter_representation_dict
            for definition in six.itervalues(self.security_definitions)
            if definition.parameter_representation_dict
        ]

    def __iter__(self):
        return six.itervalues(self.security_definitions)

# -*- coding: utf-8 -*-
import logging

log = logging.getLogger(__name__)


class SecurityDefinition(object):
    """
    Wrapper of security definition object (http://swagger.io/specification/#securityDefinitionsObject)

    :type swagger_spec: :class:`bravado_core.spec.Spec`
    :type security_definition_spec: security definition specification in dict form
    """

    def __init__(self, swagger_spec, security_definition_spec):
        self.swagger_spec = swagger_spec
        self.security_definition_spec = swagger_spec.deref(security_definition_spec)

    @property
    def location(self):
        # not using 'in' as the name since it is a keyword in python
        return self.security_definition_spec.get('in')

    @property
    def type(self):
        return self.security_definition_spec['type']

    @property
    def name(self):
        return self.security_definition_spec.get('name')

    @property
    def flow(self):
        return self.security_definition_spec.get('flow')

    @property
    def scopes(self):
        return self.security_definition_spec.get('scopes')

    @property
    def authorizationUrl(self):
        return self.security_definition_spec.get('authorizationUrl')

    @property
    def parameter_representation_dict(self):
        if self.type == 'apiKey':
            return {
                'required': False,
                'type': 'string',
                'description': self.security_definition_spec.get('description', ''),
                'name': self.name,
                'in': self.location,
            }

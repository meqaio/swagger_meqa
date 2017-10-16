# Meqa Tags and Test Suite File Format

## Meqa Tags

Meqa inserts <meqa ...> tags inside a Swagger spec to show its understanding of the spec. The generation and running of test suites depend on this understanding being correct.

The meqa tags are always appended at the end of a relevant Swagger entity's description field. The format is <meqa DefinitionName.PropertyName.MethodType flags>. It means that the tagged entity actually refers to the entity in the tag.

* DefinitionName - the Swagger definition's name.
* PropertyName - the property of the above definition.
* MethodType - one of the http methods (e.g. post). This part is only present when we want to override the meaning of the tagged operation. For instance, if the tagged operation is a POST operation, but is actually changing an existing object and thus will be tagged "put". Note that the methods in meqa tags should always be in lower case.
* Flags - the only flag we support is "weak", indicating a weak reference to break circular dependency.

Example, in the petstore spec, the <meqa Pet.id> tag is put on the petId parameter, to indicate that when making a REST call, this parameter should be filled using a Pet object's id property.
```
  /pet/{petId}:
    delete:
      description: ' <meqa Pet>'
      operationId: deletePet
      parameters:
      - description: Pet id to delete <meqa Pet.id>
        format: int64
        in: path
        name: petId
        required: true
        type: integer
```

## 
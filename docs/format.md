# Meqa Tags and Test Plan File Format

Here we explain the meqa tag's meaning and the meqa test plan file structure. The examples in this document are all based on the Swagger's demo petstore (http://petstore.swagger.io). You can download the swagger spec at http://petstore.swagger.io/v2/swagger.yaml.

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

## Test Suite Format

Each test plan yaml file has multiple test suites separated by '---'. Each test suite can have multiple tests. In the following example, the name of the test suite is "/store/order". The test suites are executed in sequential order.

```
---
/store/order:
- name: post_placeOrder_1
  path: /store/order
  method: post
- name: get_getOrderById_2
  path: /store/order/{orderId}
  method: get
- name: delete_deleteOrder_3
  path: /store/order/{orderId}
  method: delete
- name: get_getOrderById_4
  path: /store/order/{orderId}
  method: get
  expect:
    status: fail
  pathParams:
    orderId: '{{delete_deleteOrder_3.pathParams.orderId}}'
```

In this test suite, there are four tests, triggering the following REST calls to the host specified in the swagger spec. 

* POST /store/order
* GET /store/order/{orderId}
* DELETE /store/order/{orderId}
* GET /store/order/{orderId}

Note that the first three tests don't specify any parameters. By default meqa will try to pick good parameters. When placing an order, meqa will order a pet with an existing Pet.id. When getting/deleting an order, meqa will fill the {orderId} path parameter with the Order.id of the order we just placed.

The last test tries to get the order we just deleted, and expects to get a failure. In this case it explicitly sets a path parameter. The following keywords are allowed, mapping to the respective REST call parameter location.

* pathParams
* queryParams
* bodyParams
* formParams
* headerParams

When setting parameters, the value can be either a explicit value, or a template. A template has the format of '{{testName.parameterLocation.parameterName...}}'.

* testName - the name of a test.
* parameterLocation - where the parameter comes from. It can be either one of pathParams, queryParams, bodyParams, formParams, headerParams, outputs.
* parameterName - the name to look for under parameterLocation whose value is to be used as this template's value. This name can be in the form of "object.property.property...". When parameterName is just one single value without any ".", meqa will try to find a named entity that matches the parameterName.

In the above example, the template '{{delete_deleteOrder_3.pathParams.orderId}}' maps to the "orderId" path param of test "delete_deleteOrder_3".

As another example, the last test can use the following parameter template to achieve the same result:

```
- name: get_getOrderById_4
  path: /store/order/{orderId}
  method: get
  expect:
    status: fail
  pathParams:
    orderId: '{{post_placeOrder_1.outputs.id}}'
```

## Test Plan Init Section

The first test suite can have a special "meqa_init" name. The parameters under meqa_init will be applied to all the test suites in the same file. For instance, in the following code that runs against bitbucket's API, we tell all the tests to use a specific username and repo_slug.

```
---
meqa_init:
- name: meqa_init
  pathParams:
    username: meqatest
    repo_slug: swagger_repo_1
```

Similarly, each test suite can have its own meqa_init section, to set a parameter for all the tests in that test suite. For instance, the following will hardcode all the "orderId" values in path to be 800800, as well as all the "id" values in body.

```
/store/order:
- name: meqa_init
  pathParams:
    orderId: 800800
  bodyParams:
    id: 800800
- name: post_placeOrder_1
  path: /store/order
  method: post
- name: get_getOrderById_2
  path: /store/order/{orderId}
  method: get
```

## Test Result File

When running mqgo you must provide a meqa directory through "-d" option. In this directory you will find a result.yml file after you do "mqgo run". The result.yml has the same format as the test plan file, and lists all the tests in the last run, with all the parameter and expect values being the actual vaules used.

Besides checking the actual values returned from the REST server, you can also feed result.yml back to "mqgo run" as the input test plan file through "-p". This allows you to check whether the same input will always get the same output.
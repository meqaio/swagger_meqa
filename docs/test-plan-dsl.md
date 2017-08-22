# Test Plan DSL #

## Introduction
The meqa test plan is a domain specific language (DSL) that is in YAML format.

## Example
We start with an example to give an idea about the elements in the DSL.
```yaml

---
create user auto:
  - path: /user
    method: post

---
get user auto:
  - path: /user/{username}
    method: get

---
create verify:
  - name: create
    ref: create user auto
  - name: read
    ref: get user auto
    pathParams:
      username: <create.username>
  - name: read non-exist
    ref: get user auto
    pathParams:
      username: a random non-existing string
    expect:
      status: fail

```
## Test suite object
Each test suite has a unique name. It can be run individually by name. A test suite is comprised of a list
of tests. In the above example "create user auto", "get user auto" and "create verify" are test suites.

The test suite can have parameters just like the test objects. The rule is that the lower level overrides the
higher level. So if the test suite and the underlying test has the same parameter, the one specified in the test
takes priority.

## Test Object
A test object defines a single REST API call, and has the following fields.
* name: an optional name. The name is useful when multiple tests in a test case need to refer to each other.
* path: the relative path to root url
* method: the http method, e.g. GET, POST
* ref: refers to another test suite by name. This overrides (path, method)
* pathParams: a map of key value pairs to be used in the REST call path (e.g. /user/{username})
* queryParams: similar to pathParams but for query parameters
* headerParams: header parameters
* formParams: formData
* bodyParams: the REST call http body payload. Can be map, array or string.
* expect: by default we verify that a REST call returned successfully. For failure testing cases the expect field
          can be used to tell MEQA engine that this test is expecting a different status.

## Parameters
The parameters allow special values, and defaults to the special value "auto". The meaning of special values are:
* auto - pick a known good value. While we execute the tests, we keep track of the objects created and deleted. We can choose a good value based on what we know. 
    - GET - use the right parameter to retrieve one known existing object. 
    - POST - use the parameters of the right types and falls in the parameter range.
    - PUT/PATCH - similar to GET.
    - DELETE - similar to GET.
* all - iterate through all the known good values.
    - GET - get all objects one by one and verifies the values.
    - PUT/PATCH - update all objects.
    - DELETE - delete all known objects.
* `<test name.parameter name>` - use the parameter value in a previous test.

The rule for parameters when there is a name conflict are:
* The same syntax is to specify parameters at the test plan level, test suite level, and individual test level.
* Specified parameters will override generated parameters.
* Parameters specified at lower level will override the parameters at higher level. This is just like child classes can override parent classes.
* The above means that if a bottom level test hardcodes a certain parameter, we can't call that test by reference and change the parameter.
* For parameters that have a name (field in a map), it will only affect the property with that name. For instance, if the post body needs username and password as two properties, if the caller specifies the username, the password will be generated.

Each test suite can by run by itself. Each one is always run using the existing in-memory state. For instance, if a GET type method is run by itself, it will try to retrieve object the test has created before. If there doesn't exist any object, the test will just verify the failure case.

When running a whole test plan, all the tests are run in sequence.

## Open Questions
* How to specify empty map or arrays for negative tests?
    - Use null value
# Test Plan DSL #

## Introduction
The meqa test plan is a domain specific language (DSL) that is in YAML format.

## Example
We start with an example to give an idea about the elements in the DSL.
```yaml


```

## Test Object
A test object is a basic component of the test plan. It defines a test, and has the following fields.
* path: the relative path to root url
* method: the http method, e.g. GET, POST
* ref: refers to another test object by name. This overrides (path, method)
* parameters: a list of parameters
    - name: value
    - special key: in, e.g. in: path

The parameters allow special values, and defaults to the special value "auto". The meaning of special values are:
* auto - pick a known good value. While we execute the tests, we keep track of the objects created and deleted. We can choose a good value based on what we know. 
    - GET - use the right parameter to retrieve one known existing object and verifies the result. Use a parameter to try to retrieve one known inexisting object and verifies the failure.
    - POST - use the parameters of the right types and falls in the parameter range.
    - UPDATE - similar to GET.
    - DELETE - similar to GET.
* all - iterate through all the known good values.
    - GET - get all objects one by one and verifies the values.
    - UPDATE - update all objects and verifies that http return result.
    - DELETE - delete all known objects.

The parameter can be a map or an array. Parameters passed in from caller will override the lower level test cases' parameters. For instance, if a test named "create user" is constructued to create a user named "user1", and in the test plan we call "create user" with parameters "user2", then we will use "user2".

Each test object can by run by itself. Each one is always run using the existing in-memory state. For instance, if a GET type method is run by itself, it will try to retrieve object the test has created before. If there doesn't exist any object, the test will just verify the failure case.

When running a whole test plan, all the tests are run in sequence.

## Open Questions
* How to specify empty map or arrays for negative tests?
    - Use null value
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"meqa/mqplan"
	"meqa/mqswag"
	"meqa/mqutil"
	"path/filepath"

	"github.com/go-openapi/spec"
)

const (
	meqaDataDir = "meqa_data"
	swaggerFile = "swagger.yaml"
)

func main() {
	mqutil.Logger = mqutil.NewStdLogger()

	meqaPath := flag.String("meqa", meqaDataDir, "the directory that holds the meqa data and swagger.yaml files")
	inputFile := flag.String("input", swaggerFile, "the swagger spec file name")
	outputFile := flag.String("output", "swagger_tagged", "the new swagger file name, we will generated swagger_tagged.json and swagger_tagged.yaml by default")
	yesToAll := flag.Bool("y", false, "yes to all the replacements")

	flag.Parse()
	inputPath := filepath.Join(*meqaPath, *inputFile)
	outputYamlPath := filepath.Join(*meqaPath, *outputFile+".yaml")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		mqutil.Logger.Printf("can't load swagger file at the following location %s", inputPath)
		return
	}
	if _, err := os.Stat(outputYamlPath); !os.IsNotExist(err) {
		mqutil.Logger.Printf("file exists already at the output location %s", outputYamlPath)
		return
	}
	// loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(inputPath, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		return
	}

	varMap := make(map[string]*mqswag.MeqaTag)
	pathMap := swagger.Paths.Paths

	gatherTags := func(params []spec.Parameter) {
		for _, param := range params {
			if tag := mqswag.GetMeqaTag(param.Description); tag != nil {
				if o := varMap[param.Name]; o == nil {
					varMap[param.Name] = tag
				} else {
					if !tag.Equals(o) {
						mqutil.Logger.Printf("%s has conflicting tags: %s %s", param.Name, tag.ToString(), o.ToString())
					}
				}
			}
		}
	}
	// first, collect all the tags.
	for _, pathItem := range pathMap {
		gatherTags(pathItem.Parameters)
		for _, opName := range mqswag.MethodAll {
			op := mqplan.GetOperationByMethod(&pathItem, opName)
			if op != nil {
				gatherTags(op.Parameters)
			}
		}
	}

	// We don't want to ask the user for the tags they have done themselves already. Go through
	// the swagger parameters and look for the ones we would tag given what we have.
	newMap := make(map[string]*mqswag.MeqaTag)
	findPotentialTags := func(params []spec.Parameter) {
		for _, param := range params {
			t := varMap[param.Name]
			if t != nil {
				existingTag := mqswag.GetMeqaTag(param.Description)
				if existingTag == nil {
					newMap[param.Name] = t
				}
			}
		}
	}

	// Go through all the parameters, and tag the ones matching varMap.
	for _, pathItem := range pathMap {
		findPotentialTags(pathItem.Parameters)
		for _, opName := range mqswag.MethodAll {
			op := mqplan.GetOperationByMethod(&pathItem, opName)
			if op != nil {
				findPotentialTags(op.Parameters)
			}
		}
	}

	varMap = newMap

	// Then go through all the parameters and propose tags.
	ask := !(*yesToAll)
	for paramName, paramTag := range varMap {
		if !ask {
			break
		}
		for {
			answer := ""
			fmt.Printf("Do you want to tag all %s as %s (y(es)/n(o)/a(ll))?", paramName, paramTag.ToString())
			fmt.Scanln(&answer)
			if answer[0] == 'y' {
				break
			} else if answer[0] == 'n' {
				varMap[paramName] = nil
				break
			} else if answer[0] == 'a' {
				ask = false
				break
			}
		}
	}

	placeTags := func(params []spec.Parameter) {
		for i, param := range params {
			t := varMap[param.Name]
			if t != nil {
				existingTag := mqswag.GetMeqaTag(param.Description)
				if existingTag == nil {
					params[i].Description = fmt.Sprintf("%s %s", param.Description, t.ToString())
				}
			}
		}
	}

	// Go through all the parameters, and tag the ones matching varMap.
	for _, pathItem := range pathMap {
		placeTags(pathItem.Parameters)
		for _, opName := range mqswag.MethodAll {
			op := mqplan.GetOperationByMethod(&pathItem, opName)
			if op != nil {
				placeTags(op.Parameters)
			}
		}
	}

	specSwagger := ((*spec.Swagger)(swagger))
	newSwaggerBytes, err := mqutil.MarshalJsonIndentNoEscape(specSwagger)
	if err != nil {
		mqutil.Logger.Fatal(err)
	}

	newYamlBytes, err := mqutil.JsonToYaml(newSwaggerBytes)
	if err != nil {
		mqutil.Logger.Fatal(err)
	}

	err = ioutil.WriteFile(outputYamlPath, newYamlBytes, 0644)
	if err != nil {
		mqutil.Logger.Fatal(err)
	}
}

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"meqa/mqplan"
	"meqa/mqswag"
	"meqa/mqutil"
	"path/filepath"

	uuid "github.com/satori/go.uuid"
	"gopkg.in/resty.v0"
	"gopkg.in/yaml.v2"
)

const (
	meqaDataDir = "meqa_data"
	configFile  = ".config.yml"
	resultFile  = "result.yml"
	serverURL   = "https://api.meqa.io"
)

const (
	configAPIKey       = "api_key"
	configAcceptedTerm = "terms_accepted"
)

func writeConfigFile(configPath string, configMap map[string]interface{}) error {
	configBytes, err := yaml.Marshal(configMap)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(configPath, configBytes, 0644)
	if err != nil {
		return err
	}
	return nil
}

func getConfigs(meqaPath string) (map[string]interface{}, error) {
	configMap := make(map[string]interface{})
	configPath := filepath.Join(meqaPath, configFile)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		u, _ := uuid.NewV4()
		configMap[configAPIKey] = u.String()
		err = writeConfigFile(configPath, configMap)
		if err != nil {
			return nil, err
		}
		return configMap, nil
	}
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(configBytes, &configMap)
	if err != nil {
		return nil, err
	}
	return configMap, nil
}

func generateMeqa(meqaPath string, swaggerPath string) error {
	caPool := x509.NewCertPool()
	permCert := `-----BEGIN CERTIFICATE-----
MIIDVzCCAj+gAwIBAgIJAJOCmHT8l8H6MA0GCSqGSIb3DQEBCwUAMEIxCzAJBgNV
BAYTAlVTMQswCQYDVQQIDAJDQTEQMA4GA1UECgwHbWVxYS5pbzEUMBIGA1UEAwwL
YXBpLm1lcWEuaW8wHhcNMTcxMDEwMDQzMTQ0WhcNMjcxMDA4MDQzMTQ0WjBCMQsw
CQYDVQQGEwJVUzELMAkGA1UECAwCQ0ExEDAOBgNVBAoMB21lcWEuaW8xFDASBgNV
BAMMC2FwaS5tZXFhLmlvMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA
zrqFruzqLoQDDo+RnjmiBQRTz5P2thD57UCKZXASoTbGL/RppR1bgq+87WTGyqij
5+NSNbwzl+0wfLzQJ3n5klvjihDHGxsKf/cyHxuTiUtxt7IK+R5lMahLQuSReHi9
74KEDJqfUQVkR29AR7Tnay1jM/qDl1zwM2MzZJFYN/3Fb6oTCKCL07T6Ai0Ct5E3
R+Top8rD8QNK7VWivF78Pxyqi9D6OARF/t0PjQWD6PippGzwVArNbdniZw9Fybgi
6XMa7BD+5XX9kz/Yr8YbyiEMuwiIgp7Qiy9YUfdad1rlnClp79AffNt+FcPWFUAX
HOO1SfpEJsKeFsIm2gZbDQIDAQABo1AwTjAdBgNVHQ4EFgQUDBBLDmUc6ELMjI+b
4MXf5EnsbGAwHwYDVR0jBBgwFoAUDBBLDmUc6ELMjI+b4MXf5EnsbGAwDAYDVR0T
BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAbTVTBURITrhDcXvAZTaYyknyH9He
hIixE4SFAtZ5i6NWhiA9veKfcSTtGVxzwbpEx5Rhqxx9OoYA6gD/BTtyX8GSFAbu
tQelpNZSBOnDCEMRCNUc1+ccULvJdXN0MGkjtNeCgv6S3gjyhFe+xRHB8nFOiq7Z
0qCFziwr2nK5sBoISMyERlQHwTaSbqm/AvvZioDkgTwcubfP9GIa6zkc6RxBJW+S
I7nKRgKm9r+E6Yi7Kahf1bCWCmFUVZKd+Y1zyWlYZA43v9gFcy8ZHOWg5+GAIhyO
UDqHH0wRogFg9n/9p69s/RcDdn6dW6Psdtvmxug28ExUQxYTkj/6ORmoiw==
-----END CERTIFICATE-----
`
	caPool.AppendCertsFromPEM([]byte(permCert))
	config := tls.Config{RootCAs: caPool}

	resty.SetTLSClientConfig(&config)
	resty.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))

	// Get the API key, if it doesn't exist, generate one.
	configMap, err := getConfigs(meqaPath)
	if err != nil {
		return err
	}
	if configMap[configAPIKey] == nil {
		return fmt.Errorf("api_key not found in %s", filepath.Join(meqaPath, configFile))
	}
	acceptedTerm := configMap[configAcceptedTerm]
	if acceptedTerm != nil {
		acceptedTermBool, ok := acceptedTerm.(bool)
		if !ok || !acceptedTermBool {
			acceptedTerm = nil
		}
	}
	warning := `The "generate" command will send your Swagger spec to
https://api.meqa.io to be processed. This service is provided as a convenience.
You can also follow https://github.com/meqaio/swagger_meqa/tree/master/docs
to do Swagger spec processing on your local computer. By continuing and using
the https://api.meqa.io service you are agreeing to our Terms and Conditions
located at: https://github.com/meqaio/swagger_meqa/blob/master/TERMS.md.

Do you wish to proceed? y/n: `
	if acceptedTerm == nil {
		fmt.Print(warning)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" {
			os.Exit(1)
		}
		configMap[configAcceptedTerm] = true
		err = writeConfigFile(filepath.Join(meqaPath, configFile), configMap)
		if err != nil {
			return err
		}
	}

	inputBytes, err := ioutil.ReadFile(swaggerPath)
	if err != nil {
		return err
	}
	var swaggerMap map[string]interface{}
	err = json.Unmarshal(inputBytes, &swaggerMap)
	if err == nil {
		// Convert to yaml because the server python requires it
		inputBytes, err = yaml.Marshal(swaggerMap)
		if err != nil {
			fmt.Printf("Unexpected error: %s\n", err)
			os.Exit(1)
		}
	} else {
		err = yaml.Unmarshal(inputBytes, &swaggerMap)
		if err != nil {
			fmt.Printf("Failed to unmarshal file %s as yaml - error: %s\n", swaggerPath, err.Error())
			os.Exit(1)
		}
	}
	if sv := swaggerMap["swagger"]; sv != "2.0" {
		fmt.Printf("We only support swagger/openapi spec 2.0 right now. Your version is %s\n", sv)
		os.Exit(1)
	}

	bodyMap := make(map[string]interface{})
	bodyMap["api_key"] = configMap[configAPIKey]
	bodyMap["swagger"] = string(inputBytes)

	req := resty.R()
	req.SetBody(bodyMap)
	resp, err := req.Post(serverURL + "/specs")

	if status := resp.StatusCode(); status >= 300 {
		return fmt.Errorf("server call failed, status %d, body:\n%s", status, string(resp.Body()))
	}

	respMap := make(map[string]interface{})
	err = json.Unmarshal(resp.Body(), &respMap)
	if err != nil {
		return err
	}

	if respMap["swagger_meqa"] == nil {
		return fmt.Errorf("server call failed, status %d, body:\n%s", resp.StatusCode(), string(resp.Body()))
	}

	// output file name is the input swagger spec name + _meqa.yml, if there isn't a _meqa already
	_, inputFile := filepath.Split(swaggerPath)
	swaggerMeqaPath := filepath.Join(meqaPath, strings.TrimSuffix(strings.Split(inputFile, ".")[0], "_meqa")+"_meqa.yml")
	fmt.Printf("Writing tagged swagger spec to: %s\n", swaggerMeqaPath)
	err = ioutil.WriteFile(swaggerMeqaPath, []byte(respMap["swagger_meqa"].(string)), 0644)
	if err != nil {
		return err
	}
	for planName, planBody := range respMap["test_plans"].(map[string]interface{}) {
		planPath := filepath.Join(meqaPath, planName+".yml")
		fmt.Printf("Writing test suites file to: %s\n", planPath)
		err = ioutil.WriteFile(planPath, []byte(planBody.(string)), 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	genCommand := flag.NewFlagSet("generate", flag.ExitOnError)
	genCommand.SetOutput(os.Stdout)
	runCommand := flag.NewFlagSet("run", flag.ExitOnError)
	runCommand.SetOutput(os.Stdout)

	genMeqaPath := genCommand.String("d", meqaDataDir, "the directory where meqa config, log and output files reside")
	genSwaggerFile := genCommand.String("s", "", "the OpenAPI (Swagger) spec file path")

	runMeqaPath := runCommand.String("d", meqaDataDir, "the directory where meqa config, log and output files reside")
	runSwaggerFile := runCommand.String("s", "", "the meqa generated OpenAPI (Swagger) spec file path")
	testPlanFile := runCommand.String("p", "", "the test plan file name")
	resultPath := runCommand.String("r", "", "the test result file name (default result.yml in meqa_data dir)")
	testToRun := runCommand.String("t", "all", "the test to run")
	username := runCommand.String("u", "", "the username for basic HTTP authentication")
	password := runCommand.String("w", "", "the password for basic HTTP authentication")
	apitoken := runCommand.String("a", "", "the api token for bearer HTTP authentication")
	verbose := runCommand.Bool("v", false, "turn on verbose mode")

	flag.Usage = func() {
		fmt.Println("Usage: mqgo {generate|run} [options]")
		fmt.Println("generate: generate test plans to be used by run command")
		genCommand.PrintDefaults()

		fmt.Println("\nrun: run the tests the in a test plan file")
		runCommand.PrintDefaults()
	}

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	var meqaPath *string
	var swaggerFile *string
	switch os.Args[1] {
	case "generate":
		genCommand.Parse(os.Args[2:])
		meqaPath = genMeqaPath
		swaggerFile = genSwaggerFile
	case "run":
		runCommand.Parse(os.Args[2:])
		meqaPath = runMeqaPath
		swaggerFile = runSwaggerFile
	default:
		flag.Usage()
		os.Exit(1)
	}
	if len(*swaggerFile) == 0 {
		fmt.Println("You must use -s option to provide a swagger/openapi yaml spec file. Use -h to see the options")
		os.Exit(1)
	}

	fi, err := os.Stat(*meqaPath)
	if os.IsNotExist(err) {
		fmt.Printf("Meqa directory %s doesn't exist.", *meqaPath)
		os.Exit(1)
	}
	if !fi.Mode().IsDir() {
		fmt.Printf("Meqa directory %s is not a directory.", *meqaPath)
		os.Exit(1)
	}

	if os.Args[1] == "run" {
		if len(*resultPath) == 0 {
			rf := filepath.Join(*meqaPath, resultFile)
			resultPath = &rf
		}
	}

	mqutil.Logger = mqutil.NewFileLogger(filepath.Join(*meqaPath, "mqgo.log"))
	mqutil.Logger.Println(os.Args)

	if _, err := os.Stat(*swaggerFile); os.IsNotExist(err) {
		fmt.Printf("can't load swagger file at the following location %s", *swaggerFile)
		os.Exit(1)
	}

	if genCommand.Parsed() {
		err = generateMeqa(*meqaPath, *swaggerFile)
		if err != nil {
			fmt.Printf("got an err:\n%s", err.Error())
			os.Exit(1)
		}
		return
	}

	runMeqa(meqaPath, swaggerFile, testPlanFile, resultPath, testToRun, username, password, apitoken, verbose)
}

func runMeqa(meqaPath *string, swaggerFile *string, testPlanFile *string, resultPath *string,
	testToRun *string, username *string, password *string, apitoken *string, verbose *bool) {

	mqutil.Verbose = *verbose

	if len(*testPlanFile) == 0 {
		fmt.Println("You must use -p to specify a test plan file. Use -h to see more options.")
		return
	}

	if _, err := os.Stat(*testPlanFile); os.IsNotExist(err) {
		fmt.Printf("can't load test plan file at the following location %s", *testPlanFile)
		return
	}

	// load swagger.yml
	swagger, err := mqswag.CreateSwaggerFromURL(*swaggerFile, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	mqswag.ObjDB.Init(swagger)

	// load test plan
	mqplan.Current.Username = *username
	mqplan.Current.Password = *password
	mqplan.Current.ApiToken = *apitoken
	err = mqplan.Current.InitFromFile(*testPlanFile, &mqswag.ObjDB)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}

	// for testing, set the config to skip verifying https certificates
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	resty.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))

	mqplan.Current.ResultCounts = make(map[string]int)
	if *testToRun == "all" {
		for _, testSuite := range mqplan.Current.SuiteList {
			mqutil.Logger.Printf("\n---\nTest suite: %s\n", testSuite.Name)
			fmt.Printf("\n---\nTest suite: %s\n", testSuite.Name)
			counts, err := mqplan.Current.Run(testSuite.Name, nil)
			mqutil.Logger.Printf("err:\n%v", err)
			for k := range counts {
				mqplan.Current.ResultCounts[k] += counts[k]
			}
		}
	} else {
		mqutil.Logger.Printf("\n---\nTest suite: %s\n", *testToRun)
		fmt.Printf("\n---\nTest suite: %s\n", *testToRun)
		counts, err := mqplan.Current.Run(*testToRun, nil)
		mqutil.Logger.Printf("err:\n%v", err)
		for k := range counts {
			mqplan.Current.ResultCounts[k] += counts[k]
		}
	}
	mqplan.Current.LogErrors()
	mqplan.Current.PrintSummary()
	os.Remove(*resultPath)
	mqplan.Current.WriteResultToFile(*resultPath)
}

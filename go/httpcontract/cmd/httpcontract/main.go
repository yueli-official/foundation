package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yueli-official/foundation/go/httpcontract"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("httpcontract", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectPath := flags.String("project", "", "generate a complete product contract from project config")
	extractOperationErrors := flags.String("extract-operation-errors", "", "extract product-owned error declarations from an operations manifest")
	extractedOutput := flags.String("operation-errors-output", "", "write extracted operation error declarations here")
	errorPath := flags.String("errors", "", "path to a public error catalog")
	operationsPath := flags.String("operations", "", "path to an HTTP operations manifest")
	baseErrorPath := flags.String("base-errors", "", "compare errors against this previous catalog")
	baseOperationsPath := flags.String("base-operations", "", "compare operations against this previous manifest")
	goOutput := flags.String("generate-go", "", "write generated Go catalog to this path")
	goPackage := flags.String("package", "", "package name for generated Go catalog")
	tsOutput := flags.String("generate-ts", "", "write generated TypeScript catalog to this path")
	tsType := flags.String("ts-type", "", "root type name for generated TypeScript")
	i18nOutput := flags.String("generate-i18n", "", "write generated i18n inventory to this path")
	check := flags.Bool("check", false, "verify generated outputs without rewriting them")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *extractOperationErrors != "" {
		if flags.NArg() != 0 || *extractedOutput == "" {
			return fmt.Errorf("-extract-operation-errors requires -operation-errors-output")
		}
		data, err := os.ReadFile(*extractOperationErrors)
		if err != nil {
			return err
		}
		operations, err := httpcontract.ParseOperations(data)
		if err != nil {
			return err
		}
		generated, err := httpcontract.EncodeOperationErrors(httpcontract.OperationErrorsFromOperations(operations))
		if err != nil {
			return err
		}
		return writeGenerated(*extractedOutput, generated, *check)
	}
	if *projectPath != "" {
		if flags.NArg() != 0 || *errorPath != "" || *operationsPath != "" {
			return fmt.Errorf("-project cannot be combined with individual contract inputs")
		}
		return runProject(*projectPath, *check)
	}
	if flags.NArg() != 0 || (*errorPath == "" && *operationsPath == "") {
		return fmt.Errorf("usage: httpcontract [-errors catalog.json] [-operations operations.json]")
	}

	var (
		catalog    httpcontract.ErrorCatalog
		operations httpcontract.Operations
		haveErrors bool
		haveOps    bool
	)
	if *errorPath != "" {
		data, err := os.ReadFile(*errorPath)
		if err != nil {
			return fmt.Errorf("read error catalog: %w", err)
		}
		catalog, err = httpcontract.ParseErrorCatalog(data)
		if err != nil {
			return err
		}
		haveErrors = true
	}
	if *operationsPath != "" {
		data, err := os.ReadFile(*operationsPath)
		if err != nil {
			return fmt.Errorf("read operations manifest: %w", err)
		}
		operations, err = httpcontract.ParseOperations(data)
		if err != nil {
			return err
		}
		haveOps = true
	}
	if haveErrors && haveOps {
		if err := httpcontract.VerifyReferences(catalog, operations); err != nil {
			return err
		}
	}
	report := httpcontract.CompatibilityReport{Changes: []httpcontract.CompatibilityChange{}}
	if *baseErrorPath != "" {
		if !haveErrors {
			return fmt.Errorf("-base-errors requires -errors")
		}
		data, err := os.ReadFile(*baseErrorPath)
		if err != nil {
			return fmt.Errorf("read base error catalog: %w", err)
		}
		base, err := httpcontract.ParseErrorCatalog(data)
		if err != nil {
			return err
		}
		report.Changes = append(report.Changes, httpcontract.DiffErrorCatalogs(base, catalog).Changes...)
	}
	if *baseOperationsPath != "" {
		if !haveOps {
			return fmt.Errorf("-base-operations requires -operations")
		}
		data, err := os.ReadFile(*baseOperationsPath)
		if err != nil {
			return fmt.Errorf("read base operations manifest: %w", err)
		}
		base, err := httpcontract.ParseOperations(data)
		if err != nil {
			return err
		}
		report.Changes = append(report.Changes, httpcontract.DiffOperations(base, operations).Changes...)
	}
	if *goOutput != "" {
		if !haveErrors || *goPackage == "" {
			return fmt.Errorf("-generate-go requires -errors and -package")
		}
		generated, err := httpcontract.GenerateGo(catalog, *goPackage)
		if err != nil {
			return err
		}
		if err := writeGenerated(*goOutput, generated, *check); err != nil {
			return err
		}
	}
	if *tsOutput != "" {
		if !haveErrors || *tsType == "" {
			return fmt.Errorf("-generate-ts requires -errors and -ts-type")
		}
		generated, err := httpcontract.GenerateTypeScript(catalog, *tsType)
		if err != nil {
			return err
		}
		if err := writeGenerated(*tsOutput, generated, *check); err != nil {
			return err
		}
	}
	if *i18nOutput != "" {
		if !haveErrors {
			return fmt.Errorf("-generate-i18n requires -errors")
		}
		generated, err := httpcontract.GenerateI18nInventory(catalog)
		if err != nil {
			return err
		}
		if err := writeGenerated(*i18nOutput, generated, *check); err != nil {
			return err
		}
	}
	if *baseErrorPath != "" || *baseOperationsPath != "" {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		if report.HasBreaking() {
			return fmt.Errorf("HTTP result contract contains breaking changes")
		}
	} else {
		fmt.Println("HTTP result contracts are valid")
	}
	return nil
}

func runProject(path string, check bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read project config: %w", err)
	}
	project, err := httpcontract.ParseProject(data)
	if err != nil {
		return err
	}
	openAPIPath := project.OpenAPI.Output
	producerOutput := openAPIPath
	if check {
		directory, err := os.MkdirTemp("", "httpcontract-project-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(directory)
		producerOutput = filepath.Join(directory, "openapi.json")
	}
	command := exec.Command(project.OpenAPI.Producer.Command[0], project.OpenAPI.Producer.Command[1:]...)
	command.Env = append(os.Environ(), project.OpenAPI.Producer.OutputEnv+"="+producerOutput)
	for name, value := range project.OpenAPI.Producer.Env {
		command.Env = append(command.Env, name+"="+value)
	}
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run OpenAPI producer: %w", err)
	}
	openAPI, err := os.ReadFile(producerOutput)
	if err != nil {
		return fmt.Errorf("read produced OpenAPI: %w", err)
	}
	if check {
		current, err := os.ReadFile(openAPIPath)
		if err != nil {
			return fmt.Errorf("read committed OpenAPI: %w", err)
		}
		if !httpcontract.EqualGenerated(current, openAPI) {
			return fmt.Errorf("generated output is stale: %s", openAPIPath)
		}
	}
	catalogData, err := os.ReadFile(project.ErrorCatalog)
	if err != nil {
		return fmt.Errorf("read project error catalog: %w", err)
	}
	catalog, err := httpcontract.ParseErrorCatalog(catalogData)
	if err != nil {
		return err
	}
	operationErrors := project.Operations.Errors
	var operationIDs map[string]string
	var operationOverrides map[string]httpcontract.OperationOverride
	if project.Operations.ErrorsFile != "" {
		errorData, err := os.ReadFile(project.Operations.ErrorsFile)
		if err != nil {
			return fmt.Errorf("read project operation errors: %w", err)
		}
		declarations, err := httpcontract.ParseOperationErrors(errorData)
		if err != nil {
			return err
		}
		if declarations.Namespace != project.Namespace {
			return fmt.Errorf("httpcontract: operation errors namespace %q does not match project %q", declarations.Namespace, project.Namespace)
		}
		operationErrors = declarations.Operations
		operationIDs = declarations.IDs
		operationOverrides = declarations.Overrides
	}
	operations, err := httpcontract.OperationsFromOpenAPIWithIDs(openAPI, project.Namespace, operationErrors, operationIDs, operationOverrides)
	if err != nil {
		return err
	}
	if err := httpcontract.VerifyProjectCatalogCoverage(catalog, operations, project.AllowUnusedErrors...); err != nil {
		return err
	}
	operationsData, err := httpcontract.EncodeOperations(operations)
	if err != nil {
		return err
	}
	if err := writeGenerated(project.Operations.Output, operationsData, check); err != nil {
		return err
	}
	goData, err := httpcontract.GenerateGo(catalog, project.Generate.GoPackage)
	if err != nil {
		return err
	}
	if err := writeGenerated(project.Generate.GoOutput, goData, check); err != nil {
		return err
	}
	tsData, err := httpcontract.GenerateTypeScript(catalog, project.Generate.TSType)
	if err != nil {
		return err
	}
	if err := writeGenerated(project.Generate.TSOutput, tsData, check); err != nil {
		return err
	}
	i18nData, err := httpcontract.GenerateI18nInventory(catalog)
	if err != nil {
		return err
	}
	if err := writeGenerated(project.Generate.I18nOutput, i18nData, check); err != nil {
		return err
	}
	if project.LegacyCatalog != nil {
		legacyData, err := httpcontract.GenerateLegacyCatalog(catalog, project.LegacyCatalog.SchemaVersion)
		if err != nil {
			return err
		}
		if err := writeGenerated(project.LegacyCatalog.Output, legacyData, check); err != nil {
			return err
		}
	}
	fmt.Printf("HTTP project contracts are current (%d operations)\n", len(operations.Operations))
	return nil
}

func writeGenerated(path string, data []byte, check bool) error {
	if check {
		current, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read generated output %s: %w", path, err)
		}
		normalize := func(value []byte) []byte { return bytes.ReplaceAll(value, []byte("\r\n"), []byte("\n")) }
		if !bytes.Equal(normalize(current), normalize(data)) {
			return fmt.Errorf("generated output is stale: %s", path)
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write generated output %s: %w", path, err)
	}
	return nil
}

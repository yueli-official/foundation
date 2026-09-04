package httpcontract

import (
	"fmt"
	"strings"
)

// VerifyReferences checks relationships between independently valid product
// manifests. Common errors are owned by Foundation and may be referenced
// without being redeclared in a product catalog.
func VerifyReferences(catalog ErrorCatalog, operations Operations) error {
	if catalog.Namespace != operations.Namespace {
		return fmt.Errorf("httpcontract: catalog namespace %q does not match operations namespace %q", catalog.Namespace, operations.Namespace)
	}
	declared := make(map[string]struct{}, len(catalog.Errors))
	for _, definition := range catalog.Errors {
		declared[definition.Code] = struct{}{}
	}
	for _, operation := range operations.Operations {
		if operation.FailureProtocol == "oauth" {
			continue
		}
		for _, code := range operation.Errors {
			if strings.HasPrefix(code, "common.") || strings.HasPrefix(code, "foundation.") {
				continue
			}
			if _, exists := declared[code]; !exists {
				return fmt.Errorf("httpcontract: operation %q references undeclared error %q", operation.ID, code)
			}
		}
	}
	return nil
}

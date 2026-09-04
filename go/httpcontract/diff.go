package httpcontract

import (
	"fmt"
	"reflect"
	"sort"
)

type ChangeSeverity string

const (
	ChangeBreaking   ChangeSeverity = "breaking"
	ChangeBehavioral ChangeSeverity = "behavioral"
	ChangeAdditive   ChangeSeverity = "additive"
)

type CompatibilityChange struct {
	Severity ChangeSeverity `json:"severity"`
	Path     string         `json:"path"`
	Message  string         `json:"message"`
}

type CompatibilityReport struct {
	Changes []CompatibilityChange `json:"changes"`
}

func (report CompatibilityReport) HasBreaking() bool {
	for _, change := range report.Changes {
		if change.Severity == ChangeBreaking {
			return true
		}
	}
	return false
}

func DiffErrorCatalogs(before, after ErrorCatalog) CompatibilityReport {
	result := CompatibilityReport{Changes: []CompatibilityChange{}}
	oldValues := map[string]ErrorDefinition{}
	newValues := map[string]ErrorDefinition{}
	for _, value := range before.Errors {
		oldValues[value.Code] = value
	}
	for _, value := range after.Errors {
		newValues[value.Code] = value
	}
	for code, oldValue := range oldValues {
		path := "/errors/" + code
		newValue, exists := newValues[code]
		if !exists {
			result.add(ChangeBreaking, path, "public error was removed")
			continue
		}
		if oldValue.Status != newValue.Status {
			result.add(ChangeBreaking, path+"/status", fmt.Sprintf("status changed from %d to %d", oldValue.Status, newValue.Status))
		}
		if oldValue.Violations != newValue.Violations {
			result.add(ChangeBreaking, path+"/violations", "violation policy changed")
		}
		for name, oldParam := range oldValue.Params {
			newParam, exists := newValue.Params[name]
			if !exists {
				result.add(ChangeBreaking, path+"/params/"+name, "parameter was removed")
				continue
			}
			if !reflect.DeepEqual(oldParam, newParam) {
				result.add(ChangeBreaking, path+"/params/"+name, "parameter contract changed")
			}
		}
		for name, newParam := range newValue.Params {
			if _, exists := oldValue.Params[name]; !exists {
				severity := ChangeAdditive
				if newParam.Required {
					severity = ChangeBreaking
				}
				result.add(severity, path+"/params/"+name, "parameter was added")
			}
		}
		if oldValue.MessageKey != newValue.MessageKey || oldValue.RecoveryKey != newValue.RecoveryKey {
			result.add(ChangeBehavioral, path+"/presentation", "message or recovery key changed")
		}
	}
	for code := range newValues {
		if _, exists := oldValues[code]; !exists {
			result.add(ChangeAdditive, "/errors/"+code, "public error was added")
		}
	}
	result.sort()
	return result
}

func DiffOperations(before, after Operations) CompatibilityReport {
	result := CompatibilityReport{Changes: []CompatibilityChange{}}
	oldValues := map[string]Operation{}
	newValues := map[string]Operation{}
	for _, value := range before.Operations {
		oldValues[value.ID] = value
	}
	for _, value := range after.Operations {
		newValues[value.ID] = value
	}
	for id, oldValue := range oldValues {
		path := "/operations/" + id
		newValue, exists := newValues[id]
		if !exists {
			result.add(ChangeBreaking, path, "operation was removed")
			continue
		}
		if oldValue.Method != newValue.Method {
			result.add(ChangeBreaking, path+"/method", "HTTP method changed")
		}
		if oldValue.Path != newValue.Path {
			result.add(ChangeBreaking, path+"/path", "path changed")
		}
		if !reflect.DeepEqual(oldValue.Success, newValue.Success) {
			result.add(ChangeBreaking, path+"/success", "success contract changed")
		}
		if !reflect.DeepEqual(sortedStrings(oldValue.Errors), sortedStrings(newValue.Errors)) {
			result.add(ChangeBehavioral, path+"/errors", "declared failure set changed")
		}
	}
	for id := range newValues {
		if _, exists := oldValues[id]; !exists {
			result.add(ChangeAdditive, "/operations/"+id, "operation was added")
		}
	}
	result.sort()
	return result
}

func (report *CompatibilityReport) add(severity ChangeSeverity, path, message string) {
	report.Changes = append(report.Changes, CompatibilityChange{Severity: severity, Path: path, Message: message})
}

func (report *CompatibilityReport) sort() {
	sort.Slice(report.Changes, func(i, j int) bool {
		if report.Changes[i].Path != report.Changes[j].Path {
			return report.Changes[i].Path < report.Changes[j].Path
		}
		return report.Changes[i].Severity < report.Changes[j].Severity
	})
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

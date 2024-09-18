package strategy

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReleaseStrategy nested struct to hold the parsed YAML strategy
type ReleaseStrategy struct {
	ID        string     `yaml:"id"`
	Name      string     `yaml:"name"`
	Type      string     `yaml:"type"`
	Functions []Function `yaml:"functions"`
	Stages    []Stage    `yaml:"stages"`
	Rollback  Rollback   `yaml:"rollback"`
}

type Function struct {
	Name        string  `yaml:"name"`
	BaseVersion Version `yaml:"base_version"`
	NewVersion  Version `yaml:"new_version"`
}

type Version struct {
	Path string `yaml:"path"`
	Env  string `yaml:"env"` // TODO: add entrypoint and associated main file
}

type Stage struct {
	Name              string            `yaml:"name"`
	Type              string            `yaml:"type"`
	FuncName          string            `yaml:"func_name"`
	Variants          []Variant         `yaml:"variants"`
	MetricsConditions []MetricCondition `yaml:"metrics_conditions"`
	EndConditions     []EndCondition    `yaml:"end_conditions"`
	EndAction         EndAction         `yaml:"end_action"`
}

type Variant struct {
	Name              string `yaml:"name"`
	TrafficPercentage int    `yaml:"trafficPercentage"`
}

type MetricCondition struct {
	Name        string `yaml:"name"`
	Threshold   string `yaml:"threshold"`
	CompareWith string `yaml:"compareWith,omitempty"`
}

type EndCondition struct {
	Name      string `yaml:"name"`
	Threshold string `yaml:"threshold"`
}

type EndAction struct {
	OnSuccess string `yaml:"onSuccess"`
	OnFailure string `yaml:"onFailure"`
}

type Rollback struct {
	Action RollbackAction `yaml:"action"`
}

type RollbackAction struct {
	Function string `yaml:"function"`
}

// LoadStrategy reads and parses the YAML strategy file
func LoadStrategy(filePath string) (*ReleaseStrategy, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading YAML file: %v", err)
	}

	var releaseStrategy ReleaseStrategy
	err = yaml.Unmarshal(data, &releaseStrategy)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling YAML data: %v", err)
	}

	if errV := releaseStrategy.validateTrafficPercentage(); errV != nil {
		return nil, errV
	}
	if errV2 := releaseStrategy.validateCompareWithValues(); errV2 != nil {
		return nil, errV2
	}
	if errV3 := releaseStrategy.validateRollbackFunction(); errV3 != nil {
		return nil, errV3
	}
	if errV4 := releaseStrategy.validateMetricConditions(); errV4 != nil {
		return nil, errV4
	}
	if errV5 := releaseStrategy.validateEndActions(); errV5 != nil {
		return nil, errV5
	}

	log.Infof("using release strategy '%v' (%v). It has following stages: %v", releaseStrategy.Name, releaseStrategy.Type, mapStageNames(releaseStrategy.Stages))
	log.Debugf("dump: %v", releaseStrategy)

	return &releaseStrategy, nil
}

func (rs *ReleaseStrategy) GetFunctionByName(name string) *Function {
	functions := rs.Functions
	for _, function := range functions {
		if function.Name == name {
			return &function
		}
	}
	return nil
}

func (f *Function) GetVersionByName(name string) (*Version, error) {
	switch name {
	case "BaseVersion", "base_version":
		return &f.BaseVersion, nil
	case "NewVersion", "new_version":
		return &f.NewVersion, nil
	default:
		return nil, fmt.Errorf("version '%s' not found in function '%s'", name, f.Name)
	}
}

func (mc *MetricCondition) IsThresholdMet(actual float64) bool {
	if len(mc.Threshold) < 2 {
		log.Fatalf("invalid threshold format: %s", mc.Threshold)
		//return false, fmt.Errorf("invalid threshold format: %s", mc.Threshold)
	}
	operator, thrVal, errP := parseComparisonString(mc.Threshold)
	if errP != nil {
		log.Fatalf("Error parsing comparison string: %v", errP)
		//return false, errP
	}

	switch operator {
	case "<":
		return actual < thrVal
	case "<=":
		return actual <= thrVal
	case ">":
		return actual > thrVal
	case ">=":
		return actual >= thrVal
	case "=":
		return actual == thrVal
	default:
		log.Fatalf("invalid threshold operator: %s", operator)
		//return false, fmt.Errorf("invalid threshold operator: %s", operator)
		return false // dummy
	}
}

// Private

func mapStageNames(stages []Stage) []string {
	names := make([]string, len(stages))
	for i, stage := range stages {
		names[i] = stage.Name
	}
	return names
}

// parseComparisonString parses a string like "<0.02" and returns the operator and value
func parseComparisonString(comp string) (string, float64, error) {
	var operator string
	if strings.HasPrefix(comp, "<=") {
		operator = "<="
	} else if strings.HasPrefix(comp, ">=") {
		operator = ">="
	} else if strings.HasPrefix(comp, "<") {
		operator = "<"
	} else if strings.HasPrefix(comp, ">") {
		operator = ">"
	} else if strings.HasPrefix(comp, "=") {
		operator = "="
	} else {
		return "", 0, fmt.Errorf("invalid comparison string: %s", comp)
	}

	valueStr := strings.TrimPrefix(comp, operator)
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid number: %s", valueStr)
	}

	return operator, value, nil
}

// --- Validations ---
func (rs *ReleaseStrategy) validateTrafficPercentage() error {
	for _, stage := range rs.Stages {
		totalTraffic := 0
		for _, variant := range stage.Variants {
			totalTraffic += variant.TrafficPercentage
		}
		if totalTraffic != 100 {
			return fmt.Errorf("total traffic percentage for stage %s is %d, expected 100", stage.Name, totalTraffic)
		}
	}
	return nil
}
func (rs *ReleaseStrategy) validateCompareWithValues() error {
	allowedValues := map[string]bool{
		"Minimum": true,
		"Maximum": true,
		"Median":  true,
	}

	for _, stage := range rs.Stages {
		for _, metricCondition := range stage.MetricsConditions {
			if metricCondition.CompareWith != "" && !allowedValues[metricCondition.CompareWith] {
				return fmt.Errorf("invalid CompareWith value '%s' in stage '%s', allowed values are 'Minimum', 'Maximum', 'Median'", metricCondition.CompareWith, stage.Name)
			}
		}
	}
	return nil
}
func (rs *ReleaseStrategy) validateRollbackFunction() error {
	rollbackFunction := rs.Rollback.Action.Function
	for _, function := range rs.Functions {
		if _, err := function.GetVersionByName(rollbackFunction); err != nil {
			return fmt.Errorf("rollback function '%s' is not defined for '%v' function in the functions list", rollbackFunction, function.Name)
		}
	}
	return nil
}
func (rs *ReleaseStrategy) validateMetricConditions() error {
	for _, stage := range rs.Stages {
		for _, metricCondition := range stage.MetricsConditions {
			if _, _, err := parseComparisonString(metricCondition.Threshold); err != nil {
				return fmt.Errorf("invalid threshold format '%s' in metric condition '%s' of stage '%s': %v", metricCondition.Threshold, metricCondition.Name, stage.Name, err)
			}
		}
	}
	return nil
}
func (rs *ReleaseStrategy) validateEndActions() error {
	validEndActions := map[string]bool{
		"rollout":  true,
		"rollback": true,
	}

	// Collect all valid stage names
	for _, stage := range rs.Stages {
		validEndActions[stage.Name] = true
	}

	for _, stage := range rs.Stages {
		if stage.EndAction.OnSuccess == "" || stage.EndAction.OnFailure == "" {
			return fmt.Errorf("end_action for stage '%s' must have both onSuccess and onFailure keys", stage.Name)
		}
		if !validEndActions[stage.EndAction.OnSuccess] {
			return fmt.Errorf("invalid onSuccess value '%s' in end_action for stage '%s'", stage.EndAction.OnSuccess, stage.Name)
		}
		if !validEndActions[stage.EndAction.OnFailure] {
			return fmt.Errorf("invalid onFailure value '%s' in end_action for stage '%s'", stage.EndAction.OnFailure, stage.Name)
		}
		if stage.EndAction.OnSuccess == stage.Name || stage.EndAction.OnFailure == stage.Name {
			return fmt.Errorf("end_action for stage '%s' cannot have onSuccess or onFailure value same as the stage name (loop)", stage.Name)
		}
	}
	return nil
}

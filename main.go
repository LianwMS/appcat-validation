package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Incident struct {
	Uri        string      `yaml:"uri"`
	Message    string      `yaml:"message"`
	CodeSnip   string      `yaml:"codeSnip"`
	Variables  interface{} `yaml:"variables"`
	LineNumber interface{} `yaml:"lineNumber"`
}

type Violation struct {
	Incidents []Incident `yaml:"incidents"`
}

type RuleSet struct {
	Name       string               `yaml:"name"`
	Violations map[string]Violation `yaml:"violations"`
}

func containsAction(slice []ActionType, item ActionType) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// define enum for action types
type ActionType string

const (
	ActionRun      ActionType = "run"
	ActionAnalyze  ActionType = "analyze"
	ActionValidate ActionType = "validate"
)

func main() {
	// Mock input parameters for testing purposes
	appcatApplicationFolder := ``
	testDataFolder := ``
	testOutputFolder := ``
	targetProject := ""

	var actionList []ActionType = []ActionType{ActionAnalyze}
	var targetList []string
	var logFileName string
	var summaryFileName string
	var timeInFileName = time.Now().Format("20060102_150405")

	var globalIncidents = 0
	var globalRulesDetails = make(map[string]int)

	// Scan for target project(s)
	if targetProject != "" {
		projectPath := filepath.Join(testDataFolder, targetProject)
		info, err := os.Stat(projectPath)
		if err != nil || !info.IsDir() {
			log.Fatalf("The specified target project '%s' does not exist in the test data folder.", targetProject)
		}
		targetList = append(targetList, targetProject)
		logFileName = fmt.Sprintf("appcat_test_%s_%s.log", targetProject, timeInFileName)
		summaryFileName = fmt.Sprintf("appcat_test_%s_%s_summary.csv", targetProject, timeInFileName)
	} else {
		entries, err := os.ReadDir(testDataFolder)
		if err != nil {
			log.Fatalf("Failed to read test data folder: %v", err)
		}
		for _, entry := range entries {
			if entry.IsDir() && len(entry.Name()) > 0 && entry.Name()[0] != '.' {
				targetList = append(targetList, entry.Name())
			}
		}

		logFileName = fmt.Sprintf("appcat_test_%s.log", timeInFileName)
		summaryFileName = fmt.Sprintf("appcat_test_summary_%s.csv", timeInFileName)
	}

	// Define log file path
	logFilePath := filepath.Join(testOutputFolder, logFileName)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		log.Fatalf("Failed to create log file: %v", err)
	}
	defer logFile.Close()
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logger := log.New(multiWriter, "", log.LstdFlags)

	// Verify input folders
	if _, err := os.Stat(appcatApplicationFolder); os.IsNotExist(err) {
		logger.Fatalf("The application folder path '%s' does not exist.", appcatApplicationFolder)
	}
	if _, err := os.Stat(testDataFolder); os.IsNotExist(err) {
		logger.Fatalf("The test data folder path '%s' does not exist.", testDataFolder)
	}
	if _, err := os.Stat(testOutputFolder); os.IsNotExist(err) {
		err := os.MkdirAll(testOutputFolder, 0755)
		if err != nil {
			logger.Fatalf("Failed to create output folder: %v", err)
		}
	}

	// Log the start of the test
	logger.Printf("Starting AppCat test at %s", time.Now().Format(time.RFC3339))
	logger.Printf("Input parameters:")
	logger.Printf("  Application folder: %s", appcatApplicationFolder)
	logger.Printf("  Test data folder: %s", testDataFolder)
	logger.Printf("  Test output folder: %s", testOutputFolder)
	logger.Printf("  Target project(s): %s", targetProject)
	logger.Printf("Total target projects found: %d", len(targetList))

	for _, target := range targetList {
		logger.Println("------------------------------------------------------------")
		logger.Printf("Processing project: %s", target)
		projectPath := filepath.Join(testDataFolder, target)
		projectOutput := filepath.Join(testOutputFolder, target)
		projectAppcatOutput := filepath.Join(projectOutput, "appcat_output")
		analyzeOutput := filepath.Join(projectOutput, "analyze_output")

		if containsAction(actionList, ActionRun) {
			logger.Printf("[1] Would run AppCat analysis for project: %s (output: %s)", target, projectAppcatOutput)
			// Call RunAppCat function
			if err := RunAppCat(appcatApplicationFolder, projectPath, projectAppcatOutput, logger); err != nil {
				logger.Fatalf("[1] Error running AppCat for project %s: %v", target, err)
			} else {
				logger.Printf("[1] Successfully ran AppCat for project: %s", target)
			}
		}

		if containsAction(actionList, ActionAnalyze) {
			logger.Printf("[2] Would run output analysis for project: %s (output: %s)", target, analyzeOutput)
			// Call AnalyzeOutput function
			incidentsCount, rulesDetails, err := AnalyzeOutput(projectAppcatOutput, analyzeOutput, logger)
			if err != nil {
				logger.Fatalf("[2] Error analyzing output for project %s: %v", target, err)
			} else {
				logger.Printf("[2] Successfully analyzed output for project: %s", target)
				globalIncidents += incidentsCount
				for rule, count := range rulesDetails {
					globalRulesDetails[rule] += count
				}
			}
		}

		logger.Printf("Completed processing for project: %s", target)
	}

	logger.Printf("Ending AppCat test at %s", time.Now().Format(time.RFC3339))
	logger.Printf("Total incidents found across all projects: %d", globalIncidents)
	logger.Printf("Rules details across all projects:")
	for rule, count := range globalRulesDetails {
		logger.Printf("  %s: %d", rule, count)
	}

	summaryFilePath := filepath.Join(testOutputFolder, summaryFileName)
	summaryFile, err := os.Create(summaryFilePath)
	if err != nil {
		logger.Fatalf("Failed to create summary file: %v", err)
	}
	defer summaryFile.Close()
	summaryFile.WriteString("Rule,Incidents\n")
	for rule, count := range globalRulesDetails {
		summaryFile.WriteString(fmt.Sprintf("%s,%d\n", rule, count))
	}
	logger.Printf("[Analyze] Global summary written to: %s\n", summaryFilePath)
}

func RunAppCat(appcatApplicationFolder, candidateProjectFolder, appcatOutputFolder string, logger *log.Logger) error {
	// Validate input parameters
	if _, err := os.Stat(appcatApplicationFolder); os.IsNotExist(err) {
		logger.Fatalf("[AppCat] The application folder path '%s' does not exist", appcatApplicationFolder)
	}
	if _, err := os.Stat(candidateProjectFolder); os.IsNotExist(err) {
		logger.Fatalf("[AppCat] The candidate project folder path '%s' does not exist", candidateProjectFolder)
	}
	if _, err := os.Stat(appcatOutputFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(appcatOutputFolder, 0755); err != nil {
			logger.Fatalf("[AppCat] Failed to create output folder: %v", err)
		}
	}

	logger.Printf("[AppCat] Project: %s\n", candidateProjectFolder)
	logger.Printf("[AppCat] Output: %s\n", appcatOutputFolder)
	logger.Printf("[AppCat] Start run AppCat at %s\n", time.Now())

	// Prepare command
	appcatExe := filepath.Join(appcatApplicationFolder, "appcat.exe")
	cmd := exec.Command(
		appcatExe, "analyze",
		"--input", candidateProjectFolder,
		"--output", appcatOutputFolder,
		"--target", "cloud-readiness,linux,azure-appservice,azure-aks,azure-container-apps,openjdk11,openjdk17,openjdk21",
		"--overwrite",
	)
	cmd.Dir = appcatApplicationFolder
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run command
	if err := cmd.Run(); err != nil {
		logger.Fatalf("[AppCat] Error: Failed to process %s: %v", candidateProjectFolder, err)
	}

	logger.Printf("[AppCat] AppCat completed at %s\n", time.Now())
	return nil
}

func AnalyzeOutput(appcatOutputFolder, analyzeOutputFolder string, logger *log.Logger) (int, map[string]int, error) {
	logger.Printf("[Analyze] AppCat output: %s\n", appcatOutputFolder)
	logger.Printf("[Analyze] Analyze output: %s\n", analyzeOutputFolder)

	// Ensure analyze output folder exists
	if _, err := os.Stat(analyzeOutputFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(analyzeOutputFolder, 0755); err != nil {
			logger.Fatalf("failed to create analyze output folder: %v", err)
		}
	}

	outputFile := filepath.Join(appcatOutputFolder, "output.yaml")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		logger.Fatalf("No output.yaml found in folder: %s\n", appcatOutputFolder)
		return 0, nil, nil
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		logger.Fatalf("failed to read output.yaml: %v", err)
	}

	var yamlContent []RuleSet
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		logger.Fatalf("failed to parse YAML: %v", err)
	}

	totalIncidents := 0
	rulesDetails := make(map[string]int)

	for _, section := range yamlContent {
		rulesetName := section.Name
		logger.Printf("[Analyze] Processing ruleset: %s\n", rulesetName)
		if section.Violations != nil {
			for ruleName, violation := range section.Violations {
				logger.Printf("  [Analyze] Processing rule: %s\n", ruleName)
				if len(violation.Incidents) > 0 {
					for i, incident := range violation.Incidents {
						totalIncidents++
						rulesDetails[ruleName]++
						logger.Printf("    [Analyze] Processing incidents: %v %v\n", incident.Uri, incident.LineNumber)
						incidentFileName := fmt.Sprintf("%s_%d", ruleName, i)
						incidentFilePath := filepath.Join(analyzeOutputFolder, incidentFileName)

						var variablesStr string
						if incident.Variables != nil {
							variablesMap, ok := incident.Variables.(map[string]interface{})
							if ok {
								for key, value := range variablesMap {
									variablesStr += "\n  " + fmt.Sprintf("%s: %v", key, value)
								}
							} else {
								variablesStr = fmt.Sprintf("%v", incident.Variables)
							}
						} else {
							variablesStr = "nil"
						}

						incidentDetails := fmt.Sprintf(
							"ruleSet: %s\nrule: %s\nuri: %v\nmessage: %v\ncodeSnip: %v\nvariables: %v\nlineNumber: %v\n",
							rulesetName, ruleName, incident.Uri, incident.Message, incident.CodeSnip, variablesStr, incident.LineNumber,
						)
						if err := os.WriteFile(incidentFilePath, []byte(incidentDetails), 0644); err != nil {
							logger.Fatalf("Failed to write incident file: %v", err)
						}
					}
				} else {
					logger.Printf("  [Analyze] No incidents found for rule: %s\n", ruleName)
				}
			}
		} else {
			logger.Printf("[Analyze] No violations found for rule '%s'.\n", rulesetName)
		}
	}

	logger.Printf("[Analyze] Total # of incidents in folder '%s': %d\n", appcatOutputFolder, totalIncidents)
	logger.Printf("[Analyze] Rules details in folder '%s':\n", appcatOutputFolder)
	for rule, count := range rulesDetails {
		logger.Printf("  %s: %d\n", rule, count)
	}

	// write summary to analyze output folder
	summaryFilePath := filepath.Join(analyzeOutputFolder, "summary.csv")
	summaryFile, err := os.Create(summaryFilePath)
	if err != nil {
		logger.Fatalf("Failed to create summary file: %v", err)
	}
	defer summaryFile.Close()
	summaryFile.WriteString("Rule,Incidents\n")
	for rule, count := range rulesDetails {
		summaryFile.WriteString(fmt.Sprintf("%s,%d\n", rule, count))
	}
	logger.Printf("[Analyze] Summary written to: %s\n", summaryFilePath)

	return totalIncidents, rulesDetails, nil
}

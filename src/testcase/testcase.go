package testcase

import (
	"context"
	"encoding/json"
	"fmt"
	"lianwMS/appcat_validation/logger"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"gopkg.in/yaml.v3"
)

const (
	lineDelimiter = "\n"
)

type Signs struct {
	PASS string
	FAIL string
}

var signs = Signs{
	PASS: "- [x]",
	FAIL: "- [ ] :x:",
}

type ItemResultFormatStruct struct {
	PASS    string
	FAIL    string
	DETAILS string
	SUBITEM string
}

var ItemResultFormat = ItemResultFormatStruct{
	PASS:    signs.PASS + " <b>%s</b>.",
	FAIL:    signs.FAIL + " <b>%s</b>. \n\n%s\n",
	DETAILS: "  <details>\n  <summary> Details </summary>\n\n  %s\n\n</details>",
	SUBITEM: "  %s %s",
}

type ActionType string

const (
	ActionRun      ActionType = "run"
	ActionAnalyze  ActionType = "analyze"
	ActionValidate ActionType = "validate"
)

func containsAction(slice []ActionType, item ActionType) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

const (
	IncidentExtension string = ".incident"
	CSVExtension      string = ".csv"
	YamlExtension     string = ".yaml"
	LogExtension      string = ".log"
)

type Incident struct {
	Uri        string      `yaml:"uri"`
	Message    string      `yaml:"message"`
	CodeSnip   string      `yaml:"codeSnip"`
	Variables  interface{} `yaml:"variables"`
	LineNumber int         `yaml:"lineNumber"`
}

type Violation struct {
	Incidents []Incident `yaml:"incidents"`
}

type RuleSet struct {
	Name       string               `yaml:"name"`
	Violations map[string]Violation `yaml:"violations"`
}

type ValidateIncident struct {
	RuleSet    string      `yaml:"ruleSet"`
	Rule       string      `yaml:"rule"`
	Uri        string      `yaml:"uri"`
	Message    string      `yaml:"message"`
	CodeSnip   string      `yaml:"codeSnip"`
	Variables  interface{} `yaml:"variables"`
	LineNumber int         `yaml:"lineNumber"`
}

type TestCase struct {
	Name              string
	ApplicationFolder string
	ProjectFolder     string
	OutputFolder      string
	BaseLineFolder    string
	ActionList        []ActionType
}

func (tc *TestCase) GetInfo() string {
	return fmt.Sprintf("TestCase(Name: %s, ApplicationFolder: %s, ProjectFolder: %s, OutputFolder: %s)",
		tc.Name, tc.ApplicationFolder, tc.ProjectFolder, tc.OutputFolder)
}

func (tc *TestCase) getAppcatOutputFolder() string {
	return filepath.Join(tc.OutputFolder, "appcat_output")
}

func (tc *TestCase) getAnalysisOutputFolder() string {
	return filepath.Join(tc.OutputFolder, "analysis_output")
}

func (tc *TestCase) getIncidentsSummaryFile() string {
	return filepath.Join(tc.getAnalysisOutputFolder(), fmt.Sprintf("%s%s", "incidents_summary", CSVExtension))
}

func (tc *TestCase) Run() (string, error) {
	logger := logger.Get()

	if containsAction(tc.ActionList, ActionRun) {
		if _, err := tc.RunAppCat(); err != nil {
			logger.Printf("[AppCat] Error running AppCat for project %s: %v", tc.Name, err)
			return "", fmt.Errorf("error running AppCat for project %s: %w", tc.Name, err)
		}
	}

	if containsAction(tc.ActionList, ActionAnalyze) {
		if _, _, err := tc.RunAnalyze(); err != nil {
			logger.Printf("[Analyze] Error analyzing output for project %s: %v", tc.Name, err)
			return "", fmt.Errorf("error analyzing output for project %s: %w", tc.Name, err)
		}
	}

	if containsAction(tc.ActionList, ActionValidate) {
		_, caseResults, err := tc.RunValidate()
		if err != nil {
			logger.Printf("[Validate] Error validating output for project %s: %v", tc.Name, err)
			return "", fmt.Errorf("error validating output for project %s: %w", tc.Name, err)
		}
		if len(caseResults) == 0 {
			resultMessage := fmt.Sprintf(ItemResultFormat.PASS, tc.Name)
			return resultMessage, nil
		} else {
			details := ""
			for _, value := range caseResults {
				details += value + lineDelimiter
			}
			resultMessage := fmt.Sprintf(ItemResultFormat.FAIL, tc.Name, fmt.Sprintf(ItemResultFormat.DETAILS, details))
			return resultMessage, nil
		}
	}

	return "passed", nil
}

func (tc *TestCase) RunAppCat() (string, error) {
	logger := logger.Get()
	logger.Printf("[AppCat] Would run AppCat analysis for project: %s (%s)", tc.Name, tc.ProjectFolder)

	if _, err := os.Stat(tc.ProjectFolder); os.IsNotExist(err) {
		logger.Fatalf("[AppCat] The candidate project folder path '%s' does not exist", tc.ProjectFolder)
		return "", fmt.Errorf("[AppCat] The candidate project folder path '%s' does not exist", tc.ProjectFolder)
	}
	if _, err := os.Stat(tc.getAppcatOutputFolder()); os.IsNotExist(err) {
		if err := os.MkdirAll(tc.getAppcatOutputFolder(), 0755); err != nil {
			logger.Fatalf("[AppCat] Failed to create output folder: %v", err)
			return "", fmt.Errorf("[AppCat] Failed to create output folder: %w", err)
		}
	}

	logger.Printf("[AppCat] Project: %s\n", tc.ProjectFolder)
	logger.Printf("[AppCat] Output: %s\n", tc.getAppcatOutputFolder())
	logger.Printf("[AppCat] Start run AppCat at %s\n", time.Now())

	// Prepare command
	appcatExe := filepath.Join(tc.ApplicationFolder, "appcat.exe")
	cmd := exec.Command(
		appcatExe, "analyze",
		"--input", tc.ProjectFolder,
		"--output", tc.getAppcatOutputFolder(),
		"--target", "cloud-readiness,linux,azure-appservice,azure-aks,azure-container-apps,openjdk11,openjdk17,openjdk21",
		"--overwrite",
	)
	cmd.Dir = tc.ApplicationFolder
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run command
	if err := cmd.Run(); err != nil {
		logger.Fatalf("[AppCat] Error: Failed to process %s: %v", tc.ProjectFolder, err)
		return "", fmt.Errorf("[AppCat] Error: Failed to process %s: %w", tc.ProjectFolder, err)
	}

	logger.Printf("[AppCat] AppCat completed at %s\n", time.Now())

	return tc.OutputFolder, nil
}

func (tc *TestCase) RunAnalyze() (int, map[string]int, error) {
	logger := logger.Get()
	logger.Printf("[Analyze] Would run output analysis for project: %s (output: %s)", tc.Name, tc.getAnalysisOutputFolder())
	logger.Printf("[Analyze] AppCat output: %s\n", tc.getAppcatOutputFolder())
	logger.Printf("[Analyze] Analyze output: %s\n", tc.getAnalysisOutputFolder())

	// Ensure analyze output folder exists
	if _, err := os.Stat(tc.getAnalysisOutputFolder()); os.IsNotExist(err) {
		if err := os.MkdirAll(tc.getAnalysisOutputFolder(), 0755); err != nil {
			logger.Fatalf("failed to create analyze output folder: %v", err)
		}
	}

	outputFile := filepath.Join(tc.getAppcatOutputFolder(), "output.yaml")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		logger.Fatalf("No output.yaml found in folder: %s\n", tc.getAppcatOutputFolder())
		return 0, nil, nil
	}

	_, rulesDetails, totalIncidents, err := tc.ParseAppCatOutput(tc.getAppcatOutputFolder(), tc.getAnalysisOutputFolder())
	if err != nil {
		logger.Fatalf("[Analyze] Error parsing AppCat output: %v", err)
		return 0, nil, fmt.Errorf("[Analyze] Error parsing AppCat output: %w", err)
	}

	logger.Printf("[Analyze] Total # of incidents found in %s: %d\n", tc.Name, totalIncidents)
	logger.Printf("[Analyze] Rules details for %s:\n", tc.Name)
	for rule, count := range rulesDetails {
		logger.Printf("  %s: %d\n", rule, count)
	}

	// write summary to analyze output folder

	summaryFile, err := os.Create(tc.getIncidentsSummaryFile())
	if err != nil {
		logger.Fatalf("Failed to create summary file: %v", err)
	}
	defer summaryFile.Close()
	summaryFile.WriteString("Rule,Incidents\n")
	for rule, count := range rulesDetails {
		summaryFile.WriteString(fmt.Sprintf("%s,%d\n", rule, count))
	}
	logger.Printf("[Analyze] Summary written to: %s\n", tc.getIncidentsSummaryFile())

	return totalIncidents, rulesDetails, nil
	// Call AnalyzeOutput function

	// globalIncidents += incidentsCount
	// for rule, count := range rulesDetails {
	// 	// globalRulesDetails[rule][target] = count
	// 	logger.Printf("[2] Rule: %s, Target, %s, Count: %d", rule, tc.Name, count)
	// 	if _, exists := globalRulesDetails[rule]; !exists {
	// 		globalRulesDetails[rule] = make(map[string]int)
	// 	}
	// 	globalRulesDetails[rule][target] = count
	// }
}

func (tc *TestCase) ParseAppCatOutput(outputPath string, presistPath string) (map[string]ValidateIncident, map[string]int, int, error) {
	logger := logger.Get()
	logger.Printf("[ParseOutput] Parsing output from: %s\n", outputPath)

	outputFile := filepath.Join(outputPath, "output.yaml")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		logger.Fatalf("No output.yaml found in folder: %s\n", outputPath)
		return nil, nil, 0, fmt.Errorf("no output.yaml found in folder: %s", outputPath)
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		logger.Fatalf("failed to read output.yaml: %v", err)
	}

	var yamlContent []RuleSet
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		logger.Fatalf("failed to parse YAML: %v", err)
	}

	incidentsCount := 0
	ruleIncidentDetails := make(map[string]int)
	incidentsDetails := make(map[string]ValidateIncident)

	for _, section := range yamlContent {
		rulesetName := section.Name
		logger.Printf("[ParseOutput] Processing ruleset: %s\n", rulesetName)
		if section.Violations != nil {
			for ruleName, violation := range section.Violations {
				logger.Printf("  [ParseOutput] Processing rule: %s\n", ruleName)
				if len(violation.Incidents) > 0 {
					for i, incident := range violation.Incidents {
						incidentsCount++
						ruleIncidentDetails[ruleName]++
						logger.Printf("    [ParseOutput] Processing incidents: %v %v\n", incident.Uri, incident.LineNumber)
						vIncident := ValidateIncident{
							RuleSet:    rulesetName,
							Rule:       ruleName,
							Uri:        incident.Uri,
							CodeSnip:   incident.CodeSnip,
							Message:    incident.Message,
							LineNumber: incident.LineNumber,
							Variables:  incident.Variables,
						}

						// substring vincident.Uri from the first occurrence of tc.Name and include the tc.Name.
						tempPath := vIncident.Uri
						startIndex := strings.Index(vIncident.Uri, tc.Name)
						if startIndex != -1 {
							tempPath = vIncident.Uri[startIndex:]
						}
						key := fmt.Sprintf("%s-%s-%s-%d", vIncident.RuleSet, vIncident.Rule, tempPath, vIncident.LineNumber)
						logger.Printf("    [ParseOutput] Incident key: %s\n", key)
						if _, exists := incidentsDetails[key]; !exists {
							incidentsDetails[key] = vIncident
						} else {
							logger.Printf("[ParseOutput] Duplicate incident found in baseline: %s", key)
						}

						if presistPath != "" {
							incidentFileName := fmt.Sprintf("%s_%d%s", ruleName, i, IncidentExtension)
							incidentFilePath := filepath.Join(presistPath, incidentFileName)

							incidentDetails, _ := yaml.Marshal(vIncident)
							if err := os.WriteFile(incidentFilePath, []byte(incidentDetails), 0644); err != nil {
								logger.Fatalf("Failed to write incident file: %v", err)
							}
						}
					}
				} else {
					logger.Printf("  [ParseOutput] No incidents found for rule: %s\n", ruleName)
				}
			}
		} else {
			logger.Printf("[ParseOutput] No violations found for rule '%s'.\n", rulesetName)
		}
	}
	return incidentsDetails, ruleIncidentDetails, incidentsCount, nil
}

func (tc *TestCase) RunValidate() (bool, map[string]string, error) {
	logger := logger.Get()
	logger.Printf("[Validate] Would validate output for project: %s (output: %s)", tc.Name, tc.getAppcatOutputFolder())
	logger.Printf("[Validate] Analyze output: %s\n", tc.getAppcatOutputFolder())
	logger.Printf("[Validate] baseLineFolder: %s\n", tc.BaseLineFolder)

	baselineIncidents, _, _, err := tc.ParseAppCatOutput(tc.BaseLineFolder, "")
	if err != nil {
		logger.Fatalf("[Validate] Error parsing baseline output: %v", err)
		return false, nil, fmt.Errorf("[Validate] Error parsing baseline output: %w", err)
	}
	logger.Printf("[Validate] Read %d baseline incidents from folder: %s\n", len(baselineIncidents), tc.BaseLineFolder)

	incidents, _, _, err := tc.ParseAppCatOutput(tc.getAppcatOutputFolder(), "")
	if err != nil {
		logger.Fatalf("[Validate] Error parsing analyze output: %v", err)
		return false, nil, fmt.Errorf("[Validate] Error parsing analyze output: %w", err)
	}
	logger.Printf("[Validate] Read %d incidents from analyze output folder: %s\n", len(incidents), tc.getAnalysisOutputFolder())

	result := true
	resultDetails := make(map[string]string)
	// Validate each incident against the baseline
	for key, incident := range incidents {
		baselineIncident, exists := baselineIncidents[key]
		if !exists {
			logger.Printf("[Validate] Incident %s not found in baseline, marking as false", key)
			result = false
			resultDetails[key] = fmt.Sprintf("[NEW] : %s", key)
			continue
		}
		if incident.Message != baselineIncident.Message {
			logger.Printf("[Validate] Incident %s message mismatch: %s != %s", key, incident.Message, baselineIncident.Message)
			result = false
			resultDetails[key] = fmt.Sprintf("[WRONG] :%s message mismatch: %s != %s", key, incident.Message, baselineIncident.Message)
			continue
		}

		logger.Printf("[Validate] Incident %s validated successfully", key)
	}

	for key := range baselineIncidents {
		if _, exists := incidents[key]; !exists {
			logger.Printf("[Validate] Baseline incident %s not found in analyze output, marking as false", key)
			result = false
			resultDetails[key] = fmt.Sprintf("[MISS]: %s", key)
			continue
		}
	}

	logger.Printf("[Validate] Validation completed for project: %s", tc.Name)
	return result, resultDetails, nil
}

// Following code will not be use now, but can be used later to validate the output
func ValidateOutputAI(analyzeOutputFolder string, logger *log.Logger) error {
	logger.Printf("[Validate] Validating output in folder: %s\n", analyzeOutputFolder)
	azureOpenAIEndpoint := "https://openai-acl4o26y5lrhk.openai.azure.com/"
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		logger.Fatalf("ERROR: %s", err)
	}
	client, err := azopenai.NewClient(azureOpenAIEndpoint, cred, nil)
	if err != nil {
		logger.Fatalf("ERROR: %s", err)
	}

	// For range in analyzeOutputFolder, check for files with .prompt extension
	files, err := os.ReadDir(analyzeOutputFolder)
	if err != nil {
		logger.Fatalf("Failed to read analyze output folder: %v", err)
	}
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == IncidentExtension {
			filePath := filepath.Join(analyzeOutputFolder, file.Name())
			promptData, err := os.ReadFile(filePath)
			if err != nil {
				logger.Fatalf("failed to read %s: %v", filePath, err)
			}
			userPrompt := fmt.Sprintf("[Tool Result]\n%s[/Tool Result]", promptData)
			logger.Printf("[Validate] Validating file: %s\n", filePath)
			// Call ValidateSingleIncident function

			result := ValidateSingleIncidentAI(userPrompt, client, logger)

			// Save the result to a file
			resultFileName := fmt.Sprintf("%s%s", file.Name(), YamlExtension)
			resultFilePath := filepath.Join(analyzeOutputFolder, resultFileName)
			resultData, err := yaml.Marshal(result)
			if err != nil {
				logger.Fatalf("Failed to marshal validation result: %v", err)
			}
			if err := os.WriteFile(resultFilePath, resultData, 0644); err != nil {
				logger.Fatalf("Failed to write validation result file: %v", err)
			}

		}
	}
	return nil
}

func ValidateSingleIncidentAI(userPrompt_incident string, client *azopenai.Client, logger *log.Logger) map[string]interface{} {
	result := map[string]interface{}{} // Initialize result map

	// logger.Printf("[Validate] Validating incident: %s\n", userPrompt_incident)

	// Call Azure OpenAI API to validate the incident, using managed identity
	deploymentName := "llm-gpt-4o"

	// Define the parameters
	maxTokens := int32(800)
	temperature := float32(0.7)
	topP := float32(0.95)
	frequencyPenalty := float32(0)
	presencePenalty := float32(0)
	var stop []string

	messages := []azopenai.ChatRequestMessageClassification{
		&azopenai.ChatRequestSystemMessage{Content: azopenai.NewChatRequestSystemMessageContent(SystemPrompt)},
		&azopenai.ChatRequestUserMessage{Content: azopenai.NewChatRequestUserMessageContent(userPrompt_incident)},
	}

	resp, err := client.GetChatCompletions(context.TODO(), azopenai.ChatCompletionsOptions{
		Messages:         messages,
		DeploymentName:   &deploymentName,
		MaxTokens:        &maxTokens,
		Temperature:      &temperature,
		TopP:             &topP,
		FrequencyPenalty: &frequencyPenalty,
		PresencePenalty:  &presencePenalty,
		Stop:             stop,
	}, nil)
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	// Print the response
	for _, choice := range resp.Choices {
		if choice.Message != nil && choice.Message.Content != nil {
			content := *choice.Message.Content
			start := strings.Index(content, "{")
			end := strings.LastIndex(content, "}")
			if start == -1 || end == -1 || start >= end {
				logger.Fatalf("Invalid response format: %s", content)
			}
			jsonContent := content[start : end+1] // Extract the JSON part
			logger.Printf("Extracted JSON content: %s\n", jsonContent)

			// get "Result" and "Reason" from the json response and update the result map
			if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
				logger.Fatalf("Failed to unmarshal response: %v", err)
			}
			logger.Printf("Validation result: %v\n", result)
		}
	}
	return result
}

// Const system prompt for AI validation
const SystemPrompt = `You are an expert software engineer. 
You will receive a [Tool Result] produced by a Azure Migrate application and code assessment for Java. This tool is designed to help organizations modernize their Java applications to reduce costs and accelerate innovation. It uses advanced static analysis techniques to understand application structure and dependencies, and provides guidance for refactoring and migrating applications to Azure.
Each result is presented in YAML format and contains the following fields:
  - uri: File path that contains the matching code.
  - message: A description of the identified issue or migration recommendation.
  - codesnip: A code snippet from the source that matches the rule.
  - lineNumber: The specific line number in the file where the code appears.
  - variables: The relevant variables or symbols identified in the code snippet.
Verify whether the message, uri, codesnip, lineNumber, and variables are consistent and logically aligned.
Common false positive is:
  - Code match is in a README or documentation file.
  - Code match is a comment, not actual executable code.
  - Code match is clearly unrelated to the message or incorrect.
  - Any part of the result does not make logical sense or seems incorrectly matched.

Only need to return result with Json. [Result Sample]

[Result Sample]
If all fields are aligned and support the same finding, respond with:
{
  "Result": "true"
}
If they are not aligned, respond with:
{
  "Result": "false",
  "Reason": "<brief explanation of the misalignment>"
}
[/Result Sample]`

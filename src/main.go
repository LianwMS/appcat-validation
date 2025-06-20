package main

import (
	"flag"
	"fmt"
	"lianwMS/appcat_validation/logger"
	"lianwMS/appcat_validation/testcase"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// define some constants for the application
const (
	LogExtension        string = ".log"
	TestResultExtension string = ".md"
	CSVExtension        string = ".csv"
)

func main() {
	// Mock input parameters for testing purposes
	wd, _ := os.Getwd()
	appcatAppFolder := flag.String("appcat", `C:\Users\lianw\sampleRepo\azure-migrate-appcat-for-java-cli-windows-amd64-7.6.0.6-preview`, "Path to AppCat application folder")
	sourceRepoFolder := flag.String("source", filepath.Join(wd, "..", "data", "projects"), "Path to source repo folder")
	baselineFolder := flag.String("baseline", filepath.Join(wd, "..", "data", "baseline"), "Path to baseline folder")
	outputFolder := flag.String("output", filepath.Join(wd, "..", "testResults"), "Path to output folder")
	repoListFile := flag.String("target", filepath.Join(wd, "TargetCatalog", "CI"), "Target projects list")
	flag.Parse()

	// Initialize testing environment
	targetList, err := initTesting(*appcatAppFolder, *sourceRepoFolder, *baselineFolder, *outputFolder, *repoListFile)
	if err != nil {
		fmt.Printf("Error initializing testing: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	var timeInFileName = time.Now().Format("20060102_150405")
	var globalFilePrefix string = "appcat_test"
	logFilePath := filepath.Join(*outputFolder, fmt.Sprintf("%s_%s%s", globalFilePrefix, timeInFileName, LogExtension))
	err = logger.Init(logFilePath, true)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.CloseLogFile()
	var logger = logger.Get()

	// Now use *appcatApplicationFolder, *testDataFolder, *testOutputFolder, *targetProject as your variables
	logger.Printf("AppCat Application Folder: %s", *appcatAppFolder)
	logger.Printf("Source Repo Folder: %s", *sourceRepoFolder)
	logger.Printf("Baseline Folder: %s", *baselineFolder)
	logger.Printf("Output Folder: %s", *outputFolder)
	logger.Printf("Target Projects List: %s", *repoListFile)
	logger.Printf("Target Projects Found: %s", strings.Join(targetList, "\n"))

	actionList := []testcase.ActionType{testcase.ActionRun, testcase.ActionValidate}
	testCases := []testcase.TestCase{}

	// Initialize test case
	for _, target := range targetList {
		testCase := testcase.TestCase{
			Name:              target,
			ApplicationFolder: *appcatAppFolder,
			ProjectFolder:     filepath.Join(*sourceRepoFolder, target),
			BaseLineFolder:    filepath.Join(*baselineFolder, target, "appcat_output"),
			OutputFolder:      filepath.Join(*outputFolder, target),
			ActionList:        actionList,
		}
		logger.Printf("%s Created", testCase.GetInfo())
		testCases = append(testCases, testCase)
	}
	logger.Printf("Total Test Cases: %d", len(testCases))

	fullResults := make(map[string]string)
	fullIncidentsCount := 0
	fullIncidentDetails := make(map[string](map[string]int))
	for _, testCase := range testCases {
		logger.Printf("Processing Test Case: %s", testCase.Name)
		resultMessage, resultCount, resultDetails, caseErr := testCase.Run()
		if caseErr != nil {
			logger.Printf("Error running test case %s: %v", testCase.Name, caseErr)
			fullResults[testCase.Name] = fmt.Sprintf("Error: %v", caseErr)
		}
		fullResults[testCase.Name] = resultMessage
		if resultCount >= 0 {
			fullIncidentsCount += resultCount
		}
		if resultDetails != nil {
			fullIncidentDetails[testCase.Name] = resultDetails
		}
		logger.Printf("Completed Test Case: %s", testCase.Name)
	}

	// Testoutput file path
	resultFilePath := filepath.Join(*outputFolder, fmt.Sprintf("%s_%s%s", globalFilePrefix, timeInFileName, TestResultExtension))

	// Write test results to output file
	testOutputFile, err := os.Create(resultFilePath)
	if err != nil {
		logger.Fatalf("Failed to create test output file: %v", err)
	}
	defer testOutputFile.Close()
	// Write header
	testOutputFile.WriteString("# AppCat Test Results\n")
	for _, result := range fullResults {
		testOutputFile.WriteString(result + "\n")
	}

	// If actionList contains ActionAnalyze, generate a summary report
	if fullIncidentsCount > 0 {
		logger.Printf("Total incidents found across all projects: %d", fullIncidentsCount)

		rules := make(map[string]int)
		for _, targetHash := range fullIncidentDetails {
			for rule := range targetHash {
				if _, exists := rules[rule]; !exists {
					rules[rule] = targetHash[rule]
				} else {
					rules[rule] += targetHash[rule]
				}
			}
		}

		summaryFilePath := filepath.Join(*outputFolder, fmt.Sprintf("%s_%s%s", globalFilePrefix, timeInFileName, CSVExtension))
		summaryFile, err := os.Create(summaryFilePath)
		if err != nil {
			logger.Fatalf("Failed to create summary file: %v", err)
		}
		defer summaryFile.Close()

		targetNameList := strings.Join(targetList, ",")
		summaryFile.WriteString(fmt.Sprintf("Rule,%s\n", targetNameList))
		for rule, _ := range rules {
			rowValue := rule
			for _, target := range targetList {
				if targetHash, exists := fullIncidentDetails[target]; exists {
					if count, exists := targetHash[rule]; exists {
						rowValue += fmt.Sprintf(",%d", count)
					} else {
						rowValue += ", "
					}
				} else {
					rowValue += ", "
				}
			}
			summaryFile.WriteString(fmt.Sprintf("%s\n", rowValue))
		}

		// for rule, targetHash := range fullIncidentDetails {
		// 	rowValue := rule
		// 	for _, target := range targetList {
		// 		if count, exists := targetHash[target]; exists {
		// 			rowValue += fmt.Sprintf(",%d", count)
		// 		} else {
		// 			rowValue += ",0"
		// 		}
		// 	}

		logger.Printf("[Analyze] Global summary written to: %s\n", summaryFilePath)
	}
}

func initTesting(appcatAppFolder string, sourceRepoFolder string, baselineFolder string, outputFolder string, repoListFile string) ([]string, error) {
	// Verifty appcatAppFolder
	if _, err := os.Stat(appcatAppFolder); os.IsNotExist(err) {
		return nil, fmt.Errorf("the application folder path '%s' does not exist", appcatAppFolder)
	}

	// Verify sourceRepoFolder
	if _, err := os.Stat(sourceRepoFolder); os.IsNotExist(err) {
		return nil, fmt.Errorf("the source repo folder path '%s' does not exist", sourceRepoFolder)
	}

	// Verify baselineFolder
	if _, err := os.Stat(baselineFolder); os.IsNotExist(err) {
		return nil, fmt.Errorf("the baseline folder path '%s' does not exist", baselineFolder)
	}

	// Create output folder if it does not exist
	if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
		err := os.MkdirAll(outputFolder, 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create output folder: %v", err)
		}
	}

	// Verify repoListFile is exist and read it to get the list of target and return it
	if _, err := os.Stat(repoListFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("the target projects list file '%s' does not exist", repoListFile)
	}
	// Read the repoListFile to get the list of target projects
	file, err := os.ReadFile(repoListFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read target projects list file '%s': %v", repoListFile, err)
	}

	lines := strings.Split(string(file), "\n")
	targetList := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			targetList = append(targetList, trimmed)
		}
	}

	return targetList, nil
}

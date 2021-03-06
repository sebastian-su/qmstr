package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/QMSTR/qmstr/lib/go-qmstr/analysis"
	"github.com/QMSTR/qmstr/lib/go-qmstr/master"
	"github.com/QMSTR/qmstr/lib/go-qmstr/service"
	"github.com/QMSTR/qmstr/lib/go-qmstr/tester"
)

const (
	queryType = "sourcecode"
)

var (
	pkgNode         *service.PackageNode
	tests           []string
	testfunction    func(*testing.T)
	expectedTargets []string
)

type TestAnalyzer struct{}

func main() {
	fmt.Println("This is the testalyzer")
	analyzer := analysis.NewAnalyzer(&TestAnalyzer{})
	if err := analyzer.RunAnalyzerModule(); err != nil {
		log.Printf("%v failed: %v\n", analyzer.GetModuleName(), err)
		os.Exit(master.ReturnAnalyzerFailed)
	}
}

func (testanalyzer *TestAnalyzer) Configure(configMap map[string]string) error {
	if _, ok := configMap["tests"]; !ok {
		log.Println("No build graph tests provided. Running default test.")
		return nil
	}
	tests = strings.Split(configMap["tests"], ";")
	return nil
}

func (testanalyzer *TestAnalyzer) Analyze(controlService service.ControlServiceClient, analysisService service.AnalysisServiceClient, token int64) error {
	pkgNodeStream, err := controlService.GetPackageNode(context.Background(), &service.PackageNode{})
	if err != nil {
		return err
	}

	pkgNode, err = pkgNodeStream.Recv()
	if err != nil {
		return err
	}

	// Run tests for package node
	for _, test := range tests {
		if test == "TestPackageNode" {
			testfunction = TestPackageNode
		} else if test == "TestCalcBuildGraph" {
			expectedTargets = []string{"Calculator/calc", "Calculator/libcalc.so", "Calculator/libcalc.a", "Calculator/calcs"}
			testfunction = TestBuildGraph
		} else if test == "TestCurlBuildGraph" {
			expectedTargets = []string{"curl/build/src/curl", "curl/build/lib/libcurl.so"}
			testfunction = TestBuildGraph
		} else {
			log.Printf("Unknown test. Please check the test name provided in the configuration.")
			os.Exit(master.ReturnAnalyzerFailed)
		}
		testSuite := []testing.InternalTest{
			{
				Name: test,
				F:    testfunction,
			},
		}
		t := &tester.DummyTestDeps{}
		testing.MainStart(t, testSuite, nil, nil).Run()
	}

	return nil
}

func (testanalyzer *TestAnalyzer) PostAnalyze() error {
	return nil
}

func TestPackageNode(t *testing.T) {
	if len(pkgNode.Targets) < 1 {
		t.Logf("Package node '%v' is not connected to any linked targets", pkgNode.Name)
		t.Fail()
	}
}

func TestBuildGraph(t *testing.T) {
	if len(pkgNode.Targets) < 2 {
		t.Logf("Package node '%v' is not connected to all the configured linked targets", pkgNode.Name)
		t.Fail()
	} else {
		for _, expectedTarget := range expectedTargets {
			var found bool
			for _, target := range pkgNode.Targets {
				if expectedTarget == target.Path {
					found = true
					t.Logf("Package node %v is connected to %v", pkgNode.Name, target.Path)
				}
			}
			if !found {
				t.Logf("Package node %v is not connected to the configured linked target: %s", pkgNode.Name, expectedTarget)
				t.Fail()
			}
		}
	}
}

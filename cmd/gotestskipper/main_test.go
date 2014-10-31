package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/mitch000001/go-tools/test_skipper"
)

func TestSkipTests(t *testing.T) {
	t.Skip()
	testDir := "/tmp/gotestskipper/"
	src := `
	package main

	import "fmt"

	func TestFoo(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`
	fileCount := 2
	withFixtureFiles(testDir, src, fileCount, func() {
		cmd := exec.Command("go", "run", "main.go", "/tmp/gotestskipper")
		rc, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		err = cmd.Start()
		if err != nil {
			t.Fatalf("Expected no error, got '%T' with message '%s'", err, err.Error())
		}
		expected := `
		package main

		import "fmt"

		func TestFoo(t *testing.T) {
			t.Skip()
			s := "foo"
			fmt.Println(s)
		}`
		allExpected := ""
		for i := 1; i <= fileCount; i++ {
			allExpected = allExpected + expected
		}
		bytes, err := ioutil.ReadAll(rc)
		if err != nil {
			t.Fatalf("Expected no error, got '%T' with message '%s'", err, err.Error())
		}

		replacer := strings.NewReplacer("\n", "", "\t", "", " ", "")
		allExpected = replacer.Replace(allExpected)
		actual := replacer.Replace(string(bytes))

		if allExpected != actual {
			t.Fatalf("Expected \n`%s`\n\n, got \n`%s`\n", allExpected, actual)
		}
	})
}

func TestOutputStrategyWriteToFile(t *testing.T) {
	// Valid path
	path := "/tmp/bar"
	err := ioutil.WriteFile(path, []byte{}, 0700)
	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}
	defer os.Remove(path)
	content := "foo"

	pWriter := make(testskipper.PathWriter)
	writer := pWriter.ReadWriterForPath(path)
	_, err = writer.Write([]byte(content))
	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}

	strategy := &OutputStrategy{pWriter}
	err = strategy.WriteToFile()

	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}

	fileContent, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}
	fileContentString := string(fileContent)

	if fileContentString != content {
		t.Fatalf("Expected fileContent '%s', got '%s'\n", content, fileContentString)
	}

	// Invalid path
	path = "/tmp/invalid"
	writer = pWriter.ReadWriterForPath(path)
	_, err = writer.Write([]byte(content))
	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}
	err = strategy.WriteToFile()
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	if _, ok := err.(*os.PathError); !ok {
		t.Fatalf("Expected '*os.PathError', got '%T' with message: '%s'", err, err.Error())
	}
}

func TestOutputStrategyWriteToStdout(t *testing.T) {
	path := "/tmp/bar"
	content := "foo"

	pWriter := make(testskipper.PathWriter)
	writer := pWriter.ReadWriterForPath(path)
	_, err := writer.Write([]byte(content))
	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}
	realStdout := os.Stdout
	defer func() {
		os.Stdout = realStdout
	}()
	r, w, err := os.Pipe()
	os.Stdout = w

	strategy := &OutputStrategy{pWriter}
	err = strategy.WriteToStdout()

	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}

	buffer := make([]byte, len(content))
	_, err = r.Read(buffer)
	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}
	printedString := string(buffer)
	if printedString != content {
		t.Fatalf("Expected output '%s', got '%s'\n", content, printedString)
	}
}

func withFixtureFiles(dir string, src string, fileCount int, testFunc func()) {
	err := os.Mkdir(dir, 0777)
	if err != nil {
		panic(err)
	}
	for i := 1; i <= fileCount; i++ {
		fileName := fmt.Sprintf("go%d_test.go", i)
		err = ioutil.WriteFile(path.Join(dir, fileName), []byte(src), 0777)
		if err != nil {
			panic(err)
		}
	}
	defer func() {
		err = os.RemoveAll(dir)
		if err != nil {
			panic(err)
		}
	}()
	testFunc()
}

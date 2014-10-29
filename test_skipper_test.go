package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestTestFuncVisitor(t *testing.T) {
	src := `
		package main

		import "testing"

		func testFoo(testing.T) {}
		func bar(*testing.T) {}
		func TestBaz(string) {}
		func Test(*testing.T) {}
	`
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", src, parser.AllErrors)
	if err != nil {
		t.Fatalf("Error parsing source code: `%s`", src)
	}
	var buffer bytes.Buffer
	visitAction := func(f *ast.FuncDecl) {
		printer.Fprint(&buffer, fileSet, f)
	}

	ast.Walk(&testFuncVisitor{visitAction: visitAction}, file)

	expected := "func Test(*testing.T) {}"
	actual := strings.Replace(strings.Trim(buffer.String(), " \t\n"), "\t", " ", -1)

	if actual != expected {
		t.Fatalf("Expected '%s', got '%s'\n", expected, actual)
	}
}

func TestNewTestFuncVisitor(t *testing.T) {
	// With no visitAction func
	visitor := NewTestFuncVisitor(nil)

	if visitor.(*testFuncVisitor).visitAction == nil {
		t.Fatalf("Expected visitAction to not be nil")
	}

	var actual string
	visitAction := func(*ast.FuncDecl) {
		actual = "called"
	}

	visitor = NewTestFuncVisitor(visitAction)
	visitor.(*testFuncVisitor).visitAction(&ast.FuncDecl{})

	if actual != "called" {
		t.Fatal("Expected visitAction to be set properly")
	}
}

func TestSkipTestVisitorAction(t *testing.T) {
	src := `
	package main

	import "fmt"

	func TestFoo(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", src, parser.AllErrors)
	if err != nil {
		panic(err)
	}

	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fDecl, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fDecl
		}
	}

	skipTestVisitorAction(funcDecl)

	var buffer bytes.Buffer
	printer.Fprint(&buffer, fileSet, file)

	replacer := strings.NewReplacer("\n", "", "\t", "", " ", "")

	expected := `
	package main

	import "fmt"

	func TestFoo(t *testing.T) {
		t.Skip()

		s := "foo"
		fmt.Println(s)
	}`
	expected = replacer.Replace(expected)
	actual := replacer.Replace(buffer.String())

	if expected != actual {
		t.Fatalf("Expected \n`%s`\n\n, got \n`%s`\n", expected, actual)
	}
}

func TestUnskipTestVisitorAction(t *testing.T) {
	src := `
	package main

	import "fmt"

	func TestFoo(t *testing.T) {
		t.Skip()

		s := "foo"
		fmt.Println(s)
	}`
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", src, parser.AllErrors)
	if err != nil {
		panic(err)
	}

	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fDecl, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fDecl
		}
	}

	unskipTestVisitorAction(funcDecl)

	var buffer bytes.Buffer
	printer.Fprint(&buffer, fileSet, file)

	replacer := strings.NewReplacer("\n", "", "\t", "", " ", "")

	expected := `
	package main

	import "fmt"

	func TestFoo(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`
	expected = replacer.Replace(expected)
	actual := replacer.Replace(buffer.String())

	if expected != actual {
		t.Fatalf("Expected \n`%s`\n\n, got \n`%s`\n", expected, actual)
	}
}

type visitor struct{}

func (v visitor) Visit(node ast.Node) ast.Visitor {
	if f, ok := node.(*ast.FuncDecl); ok {
		name := ast.NewIdent("TestBar")
		f.Name = name
		return nil
	}
	return v
}

func TestWalkFile(t *testing.T) {
	src := `
	package main

	import "fmt"

	func TestFoo(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`

	tmpFilePath := "tempFile.go"
	err := ioutil.WriteFile(tmpFilePath, []byte(src), 0777)
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpFilePath)

	var buffer bytes.Buffer

	err = walkFile(tmpFilePath, &buffer, &visitor{})

	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}

	expected := `
	package main

	import "fmt"

	func TestBar(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`
	replacer := strings.NewReplacer("\n", "", "\t", "", " ", "")
	expected = replacer.Replace(expected)
	actual := replacer.Replace(buffer.String())

	if expected != actual {
		t.Fatalf("Expected \n`%s`\n\n, got \n`%s`\n", expected, actual)
	}

	// No real path
	buffer.Reset()
	err = walkFile("foobar.go", &buffer, &visitor{})
	if err == nil {
		t.Fatal("Expected an error")
	}
}

func TestWalkDir(t *testing.T) {
	src := `
	package main

	import "fmt"

	func TestFoo(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`
	src2 := `
	package main

	import "fmt"

	func TestBaz(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`

	tmpDir := "foo"
	tmpFilePath := "tempFile.go"
	tmpFilePath2 := "tempFile2.go"
	err := os.Mkdir(tmpDir, 0777)
	err = ioutil.WriteFile(path.Join(tmpDir, tmpFilePath), []byte(src), 0777)
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile(path.Join(tmpDir, tmpFilePath2), []byte(src2), 0777)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	output := NewOutputStrategy()

	err = walkDir(tmpDir, output, &visitor{})

	if err != nil {
		t.Fatalf("Expected no error, got '%T' with message: '%s'\n", err, err.Error())
	}

	replacer := strings.NewReplacer("\n", "", "\t", "", " ", "")

	var actual string
	for _, buffer := range output.pathWriters {
		actual = actual + replacer.Replace(buffer.String())
	}

	expected := `
	package main

	import "fmt"

	func TestBar(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}
	package main

	import "fmt"

	func TestBar(t *testing.T) {
		s := "foo"
		fmt.Println(s)
	}`
	expected = replacer.Replace(expected)

	if expected != actual {
		t.Fatalf("Expected \n`%s`\n\n, got \n`%s`\n", expected, actual)
	}

	// No real path
	output = NewOutputStrategy()
	err = walkDir("foobar", output, &visitor{})
	if err == nil {
		t.Fatal("Expected an error")
	}
}

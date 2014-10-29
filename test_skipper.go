package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"strings"
)

var (
	write    = flag.Bool("w", false, "write result to (source) file instead of stdout")
	unskip   = flag.Bool("u", false, "unskips all skipped tests instead of skipping them")
	exitCode = 0
)

type testFuncVisitor struct {
	visitAction func(*ast.FuncDecl)
}

func (f testFuncVisitor) Visit(node ast.Node) ast.Visitor {
	if funcDecl, ok := node.(*ast.FuncDecl); ok {
		if strings.HasPrefix(funcDecl.Name.Name, "Test") {
			if len(funcDecl.Type.Params.List) == 1 {
				param := funcDecl.Type.Params.List[0]
				var buffer bytes.Buffer
				printer.Fprint(&buffer, token.NewFileSet(), param.Type)
				if "*testing.T" == buffer.String() {
					f.visitAction(funcDecl)
					return nil
				}
			}
		}
	}
	return f
}

func NewTestFuncVisitor(visitAction func(*ast.FuncDecl)) ast.Visitor {
	if visitAction == nil {
		visitAction = func(*ast.FuncDecl) {}
	}
	return &testFuncVisitor{visitAction: visitAction}
}

func skipTestVisitorAction(f *ast.FuncDecl) {
	testingParamName := f.Type.Params.List[0].Names[0].Name
	skipTestString := fmt.Sprintf("%s.Skip();", testingParamName)
	skipTestExpr, err := parser.ParseExpr(skipTestString)
	if err != nil {
		panic(err)
	}
	newBodyList := make([]ast.Stmt, len(f.Body.List)+1)
	newBodyList[0] = &ast.ExprStmt{X: skipTestExpr}
	for i, stmt := range f.Body.List {
		newBodyList[i+1] = stmt
	}
	f.Body.List = newBodyList
}

func unskipTestVisitorAction(f *ast.FuncDecl) {
	testingParamName := f.Type.Params.List[0].Names[0].Name
	skipTestString := fmt.Sprintf("%s.Skip()", testingParamName)
	var buffer bytes.Buffer
	printer.Fprint(&buffer, token.NewFileSet(), f.Body.List[0])
	if buffer.String() == skipTestString {
		newBodyList := make([]ast.Stmt, len(f.Body.List)-1)
		for i, _ := range newBodyList {
			newBodyList[i] = f.Body.List[i+1]
		}
		f.Body.List = newBodyList
	}
}

type outputStrategy struct {
	pathWriters map[string]*bytes.Buffer
}

func NewOutputStrategy() *outputStrategy {
	o := &outputStrategy{}
	o.pathWriters = make(map[string]*bytes.Buffer)
	return o
}

func (o *outputStrategy) WriterForPath(path string) io.Writer {
	if writer, ok := o.pathWriters[path]; ok {
		return writer
	}
	var writer bytes.Buffer
	o.pathWriters[path] = &writer
	return &writer
}

func (o *outputStrategy) WriteToFile() error {
	for path, buffer := range o.pathWriters {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			return err
		}
		_, err = io.Copy(file, buffer)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *outputStrategy) WriteToStdout() error {
	for _, buffer := range o.pathWriters {
		_, err := io.Copy(os.Stdout, buffer)
		if err != nil {
			return err
		}
	}
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: test_skipper [flags] [path ...]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	var visitAction func(*ast.FuncDecl)
	if *unskip {
		visitAction = unskipTestVisitorAction
	} else {
		visitAction = skipTestVisitorAction
	}

	if flag.NArg() == 0 {
		flag.Usage()
	}

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)

		testFuncVisitor := NewTestFuncVisitor(visitAction)
		output := NewOutputStrategy()

		switch dir, err := os.Stat(path); {
		case err != nil:
			report(err)
		case dir.IsDir():
			if err := walkDir(path, output, testFuncVisitor); err != nil {
				report(err)
			}
		default:
			writer := output.WriterForPath(path)
			if err := walkFile(path, writer, testFuncVisitor); err != nil {
				report(err)
			} else {
				err := writeOutput(output)
				if err != nil {
					report(err)
				}
			}
		}
	}
	os.Exit(exitCode)
}

func writeOutput(output *outputStrategy) error {
	if *write {
		err := output.WriteToFile()
		if err != nil {
			return err
		}
	} else {
		err := output.WriteToStdout()
		if err != nil {
			return err
		}
	}
	return nil
}

func report(err error) {
	scanner.PrintError(os.Stderr, err)
	exitCode = 2
}

func onlyTestFileAndDirFilter(info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	if strings.HasSuffix(info.Name(), "_test") {
		return false
	}
	return true
}

func walkDir(path string, output *outputStrategy, visitor ast.Visitor) error {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, path, onlyTestFileAndDirFilter, parser.ParseComments)
	if err != nil {
		return err
	}
	fmt.Printf("Packages: %+#v\n", packages)
	for _, pkg := range packages {
		for path, file := range pkg.Files {
			writer := output.WriterForPath(path)
			ast.Walk(visitor, file)
			printer.Fprint(writer, fileSet, file)
		}
	}
	return nil
}

func walkFile(path string, output io.Writer, visitor ast.Visitor) error {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	ast.Walk(visitor, file)
	printer.Fprint(output, fileSet, file)
	return nil
}

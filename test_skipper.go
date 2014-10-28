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
	"io/ioutil"
	"os"
	"strings"
)

var (
	write    = flag.Bool("w", false, "write result to (source) file instead of stdout")
	unskip   = flag.Bool("u", false, "unskips all skipped tests instead of skipping them")
	exitCode = 0
)

type TestFuncVisitor struct {
	visitAction func(*ast.FuncDecl)
}

func (f TestFuncVisitor) Visit(node ast.Node) ast.Visitor {
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
	return &TestFuncVisitor{visitAction: visitAction}
}

func SkipTestVisitorAction(f *ast.FuncDecl) {
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

func UnskipTestVisitorAction(f *ast.FuncDecl) {
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

type OutputStrategy struct {
	pathWriters map[string]*bytes.Buffer
}

func (o *OutputStrategy) WriterForPath(path string) io.Writer {
	if writer, ok := o.pathWriters[path]; ok {
		return writer
	}
	var writer bytes.Buffer
	o.pathWriters[path] = &writer
	return &writer
}

func (o *OutputStrategy) WriteToFile() error {
	for path, buffer := range o.pathWriters {
		err := ioutil.WriteFile(path, buffer.Bytes(), 0)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *OutputStrategy) WriteToStdout() error {
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
		visitAction = UnskipTestVisitorAction
	} else {
		visitAction = SkipTestVisitorAction
	}

	if flag.NArg() == 0 {
		flag.Usage()
	}

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)

		testFuncVisitor := NewTestFuncVisitor(visitAction)
		output := &OutputStrategy{}

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

func writeOutput(output *OutputStrategy) error {
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

func walkDir(path string, output *OutputStrategy, visitor ast.Visitor) error {
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

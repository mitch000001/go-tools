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
	FileSet     *token.FileSet
	visitAction func(*ast.FuncDecl)
}

func (f TestFuncVisitor) Visit(node ast.Node) ast.Visitor {
	if funcDecl, ok := node.(*ast.FuncDecl); ok {
		if strings.HasPrefix(funcDecl.Name.Name, "Test") {
			if len(funcDecl.Type.Params.List) == 1 {
				param := funcDecl.Type.Params.List[0]
				var buffer bytes.Buffer
				printer.Fprint(&buffer, f.FileSet, param.Type)
				if "*testing.T" == buffer.String() {
					f.visitAction(funcDecl)
					return nil
				}
			}
		}
	}
	return f
}

func NewTestFuncVisitor(fileSet *token.FileSet, visitAction func(*ast.FuncDecl)) ast.Visitor {
	if visitAction == nil {
		visitAction = func(*ast.FuncDecl) {}
	}
	return &TestFuncVisitor{FileSet: fileSet, visitAction: visitAction}
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

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)
		switch dir, err := os.Stat(path); {
		case err != nil:
			report(err)
		case dir.IsDir():
			walkDir(path)
		default:
			var buffer bytes.Buffer
			if err := walkFile(path, &buffer, visitAction); err != nil {
				report(err)
			} else {
				if *write {
					err = ioutil.WriteFile(path, buffer.Bytes(), 0)
					if err != nil {
						report(err)
					}
				} else {
					_, err = io.Copy(os.Stdout, &buffer)
					if err != nil {
						report(err)
					}
				}
			}
		}
	}
	os.Exit(exitCode)
}

func report(err error) {
	scanner.PrintError(os.Stderr, err)
	exitCode = 2
}

func walkDir(path string) {}

func walkFile(path string, output io.Writer, visitAction func(*ast.FuncDecl)) error {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	ast.Walk(NewTestFuncVisitor(fileSet, visitAction), file)
	printer.Fprint(output, fileSet, file)
	return nil
}

package testskipper

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"strings"
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

const skipTestStatementTemplate = "%s.Skip()"

func SkipTestVisitorAction(f *ast.FuncDecl) {
	testingParamName := f.Type.Params.List[0].Names[0].Name
	skipTestString := fmt.Sprintf(skipTestStatementTemplate, testingParamName)
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
	skipTestString := fmt.Sprintf(skipTestStatementTemplate, testingParamName)
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

type PathWriter map[string]*bytes.Buffer

func (p PathWriter) WriterForPath(path string) io.Writer {
	if writer, ok := p[path]; ok {
		return writer
	}
	var writer bytes.Buffer
	p[path] = &writer
	return &writer
}

type OutputStrategy struct {
	PathWriter PathWriter
}

func (o *OutputStrategy) WriteToFile() error {
	for path, buffer := range o.PathWriter {
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

func (o *OutputStrategy) WriteToStdout() error {
	for _, buffer := range o.PathWriter {
		_, err := io.Copy(os.Stdout, buffer)
		if err != nil {
			return err
		}
	}
	return nil
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

func WalkDir(path string, pathWriter PathWriter, visitor ast.Visitor) error {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, path, onlyTestFileAndDirFilter, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, pkg := range packages {
		for path, file := range pkg.Files {
			writer := pathWriter.WriterForPath(path)
			ast.Walk(visitor, file)
			printer.Fprint(writer, fileSet, file)
		}
	}
	return nil
}

func WalkFile(path string, output io.Writer, visitor ast.Visitor) error {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	ast.Walk(visitor, file)
	printer.Fprint(output, fileSet, file)
	return nil
}

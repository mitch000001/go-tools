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
	visitAction FuncVisitAction
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

type FuncVisitAction func(*ast.FuncDecl)

// NewTestFuncVisitor returns an ast.Visitor which performs the action
// specified in visitAction
//
// The visitor will only call the visitAction on test function declarations
func NewTestFuncVisitor(visitAction FuncVisitAction) ast.Visitor {
	return &testFuncVisitor{visitAction: visitAction}
}

const skipTestStatementTemplate = "%s.Skip()"

// SkipTestVisitorAction defines a visitAction which adds a
//  t.Skip()
// statement to the test function
//
// It is garanteed that the *ast.FuncDecl is a testing function with the
// signature func TestXXX(*testing.T)
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

// UnSkipTestVisitorAction defines a visitAction which removes a
//  t.Skip()
// statement from the test function if given at first line of the func body
//
// It is garanteed that the *ast.FuncDecl is a testing function with the
// signature func TestXXX(*testing.T)
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

// PathWriter provides a mapping of paths to buffers
type PathWriter map[string]*bytes.Buffer

// ReadWriterForPath returns an io.ReadWriter for the provided path
// If there is already an entry for path, the io.ReadWriter associated
// to that path will be returned, otherwise an empty io.ReadWriter is returned
func (p PathWriter) ReadWriterForPath(path string) io.ReadWriter {
	if writer, ok := p[path]; ok {
		return writer
	}
	var writer bytes.Buffer
	p[path] = &writer
	return &writer
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

// WalkDir applies the visitor to all files found at path and writes the visited
// AST into pathWriter.
func WalkDir(path string, pathWriter PathWriter, visitor ast.Visitor) error {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, path, onlyTestFileAndDirFilter, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, pkg := range packages {
		for path, file := range pkg.Files {
			writer := pathWriter.ReadWriterForPath(path)
			ast.Walk(visitor, file)
			printer.Fprint(writer, fileSet, file)
		}
	}
	return nil
}

// WalkFile applies the visitor to the file found at path and writes the visited
// AST into output.
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

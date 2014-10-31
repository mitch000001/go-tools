package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/scanner"
	"os"

	"github.com/mitch000001/go-tools/test_skipper"
)

var (
	write    = flag.Bool("w", false, "write result to (source) file instead of stdout")
	unskip   = flag.Bool("u", false, "unskips all skipped tests instead of skipping them")
	exitCode = 0
)

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
		visitAction = testskipper.UnskipTestVisitorAction
	} else {
		visitAction = testskipper.SkipTestVisitorAction
	}

	if flag.NArg() == 0 {
		flag.Usage()
	}

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)

		testFuncVisitor := testskipper.NewTestFuncVisitor(visitAction)

		pathWriter := make(testskipper.PathWriter)
		output := &testskipper.OutputStrategy{pathWriter}

		switch dir, err := os.Stat(path); {
		case err != nil:
			report(err)
		case dir.IsDir():
			if err := testskipper.WalkDir(path, pathWriter, testFuncVisitor); err != nil {
				report(err)
			}
		default:
			writer := pathWriter.WriterForPath(path)
			if err := testskipper.WalkFile(path, writer, testFuncVisitor); err != nil {
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

func writeOutput(output *testskipper.OutputStrategy) error {
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

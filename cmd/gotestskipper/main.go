package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/scanner"
	"io"
	"os"

	"github.com/mitch000001/go-tools/testskipper"
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

type OutputStrategy struct {
	PathWriter testskipper.PathWriter
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
		output := &OutputStrategy{pathWriter}

		switch dir, err := os.Stat(path); {
		case err != nil:
			report(err)
		case dir.IsDir():
			if err := testskipper.WalkDir(path, pathWriter, testFuncVisitor); err != nil {
				report(err)
			} else {
				err := writeOutput(output)
				if err != nil {
					report(err)
				}
			}

		default:
			writer := pathWriter.ReadWriterForPath(path)
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

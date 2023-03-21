package exec_test

import (
	"bytes"
	"fmt"
	"strings"

	"go.nhat.io/exec"
)

func ExampleRun() {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	_, err := exec.Run("echo", exec.WithArgs("hello world"),
		exec.WithStdout(stdout),
		exec.WithStderr(stderr),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(stdout.String())
	fmt.Println(stderr.String())

	// Output:
	// hello world
	//
}

func ExampleRun_pipe() {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	_, err := exec.Run("echo", exec.WithArgs("hello world"),
		exec.WithStdout(stdout),
		exec.WithStderr(stderr),
		exec.Pipe("grep", "-o", "hello"),
		exec.Pipe("tr", "[:lower:]", "[:upper:]"),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(stdout.String())
	fmt.Println(stderr.String())

	// Output:
	// HELLO
	//
}

func ExampleRun_stdin() {
	stdin := strings.NewReader("hello world")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	_, err := exec.Run("cat",
		exec.WithStdin(stdin),
		exec.WithStdout(stdout),
		exec.WithStderr(stderr),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(stdout.String())
	fmt.Println(stderr.String())

	// Output:
	// hello world
	//
}

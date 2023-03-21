# Exec

[![GitHub Releases](https://img.shields.io/github/v/release/nhatthm/go-exec)](https://github.com/nhatthm/go-exec/releases/latest)
[![Build Status](https://github.com/nhatthm/go-exec/actions/workflows/test.yaml/badge.svg)](https://github.com/nhatthm/go-exec/actions/workflows/test.yaml)
[![codecov](https://codecov.io/gh/nhatthm/go-exec/branch/master/graph/badge.svg?token=eTdAgDE2vR)](https://codecov.io/gh/nhatthm/go-exec)
[![Go Report Card](https://goreportcard.com/badge/go.nhat.io/exec)](https://goreportcard.com/report/go.nhat.io/exec)
[![GoDevDoc](https://img.shields.io/badge/dev-doc-00ADD8?logo=go)](https://pkg.go.dev/go.nhat.io/exec)
[![Donate](https://img.shields.io/badge/Donate-PayPal-green.svg)](https://www.paypal.com/donate/?hosted_button_id=PJZSGJN57TDJY)

Package `exec` runs external commands.

## Prerequisites

- `Go >= 1.19`

## Install

```bash
go get go.nhat.io/exec
```

## Usage

Run a command:

```go
package example_test

import (
	"bytes"
	"fmt"

	"go.nhat.io/exec"
)

func ExampleRun() {
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)

	_, err := exec.Run("echo", exec.WithArgs("hello world"),
		exec.WithStdout(outBuf),
		exec.WithStderr(errBuf),
	)

	if err != nil {
		panic(err)
	}

	fmt.Println(outBuf.String())
	fmt.Println(errBuf.String())

	// Output:
	// hello world
	//
}
```

Run a command with pipes:

```go
package example_test

import (
    "bytes"
    "fmt"

    "go.nhat.io/exec"
)

func ExampleRun_pipe() {
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)

	_, err := exec.Run("echo", exec.WithArgs("hello world"),
		exec.WithStdout(outBuf),
		exec.WithStderr(errBuf),
		exec.Pipe("grep", "-o", "hello"),
		exec.Pipe("tr", "[:lower:]", "[:upper:]"),
	)

	if err != nil {
		panic(err)
	}

	fmt.Println(outBuf.String())
	fmt.Println(errBuf.String())

	// Output:
	// HELLO
	//
}
```

## Donation

If this project help you reduce time to develop, you can give me a cup of coffee :)

### Paypal donation

[![paypal](https://www.paypalobjects.com/en_US/i/btn/btn_donateCC_LG.gif)](https://www.paypal.com/donate/?hosted_button_id=PJZSGJN57TDJY)

&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;or scan this

<img src="https://user-images.githubusercontent.com/1154587/113494222-ad8cb200-94e6-11eb-9ef3-eb883ada222a.png" width="147px" />

package exec_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/bool64/ctxd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"go.nhat.io/exec"
	exectest "go.nhat.io/exec/test"
)

func TestLookPath(t *testing.T) {
	t.Parallel()

	path, err := exec.LookPath("echo")

	assert.NotEmpty(t, path)
	assert.NoError(t, err)

	path, err = exec.LookPath("not_found")

	assert.Empty(t, path)
	assert.Error(t, err)
}

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("echo", exec.WithArgs("hello world"))

	assert.NotEmpty(t, cmd.Path)
	assert.Equal(t, []string{"hello world"}, cmd.Args[1:])
	assert.NoError(t, cmd.Err)
}

func TestCommandWithContext(t *testing.T) {
	t.Parallel()

	cmd := exec.CommandContext(context.Background(), "echo", exec.WithArgs("hello world"))

	assert.NotEmpty(t, cmd.Path)
	assert.Equal(t, []string{"hello world"}, cmd.Args[1:])
	assert.NoError(t, cmd.Err)
}

func TestCmd_String_LookupError(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("not_found", exec.WithArgs("hello world"))

	actual := cmd.String()
	expected := "not_found 'hello world'"

	assert.Equal(t, expected, actual)
}

func TestCmd_String_Pipe(t *testing.T) {
	t.Parallel()

	echoPath, err := exec.LookPath("echo")
	require.NoError(t, err)

	grepPath, err := exec.LookPath("grep")
	require.NoError(t, err)

	trPath, err := exec.LookPath("tr")
	require.NoError(t, err)

	cmd := exec.Command("echo",
		exec.WithArgs("hello world"),
		exec.Pipe("grep", "-o", "hello"),
		exec.Pipe("tr", "[:lower:]", "[:upper:]"),
	)

	actual := cmd.String()
	expected := "%s 'hello world' | %s -o hello | %s \\[:lower:] \\[:upper:]"
	expected = fmt.Sprintf(expected, echoPath, grepPath, trPath)

	assert.Equal(t, expected, actual)
}

func TestCmd_Start_AlreadyStarted(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("echo", exec.WithArgs("hello world"))

	err := cmd.Start()
	require.NoError(t, err)

	defer cmd.Wait() //nolint:errcheck

	actual := cmd.Start()
	expected := `exec: already started`

	assert.EqualError(t, actual, expected)
}

func TestCmd_Wait_NotStarted(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("echo", exec.WithArgs("hello world"))

	actual := cmd.Wait()
	expected := `exec: not started`

	assert.EqualError(t, actual, expected)
}

func TestCmd_Wait_AlreadyWaited(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("echo", exec.WithArgs("hello world"))

	err := cmd.Start()
	require.NoError(t, err)

	err = cmd.Wait()
	require.NoError(t, err)

	actual := cmd.Wait()
	expected := `exec: Wait was already called`

	assert.EqualError(t, actual, expected)
}

func TestRun_LookUpError(t *testing.T) {
	t.Parallel()

	logger := &ctxd.LoggerMock{}

	cmd, err := exec.Run("random-name-that-does-not-exist",
		exec.WithLogger(logger),
	)

	t.Log(logger.String())

	assert.NotNil(t, cmd)
	assert.ErrorIs(t, err, exec.ErrNotFound)
}

func TestRun_Error_AlreadyStarted(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("echo",
		exec.WithArgs("hello world"),
	)

	err := cmd.Start()
	require.NoError(t, err)

	defer cmd.Wait() //nolint: errcheck

	actual := cmd.Run()
	expected := `exec: already started`

	assert.EqualError(t, actual, expected)
}

func TestRun_Error_CommandExitedWithError(t *testing.T) { //nolint: paralleltest
	const (
		binaryName    = "test-binary"
		binaryContent = `set -e
echo >&2 "this is an error"
exit 1
`
	)

	exectest.Test(binaryName, binaryContent, func(t *testing.T) {
		t.Helper()

		cmd, err := exec.Run(binaryName)

		assert.EqualError(t, err, `exit status 1`)
		assert.False(t, cmd.ProcessState.Success())
	})(t)
}

func TestRun_Success_SingleRun(t *testing.T) { //nolint: paralleltest
	const (
		binaryName    = "test-binary"
		binaryContent = `set -e
echo >&2 "this is an error"
echo "this is an normal output"
echo "ARGS=${@}"
echo "SINGLE_ENV=${SINGLE_ENV}"
echo "ENV_1=${ENV_1}"
echo "ENV_2=${ENV_2}"
echo "STDIN=$(cat -)"
`
	)

	exectest.Test(binaryName, binaryContent, func(t *testing.T) {
		t.Helper()

		stdout := newSafeBuffer()
		stderr := newSafeBuffer()
		stdin := newSafeBuffer()

		_, _ = stdin.Write([]byte("this is stdin")) //nolint: errcheck

		cmd, err := exec.Run(binaryName,
			exec.WithArgs("arg1", "arg2"),
			exec.WithEnv("SINGLE_ENV", "single"),
			exec.WithEnvs(map[string]string{
				"ENV_1": "env1",
				"ENV_2": "env2",
			}),
			exec.WithStdout(stdout),
			exec.WithStderr(stderr),
			exec.WithStdin(stdin),
		)

		assert.NoError(t, err)
		assert.True(t, cmd.ProcessState.Success())

		const (
			expectedStdErr = `this is an error`
			expectedStdOut = `this is an normal output
ARGS=arg1 arg2
SINGLE_ENV=single
ENV_1=env1
ENV_2=env2
STDIN=this is stdin`
		)

		assert.Equal(t, expectedStdErr, getOutput(stderr))
		assert.Equal(t, expectedStdOut, getOutput(stdout))
	})(t)
}

func TestRun_Error_Pipe_MiddleCommandNotFound(t *testing.T) {
	t.Parallel()

	logger := &ctxd.LoggerMock{}
	cmdOut := newSafeBuffer()
	cmdErr := newSafeBuffer()

	_, err := exec.Run("echo", exec.WithArgs("hello world"),
		exec.WithStdout(cmdOut),
		exec.WithStderr(cmdErr),
		exec.WithLogger(logger),
		exec.Pipe("not_found"),
	)

	t.Log(logger.String())

	assert.ErrorIs(t, err, exec.ErrNotFound)
}

func TestRun_Error_Pipe_ErrorAtTheBeginning(t *testing.T) {
	t.Parallel()

	logger := &ctxd.LoggerMock{}
	cmdOut := newSafeBuffer()
	cmdErr := newSafeBuffer()

	_, err := exec.Run("grep", exec.WithArgs("not_found", "/does/not/exist"),
		exec.WithStdout(cmdOut),
		exec.WithStderr(cmdErr),
		exec.WithLogger(logger),
		exec.Pipe("echo", "hello world"),
	)

	t.Log(logger.String())

	assert.EqualError(t, err, `exit status 2`)
}

func TestRun_Error_Pipe_ErrorAtTheMiddle(t *testing.T) {
	t.Parallel()

	logger := &ctxd.LoggerMock{}
	cmdOut := newSafeBuffer()
	cmdErr := newSafeBuffer()

	_, err := exec.Run("echo", exec.WithArgs("a\nb\nc"),
		exec.WithStdout(cmdOut),
		exec.WithStderr(cmdErr),
		exec.WithLogger(logger),
		exec.Pipe("grep", "B"),
		exec.Pipe("sed", "-E", "s#b#B#g"),
	)

	t.Log(logger.String())

	assert.EqualError(t, err, `exit status 1`)
}

func TestRun_Error_Pipe_ErrorATTheEnd(t *testing.T) {
	t.Parallel()

	logger := &ctxd.LoggerMock{}
	cmdOut := newSafeBuffer()
	cmdErr := newSafeBuffer()

	_, err := exec.Run("echo", exec.WithArgs("a\nb\nc"),
		exec.WithStdout(cmdOut),
		exec.WithStderr(cmdErr),
		exec.WithLogger(logger),
		exec.Pipe("sed", "-E", "s#b#B#g"),
		exec.Pipe("grep", "b"),
	)

	t.Log(logger.String())

	assert.EqualError(t, err, `exit status 1`)
}

func TestRun_Success_Pipe(t *testing.T) {
	t.Parallel()

	logger := &ctxd.LoggerMock{}
	cmdOut := newSafeBuffer()
	cmdErr := newSafeBuffer()

	_, err := exec.Run("echo", exec.WithArgs("a\nb\nc"),
		exec.WithStdout(cmdOut),
		exec.WithStderr(cmdErr),
		exec.WithLogger(logger),
		exec.WithTracer(trace.NewNoopTracerProvider().Tracer("")),
		exec.Pipe("grep", "b"),
		exec.Pipe("sed", "-E", "s#b#B#g"),
	)

	t.Log(logger.String())

	require.NoError(t, err)

	assert.Equal(t, "B", getOutput(cmdOut))
}

func Test_AppendArgs(t *testing.T) {
	t.Parallel()

	logger := &ctxd.LoggerMock{}
	cmdOut := newSafeBuffer()
	cmdErr := newSafeBuffer()

	_, err := exec.Run("echo", exec.WithArgs("a", "b"),
		exec.AppendArgs("c", "d"),
		exec.WithStdout(cmdOut),
		exec.WithStderr(cmdErr),
		exec.WithLogger(logger),
	)

	t.Log(logger.String())

	require.NoError(t, err)

	assert.Equal(t, "a b c d", getOutput(cmdOut))
}

func Test_WithArgsRedaction(t *testing.T) {
	t.Parallel()

	_, err := exec.Run("echo", exec.WithArgs("hello"),
		exec.AppendArgs("c", "d"),
		exec.WithStdout(io.Discard),
		exec.WithStderr(io.Discard),
		exec.WithArgsRedaction(func(args []string) []string {
			return nil
		}),
	)

	require.NoError(t, err)
}

func getOutput(s fmt.Stringer) string {
	return strings.Trim(s.String(), "\r\n ")
}

// safeBuffer is a buffer with a lock.
type safeBuffer struct {
	mu  sync.Locker
	buf *bytes.Buffer
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p) //nolint: wrapcheck
}

func (b *safeBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Read(p) //nolint: wrapcheck
}

func newSafeBuffer() *safeBuffer {
	return &safeBuffer{
		mu:  &sync.Mutex{},
		buf: new(bytes.Buffer),
	}
}

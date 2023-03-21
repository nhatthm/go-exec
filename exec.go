// Package exec is a wrapper around os/exec that provides a few extra features.
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bool64/ctxd"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ErrNotFound is the error resulting if a path search failed to find an executable file.
var ErrNotFound = exec.ErrNotFound

// LookPath searches for an executable named file in the directories named by the PATH environment variable.
// If file contains a slash, it is tried directly and the PATH is not consulted.
// Otherwise, on success, the result is an absolute path.
func LookPath(file string) (string, error) {
	return exec.LookPath(file) //nolint: wrapcheck
}

// Cmd is a wrapper around exec.Cmd.
type Cmd struct {
	*exec.Cmd
	Next *Cmd

	ctx    context.Context //nolint: containedctx
	stdErr *bytes.Buffer
	closer io.Closer
	tracer trace.Tracer
	logger ctxd.Logger
}

// String returns a human-readable description of c. It is intended only for debugging.
// In particular, it is not suitable for use as input to a shell.
//
// The output of String may vary across Go releases.
func (c *Cmd) String() string {
	b := new(strings.Builder)
	b.WriteString(c.Cmd.String())

	if c.Next != nil {
		b.WriteString(" | ")
		b.WriteString(c.Next.String())
	}

	return b.String()
}

// Start starts the specified command but does not wait for it to complete.
//
// If Start returns successfully, the c.Process field will be set.
//
// After a successful call to Start the Wait method must be called in order to release associated system resources.
func (c *Cmd) Start() error {
	if c.Process != nil {
		return errors.New("exec: already started") //nolint: goerr113
	}

	ctx, span := c.tracer.Start(c.ctx, "exec:run",
		trace.WithAttributes(
			attribute.StringSlice("exec.args", c.Args),
		),
	)

	sc := span.SpanContext()

	if c.Cmd.Stderr == nil {
		c.Cmd.Stderr = c.stdErr
	} else {
		c.Cmd.Stderr = io.MultiWriter(c.stdErr, c.Cmd.Stderr)
	}

	c.ctx = ctx
	c.Cmd.Env = append(c.Cmd.Env,
		fmt.Sprintf("TRACE_ID=%s", sc.TraceID().String()),
		fmt.Sprintf("SPAN_ID=%s", sc.SpanID().String()),
	)

	if c.Next != nil {
		c.Next.ctx = ctx
	}

	if err := c.Cmd.Start(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()

		return err //nolint: wrapcheck
	}

	span.SetStatus(codes.Ok, "")

	return nil
}

// Wait waits for the command to exit and waits for any copying to
// stdin or copying from stdout or stderr to complete.
//
// The command must have been started by Start.
//
// The returned error is nil if the command runs, has no problems
// copying stdin, stdout, and stderr, and exits with a zero exit
// status.
//
// If the command fails to run or doesn't complete successfully, the
// error is of type *ExitError. Other error types may be
// returned for I/O problems.
//
// If any of c.Stdin, c.Stdout or c.Stderr are not an *os.File, Wait also waits
// for the respective I/O loop copying to or from the process to complete.
//
// Wait releases any resources associated with the Cmd.
func (c *Cmd) Wait() (err error) {
	if c.Process == nil {
		return errors.New("exec: not started") //nolint: goerr113
	}

	if c.ProcessState != nil {
		return errors.New("exec: Wait was already called") //nolint: goerr113
	}

	span := trace.SpanFromContext(c.ctx)
	defer func() {
		span.SetAttributes(
			attribute.Int("exec.exit_code", c.ProcessState.ExitCode()),
		)

		if err == nil {
			span.SetStatus(codes.Ok, "")
		} else {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}

		span.End()
	}()

	if c.Next != nil {
		if err = c.Next.Start(); err != nil {
			span.End()

			return err
		}

		defer func() {
			if err == nil {
				err = c.Next.Wait()
			} else {
				nextSpan := trace.SpanFromContext(c.Next.ctx)

				nextSpan.SetStatus(codes.Error, fmt.Sprintf("`%s` exited with code %d", c.Cmd.String(), c.ProcessState.ExitCode()))
				nextSpan.End()
			}
		}()
	}

	defer c.closer.Close() //nolint: errcheck

	if err = c.Cmd.Wait(); err != nil {
		out := strings.Trim(c.stdErr.String(), "\r\n ")

		c.logger.Debug(c.ctx, fmt.Sprintf("failed to execute `%s`", filepath.Base(c.Path)),
			"exec.error", err,
			"exec.exit_code", c.ProcessState.ExitCode(),
			"exec.command", c.Cmd.String(),
			"exec.output", out,
		)
	}

	return err
}

// Run starts the specified command and waits for it to complete.
//
// The returned error is nil if the command runs, has no problems copying stdin, stdout, and stderr, and exits with a
// zero exit status.
//
// If the command starts but does not complete successfully, the error is of type *ExitError. Other error types may be
// returned for other situations.
//
// If the calling goroutine has locked the operating system thread
// with runtime.LockOSThread and modified any inheritable OS-level
// thread state (for example, Linux or Plan 9 name spaces), the new
// process will inherit the caller's thread state.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}

	return c.Wait()
}

// Command returns the Cmd struct to execute the named program with the given arguments.
//
// See os/exec.Command for more information.
func Command(name string, opts ...Option) *Cmd {
	return CommandContext(context.Background(), name, opts...)
}

// CommandContext is like Command but includes a context.
//
// See os/exec.CommandContext for more information.
func CommandContext(ctx context.Context, name string, opts ...Option) *Cmd {
	c := &Cmd{
		Cmd: exec.CommandContext(ctx, filepath.Clean(name)), //nolint: gosec

		ctx:    ctx,
		stdErr: new(bytes.Buffer),
		tracer: trace.NewNoopTracerProvider().Tracer(""),
		logger: ctxd.NoOpLogger{},
		closer: io.NopCloser(nil),
	}

	c.Cmd.Env = os.Environ()

	for _, opt := range opts {
		opt.applyOption(c)
	}

	c.Err = setupCmd(c)

	return c
}

// Run runs the command.
//
// See os/exec.Command.Run for more information.
func Run(name string, opts ...Option) (*Cmd, error) {
	return RunWithContext(context.Background(), name, opts...)
}

// RunWithContext runs the command with the given context.
//
// See os/exec.Command.Run for more information.
func RunWithContext(ctx context.Context, name string, opts ...Option) (_ *Cmd, err error) {
	cmd := CommandContext(ctx, name, opts...)
	if cmd.Err != nil {
		cmd.logger.Debug(ctx, cmd.Err.Error())

		return cmd, cmd.Err
	}

	return cmd, cmd.Run()
}

func setupCmd(cmd *Cmd) error {
	if cmd.Err != nil {
		cmd.logger.Debug(cmd.ctx, fmt.Sprintf("%s not found", filepath.Base(cmd.Path)))

		return cmd.Err
	}

	if cmd.Next != nil {
		if cmd.Next.Err == nil {
			pIn, pOut := io.Pipe()

			cmd.Next.Stdout = cmd.Stdout
			cmd.Next.Stdin = pIn
			cmd.Next.Stderr = cmd.Stderr
			cmd.Next.Env = cmd.Env
			cmd.Next.tracer = cmd.tracer
			cmd.Next.logger = cmd.logger

			cmd.Stdout = pOut
			cmd.closer = pOut
		}

		return setupCmd(cmd.Next)
	}

	return nil
}

// Option is an option to configure the Cmd.
type Option interface {
	applyOption(c *Cmd)
}

type optionFunc func(c *Cmd)

func (f optionFunc) applyOption(c *Cmd) {
	f(c)
}

// Pipe pipes the output to the next command.
func Pipe(name string, args ...string) Option {
	return optionFunc(func(c *Cmd) {
		if c.Next == nil {
			c.Next = CommandContext(c.ctx, name, WithArgs(args...)) //nolint: gosec
		} else {
			Pipe(name, args...).applyOption(c.Next)
		}
	})
}

// WithArgs sets the arguments.
func WithArgs(args ...string) Option {
	return optionFunc(func(c *Cmd) {
		c.Args = append([]string{c.Path}, args...)
	})
}

// WithEnv sets the environment variable.
func WithEnv(key, value string) Option {
	return optionFunc(func(c *Cmd) {
		c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
	})
}

// WithEnvs sets the environment variables.
func WithEnvs(envs map[string]string) Option {
	return optionFunc(func(c *Cmd) {
		for key, value := range envs {
			c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
		}
	})
}

// WithStdin sets the standard input.
func WithStdin(in io.Reader) Option {
	return optionFunc(func(c *Cmd) {
		c.Stdin = in
	})
}

// WithStdout sets the standard output.
func WithStdout(out io.Writer) Option {
	return optionFunc(func(c *Cmd) {
		c.Stdout = out
	})
}

// WithStderr sets the standard error.
func WithStderr(err io.Writer) Option {
	return optionFunc(func(c *Cmd) {
		c.Stderr = err
	})
}

// WithTracer sets the tracer.
func WithTracer(tracer trace.Tracer) Option {
	return optionFunc(func(c *Cmd) {
		c.tracer = tracer
	})
}

// WithLogger sets the logger.
func WithLogger(logger ctxd.Logger) Option {
	return optionFunc(func(c *Cmd) {
		c.logger = logger
	})
}

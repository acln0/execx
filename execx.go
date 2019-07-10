// Copyright 2019 Andrei Tudor Călin
//
// Permission to use, copy, modify, and/or distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

// Package execx provides extensions to os/exec, for the purpose of collecting
// richer exit errors.
package execx

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"acln.ro/env"
)

// Cmdline returns an approximation of the command line invocation equivalent
// to cmd. The returned string is the concatenation of filepath.Base(cmd.Path)
// and cmd.Args, separated by spaces.
//
// Note that Cmdline does not produce shell-safe output, and does not account
// for environment variables. Cmdline should be used for strictly informative
// purposes, such as logging or debugging.
func Cmdline(cmd *exec.Cmd) string {
	return cmdline(cmd.Path, cmd.Args)
}

// Wrap wraps an *exec.ExitError in a *ExitError, decorating it with
// additional details about the command. For convenience, Wrap also makes
// the following decisions:
//
// If err is nil, Wrap returns nil.
//
// If err is not of type *exec.ExitError, it is returned unchanged.
//
// If err is of type *exec.ExitError, but did not originate from cmd, it is
// returned unchanged.
func Wrap(err error, cmd *exec.Cmd) error {
	if err == nil {
		return nil
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		return err
	}
	if ee.ProcessState != cmd.ProcessState {
		return ee
	}
	newee := &ExitError{
		ExitError: ee,
		Path:      cmd.Path,
		Args:      cmd.Args,
		Dir:       cmd.Dir,
		ParentEnv: env.Variables(),
	}
	if newee.Dir == "" {
		wd, err := os.Getwd()
		if err == nil {
			newee.Dir = wd
		}
	}
	if cmd.Env == nil {
		newee.ChildEnv = newee.ParentEnv
	} else {
		newee.ChildEnv = env.Parse(cmd.Env...)
	}
	return newee
}

// ExitError wraps an *os/exec.ExitError with additional details.
type ExitError struct {
	// The original ExitError returned by os/exec.
	*exec.ExitError

	// Path is the path of the command which was executed.
	Path string

	// Args holds command line arguments.
	Args []string

	// Dir holds the working directory for the child process.
	Dir string

	// ParentEnv is the environment of the parent process.
	ParentEnv env.Map

	// ChildEnv is the environment of the child process.
	ChildEnv env.Map
}

// Cmdline returns the concatenation of filepath.Base(e.Path) and e.Args,
// separated by spaces. See func Cmdline.
func (e *ExitError) Cmdline() string {
	return cmdline(e.Path, e.Args)
}

// Unwrap returns e.ExitError.
func (e *ExitError) Unwrap() error {
	return e.ExitError
}

// Error returns e.ExitError.Error().
func (e *ExitError) Error() string {
	return e.ExitError.Error()
}

// Format implements fmt.Formatter for *ExitError as follows:
//
// If the verb is anything other than 'v', Format emits no output.
//
// For "%v", Format emits e.Cmdline(), the exit status of the process,
// and standard error output, if it was captured.
//
// For "%+v", Format emits everything "%v" emits, and some additional details
// about the child process: its working directory, its user and system CPU
// time, its environment, etc.
func (e *ExitError) Format(s fmt.State, verb rune) {
	if verb != 'v' {
		return
	}
	if s.Flag('+') {
		e.formatDetail(s)
	} else {
		e.formatBasic(s)
	}
}

func (e *ExitError) formatDetail(w io.Writer) {
	e.formatBasic(w)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "workdir: %s\n", e.Dir)
	fmt.Fprintf(w, "user time: %v\n", e.UserTime())
	fmt.Fprintf(w, "system time: %v\n", e.SystemTime())
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%+v", e.ChildEnv)
}

func (e *ExitError) formatBasic(w io.Writer) {
	fmt.Fprintf(w, "%s: %s", e.Cmdline(), e.ExitError.Error())
	if e.ExitError.Stderr != nil {
		fmt.Fprintf(w, ": %s", e.ExitError.Stderr)
	}
}

func cmdline(path string, args []string) string {
	var cmdline []string
	cmdline = append(cmdline, filepath.Base(path))
	cmdline = append(cmdline, args[1:]...)
	return strings.Join(cmdline, " ")
}

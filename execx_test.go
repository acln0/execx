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

package execx_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"acln.ro/env"
	"acln.ro/execx"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestMain(m *testing.M) {
	if os.Getenv("EXECX_TEST") == "on" {
		os.Stderr.WriteString("whoops")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestCmdline(t *testing.T) {
	cmd := exec.Command("git", "fetch", "something")
	got := execx.Cmdline(cmd)
	want := "git fetch something"
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatal(diff)
	}
}

var ignoreExitError = cmpopts.IgnoreFields(execx.ExitError{}, "ExitError")

const timeout = 100 * time.Millisecond

func TestWrap(t *testing.T) {
	t.Run("nil", testWrapNil)
	t.Run("NotExitError", testWrapNotExitError)
	t.Run("AnotherError", testWrapAnotherError)
	t.Run("WithParentEnv", testWrapWithParentEnv)
	t.Run("WithCustomEnv", testWrapWithCustomEnv)
}

func testWrapNil(t *testing.T) {
	err := execx.Wrap(nil, nil)
	if err != nil {
		t.Fatalf("wrapped nil error in non-nil error")
	}
}

func testWrapNotExitError(t *testing.T) {
	sentinel := errors.New("test")
	err := execx.Wrap(sentinel, nil)
	if err != sentinel {
		t.Fatalf("didn't return the same error")
	}
}

func testWrapAnotherError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	self1 := exec.CommandContext(ctx, os.Args[0])
	self2 := exec.CommandContext(ctx, os.Args[0])

	err1 := self1.Run()
	err2 := self2.Run()

	if err := execx.Wrap(err1, self2); err != err1 {
		t.Fatalf("didn't return the same error")
	}
	if err := execx.Wrap(err2, self1); err != err2 {
		t.Fatalf("didn't return the same error")
	}
}

func testWrapWithParentEnv(t *testing.T) {
	os.Setenv("EXECX_TEST", "on")
	defer os.Unsetenv("EXECX_TEST")

	parentEnv := env.Variables()
	childEnv := env.Variables()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	self := exec.CommandContext(ctx, os.Args[0])

	want := &execx.ExitError{
		Path:      self.Path,
		Args:      self.Args,
		Dir:       mustGetwd(t),
		ParentEnv: parentEnv,
		ChildEnv:  childEnv,
	}
	_, err := self.Output()
	checkExitError(t, err, self, want)
}

func testWrapWithCustomEnv(t *testing.T) {
	err, self := execSelf()
	err = execx.Wrap(err, self)
	want := &execx.ExitError{
		Path:      self.Path,
		Args:      self.Args,
		Dir:       mustGetwd(t),
		ParentEnv: err.(*execx.ExitError).ParentEnv,
		ChildEnv:  err.(*execx.ExitError).ChildEnv,
	}
	checkExitError(t, err, self, want)
}

func checkExitError(t *testing.T, err error, cmd *exec.Cmd, want *execx.ExitError) {
	t.Helper()

	if err == nil {
		t.Fatal("didn't get nil error")
	}

	err = execx.Wrap(err, cmd)
	ee, ok := err.(*execx.ExitError)
	if !ok {
		t.Fatalf("got %T, want %T", err, (*execx.ExitError)(nil))
	}

	if diff := cmp.Diff(ee, want, ignoreExitError); diff != "" {
		t.Fatalf(diff)
	}
}

func TestExitError(t *testing.T) {
	t.Run("ErrorMethod", testExitErrorErrorMethod)
	t.Run("Print", testExitErrorPrint)
	t.Run("Unwrap", testExitErrorUnwrap)
}

func testExitErrorErrorMethod(t *testing.T) {
	err, self := execSelf()
	want := err.Error()
	err = execx.Wrap(err, self)
	got := err.Error()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func testExitErrorPrint(t *testing.T) {
	t.Run("Basic", testExitErrorPrintBasic)
	t.Run("Detail", testExitErrorPrintDetail)
	t.Run("BadVerb", testExitErrorPrintBadVerb)
}

func testExitErrorPrintBasic(t *testing.T) {
	got := fmt.Sprintf("%v", execx.Wrap(execSelf()))
	if !strings.Contains(got, "execx.test: exit status 1") {
		t.Errorf("error message doesn't contain command line and exit status")
	}
}

func testExitErrorPrintDetail(t *testing.T) {
	err := execx.Wrap(execSelf())
	got := fmt.Sprintf("%+v", err)
	if !strings.Contains(got, fmt.Sprintf("%+v", err.(*execx.ExitError).ChildEnv)) {
		t.Errorf("error message doesn't contain environment")
	}
}

func testExitErrorPrintBadVerb(t *testing.T) {
	got := fmt.Sprintf("%d", execx.Wrap(execSelf()))
	if got != "" {
		t.Errorf("emitted output on bad flag")
	}
}

func testExitErrorUnwrap(t *testing.T) {
	parentEnv := env.Variables()
	childEnv := env.Merge(parentEnv, env.Map{"EXECX_TEST": "on"})

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	self := exec.CommandContext(ctx, os.Args[0])
	self.Env = childEnv.Encode()

	_, err := self.Output()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("got %T, want %T", err, (*exec.ExitError)(nil))
	}

	err = execx.Wrap(err, self)
	u, ok := err.(interface{ Unwrap() error })
	if !ok {
		t.Fatalf("wrapped error does not have an Unwrap() error method")
	}
	if u.Unwrap() != ee {
		t.Fatalf("unwrapped wrong error")
	}
}

func execSelf() (error, *exec.Cmd) {
	parentEnv := env.Variables()
	childEnv := env.Merge(parentEnv, env.Map{"EXECX_TEST": "on"})

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	self := exec.CommandContext(ctx, os.Args[0])
	self.Env = childEnv.Encode()

	_, err := self.Output()
	return err, self
}

func mustGetwd(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

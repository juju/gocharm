package errors_test

import (
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"
	"testing"

	"launchpad.net/errgo/errors"
)

var (
	_ errors.Causer     = (*errors.Err)(nil)
	_ errors.Locationer = (*errors.Err)(nil)
	_ errors.Diagnoser  = (*errors.Err)(nil)
)

func TestNew(t *testing.T) {
	err := errors.New("foo") //err TestNew
	checkErr(t, err, nil, "foo", "[{$TestNew$: foo}]", err)
}

func TestNewf(t *testing.T) {
	err := errors.Newf("foo %d", 5) //err TestNewf
	checkErr(t, err, nil, "foo 5", "[{$TestNewf$: foo 5}]", err)
}

var someErr = errors.New("some error")

func TestWrap(t *testing.T) {
	err0 := errors.WithDiagnosisf(someErr, nil, "foo") //err TestWrap#0
	err := errors.Wrap(err0)                           //err TestWrap#1
	checkErr(t, err, err0, "foo", "[{$TestWrap#1$: } {$TestWrap#0$: foo}]", err)

	err = errors.Wrap(nil)
	if err != nil {
		t.Fatalf("expected nil got %#v", err)
	}
}

func TestWrapf(t *testing.T) {
	err0 := errors.WithDiagnosisf(someErr, nil, "foo") //err TestWrapf#0
	err := errors.Wrapf(err0, "bar")                   //err TestWrapf#1
	checkErr(t, err, err0, "bar: foo", "[{$TestWrapf#1$: bar} {$TestWrapf#0$: foo}]", err)

	err = errors.Wrapf(nil, "bar")
	if err != nil {
		t.Fatalf("expected nil got %#v", err)
	}
}

func TestWrapFunc(t *testing.T) {
	err0 := errors.New("zero")
	err1 := errors.New("one")

	allowVals := func(vals ...error) (r []func(error) bool) {
		for _, val := range vals {
			r = append(r, errors.Is(val))
		}
		return
	}
	tests := []struct {
		err       error
		allow0    []func(error) bool
		allow1    []func(error) bool
		diagnosis error
	}{{
		err:       err0,
		allow0:    allowVals(err0),
		diagnosis: err0,
	}, {
		err:       err1,
		allow0:    allowVals(err0),
		diagnosis: nil,
	}, {
		err:       err0,
		allow1:    allowVals(err0),
		diagnosis: err0,
	}, {
		err:       err0,
		allow0:    allowVals(err1),
		allow1:    allowVals(err0),
		diagnosis: err0,
	}, {
		err:       err0,
		allow0:    allowVals(err0, err1),
		diagnosis: err0,
	}, {
		err:       err1,
		allow0:    allowVals(err0, err1),
		diagnosis: err1,
	}, {
		err:       err0,
		allow1:    allowVals(err0, err1),
		diagnosis: err0,
	}, {
		err:       err1,
		allow1:    allowVals(err0, err1),
		diagnosis: err1,
	}}
	for i, test := range tests {
		wrap := errors.WrapFunc(test.allow0...)
		err := wrap(test.err, test.allow1...)
		diag := errors.Diagnosis(err)
		wantDiag := test.diagnosis
		if wantDiag == nil {
			wantDiag = err
		}
		if diag != wantDiag {
			t.Errorf("test %d. got %#v want %#v", i, diag, err)
		}
	}
}

type embed struct {
	*errors.Err
}

func TestDiagnosis(t *testing.T) {
	if diag := errors.Diagnosis(someErr); diag != someErr {
		t.Fatalf("expected %q kind; got %#v", someErr, diag)
	}
	otherErr := errors.New("other error")
	causeErr := errors.New("cause error")                          //err TestDiagnosis#1
	err := errors.WithDiagnosisf(otherErr, causeErr, "foo %d", 99) //err TestDiagnosis#2
	if errors.Diagnosis(err) != otherErr {
		t.Fatalf("expected %q; got %#v", otherErr, errors.Diagnosis(err))
	}
	checkErr(t, err, causeErr, "foo 99: cause error", "[{$TestDiagnosis#2$: foo 99} {$TestDiagnosis#1$: cause error}]", otherErr)
	err = &embed{err.(*errors.Err)}
	if errors.Diagnosis(err) != otherErr {
		t.Fatalf("expected %q; got %#v", otherErr, errors.Diagnosis(err))
	}
}

func TestInfo(t *testing.T) {
	if info := errors.Info(nil); info != "[]" {
		t.Fatalf("errors.Info(nil) got %q want %q", info, "[]")
	}

	otherErr := fmt.Errorf("other")
	checkErr(t, otherErr, nil, "other", "[{other}]", otherErr)

	err0 := &embed{errors.New("foo").(*errors.Err)} //err TestStack#0
	checkErr(t, err0, nil, "foo", "[{$TestStack#0$: foo}]", err0)

	err1 := &embed{errors.Wrapf(err0, "bar").(*errors.Err)} //err TestStack#1
	checkErr(t, err1, err0, "bar: foo", "[{$TestStack#1$: bar} {$TestStack#0$: foo}]", err1)

	err2 := errors.Wrap(err1) //err TestStack#2
	checkErr(t, err2, err1, "bar: foo", "[{$TestStack#2$: } {$TestStack#1$: bar} {$TestStack#0$: foo}]", err2)
}

func TestMatch(t *testing.T) {
	type errTest func(error) bool
	allow := func(ss ...string) []func(error) bool {
		fns := make([]func(error) bool, len(ss))
		for i, s := range ss {
			s := s
			fns[i] = func(err error) bool {
				return err != nil && err.Error() == s
			}
		}
		return fns
	}
	tests := []struct {
		err error
		fns []func(error) bool
		ok  bool
	}{{
		err: errors.New("foo"),
		fns: allow("foo"),
		ok:  true,
	}, {
		err: errors.New("foo"),
		fns: allow("bar"),
		ok:  false,
	}, {
		err: errors.New("foo"),
		fns: allow("bar", "foo"),
		ok:  true,
	}, {
		err: errors.New("foo"),
		fns: nil,
		ok:  false,
	}, {
		err: nil,
		fns: nil,
		ok:  false,
	}}

	for i, test := range tests {
		ok := errors.Match(test.err, test.fns...)
		if ok != test.ok {
			t.Fatalf("test %d: expected %v got %v", i, test.ok, ok)
		}
	}
}

func TestLocation(t *testing.T) {
	loc := errors.Location{"foo", 35}
	if loc.String() != "foo:35" {
		t.Fatalf("expected \"foo:35\" got %q", loc.String)
	}
}

func checkErr(t *testing.T, err, cause error, msg string, info string, diag error) {
	if err.Error() != msg {
		t.Fatalf("unexpected message: want %q; got %q", msg, err.Error())
	}
	if errors.Cause(err) != cause {
		t.Fatalf("unexpected cause: want %q; got %v", cause, errors.Cause(err))
	}
	if errors.Diagnosis(err) != diag {
		t.Fatalf("unexpected diagnosis: want %#v; got %#v", diag, errors.Diagnosis(err))
	}
	wantInfo := replaceLocations(info)
	if gotInfo := errors.Info(err); gotInfo != wantInfo {
		t.Fatalf("unexpected info: want %q; got %q", wantInfo, gotInfo)
	}
}

func replaceLocations(s string) string {
	t := ""
	for {
		i := strings.Index(s, "$")
		if i == -1 {
			break
		}
		t += s[0:i]
		s = s[i+1:]
		i = strings.Index(s, "$")
		if i == -1 {
			panic("no second $")
		}
		t += location(s[0:i]).String()
		s = s[i+1:]
	}
	t += s
	return t
}

func location(tag string) errors.Location {
	line, ok := tagToLine[tag]
	if !ok {
		panic(fmt.Errorf("tag %q not found", tag))
	}
	return errors.Location{
		File: filename,
		Line: line,
	}
}

var tagToLine = make(map[string]int)
var filename string

func init() {
	data, err := ioutil.ReadFile("errors_test.go")
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if j := strings.Index(line, "//err "); j >= 0 {
			tagToLine[line[j+len("//err "):]] = i + 1
		}
	}
	_, filename, _, _ = runtime.Caller(0)
}

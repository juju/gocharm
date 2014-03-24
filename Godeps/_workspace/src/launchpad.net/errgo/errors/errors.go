// The errors package provides a way to create
// and diagnose errors. It is compatible with
// the usual Go error idioms but adds a way to wrap errors
// so that they record source location information
// while retaining a consistent way for code to
// inspect errors to find out particular problems
// with an emphasis on increasing code maintainability.
//
package errors

import (
	"bytes"
	"fmt"
	"runtime"

	"github.com/loggo/loggo"
)

const debug = false

var logger loggo.Logger

func init() {
	if debug {
		logger = loggo.GetLogger("juju.errgo.errors")
	}
}

// Location describes a source code location.
type Location struct {
	File string
	Line int
}

// String returns a location in filename.go:99 format.
func (loc Location) String() string {
	return fmt.Sprintf("%s:%d", loc.File, loc.Line)
}

// IsSet reports whether the location has been set.
func (loc Location) IsSet() bool {
	return loc.File != ""
}

// Err holds a description of an error along with information about
// where the error was created.
type Err struct {
	// Message_ holds the text of the error message.
	// It may be empty if Cause is set.
	Message_ string

	// Diagnosis_ holds the diagnosis of the error by the
	// one that created it. It should be nil to signify
	// no diagnosis, which will cause Diagnosis to return
	// the error itself.
	Diagnosis_ error

	// Cause optionally holds the underlying cause of the error.
	Cause_ error

	// Location holds the source code location
	// where the error was created.
	Location_ Location
}

func (e *Err) Location() Location {
	return e.Location_
}

// Cause returns the cause of the error, if any.
func (e *Err) Cause() error {
	return e.Cause_
}

// Message returns the top level error message.
func (e *Err) Message() string {
	return e.Message_
}

// Error implements error.Error.
func (e *Err) Error() string {
	switch {
	case e.Message_ == "" && e.Cause == nil:
		return "<no error>"
	case e.Message_ == "":
		return e.Cause_.Error()
	case e.Cause_ == nil:
		return e.Message_
	}
	return fmt.Sprintf("%s: %v", e.Message_, e.Cause_)
}

func (e *Err) GoString() string {
	return Info(e)
}

// Diagnosis implements Diagnoser.
func (e *Err) Diagnosis() error {
	return e.Diagnosis_
}

// Diagnoser is the type of an error that may provide
// an error diagnosis. It may return nil if there is
// no diagnosis.
type Diagnoser interface {
	Diagnosis() error
}

// Causer is the type of an error that may have
// an underlying error that caused it.
// TODO(rog) choose a better name for this interface.
type Causer interface {
	// Message returns the top level error message,
	// not including the cause.
	Message() string
	// Cause returns the underlying error, or nil
	// if there is none.
	Cause() error
}

// Location can be implemented by any error type
// that wants to expose the source location of an error.
type Locationer interface {
	Location() Location
}

// Info returns information about the causes of the error,
// in the format:
// [{filename:99: error one} {otherfile:55: cause of error one}]
func Info(err error) string {
	if err == nil {
		return "[]"
	}
	var s []byte
	s = append(s, '[')
	for {
		s = append(s, '{')
		if err, ok := err.(Locationer); ok {
			loc := err.Location()
			if loc.IsSet() {
				s = append(s, loc.String()...)
				s = append(s, ": "...)
			}
		}
		if cerr, ok := err.(Causer); ok {
			s = append(s, cerr.Message()...)
			err = cerr.Cause()
		} else {
			s = append(s, err.Error()...)
			err = nil
		}
		if debug {
			if diagErr, ok := err.(Diagnoser); ok {
				if diag := diagErr.Diagnosis(); diag != nil {
					s = append(s, fmt.Sprintf("=%T", diag)...)
					s = append(s, Info(diag)...)
				}
			}
		}
		s = append(s, '}')
		if err == nil {
			break
		}
		s = append(s, ' ')
	}
	s = append(s, ']')
	return string(s)
}

// Locate records the source location of the error by setting
// e.Location, at callDepth stack frames above the call.
func (e *Err) SetLocation(callDepth int) {
	_, file, line, _ := runtime.Caller(callDepth + 1)
	e.Location_ = Location{file, line}
}

func setLocation(err error, callDepth int) {
	if e, _ := err.(*Err); e != nil {
		e.SetLocation(callDepth + 1)
	}
}

// New returns a new error with the given error message
// and no diagnosis.
func New(s string) error {
	err := &Err{Message_: s}
	err.SetLocation(1)
	return err
}

// Newf returns a new error with the given printf-formatted error
// message and no diagnosis.
func Newf(f string, a ...interface{}) error {
	err := &Err{Message_: fmt.Sprintf(f, a...)}
	err.SetLocation(1)
	return err
}

// Match returns whether any of the given
// functions returns true when called with err as an
// argument. If no functions are specified,
// Match returns false.
func Match(err error, allow ...func(error) bool) bool {
	for _, f := range allow {
		if f(err) {
			return true
		}
	}
	return false
}

// Is returns a function that returns whether the
// an error is equal to the given error.
// It is intended to be used as a "allow" argument
// to Wrap and friends; for example:
//
// 	return errors.Wrap(err, errors.Is(http.ErrNoCookie))
//
// would return an error with an http.ErrNoCookie diagnosis
// only if that was err's diagnosis; otherwise the diagnosis
// would be itself.
func Is(err error) func(error) bool {
	return func(err1 error) bool {
		return err == err1
	}
}

// Any returns true. It can be used as an argument to Wrap
// to allow any diagnosis to pass through to the wrapped
// error.
func Any(error) bool {
	return true
}

// WrapMsg returns an Err that has the given cause,
// adding the given message as context, and allowing
// the given diagnoses to be retained in the returned error
// (see Wrap for an explanation of the allow parameter)
//
// If err is nil, WrapMsg returns nil.
func WrapMsg(cause error, msg string, allow ...func(error) bool) error {
	if cause == nil {
		return nil
	}
	newErr := &Err{
		Cause_:   cause,
		Message_: msg,
	}
	if len(allow) > 0 {
		if diag := Diagnosis(cause); Match(diag, allow...) {
			newErr.Diagnosis_ = diag
		}
	}
	if debug {
		if newd, oldd := newErr.Diagnosis_, Diagnosis(cause); newd != oldd {
			logger.Infof("Wrap diagnosis %[1]T(%[1]v)->%[2]T(%[2]v)", oldd, newd)
			logger.Infof("call stack: %s", callers(0, 20))
			logger.Infof("len(allow) == %d", len(allow))
			logger.Infof("old error %#v", cause)
			logger.Infof("new error %#v", newErr)
		}
	}
	return newErr
}

// Wrap returns an Err that wraps the given error.  The error message
// is unchanged, but the error stack records the caller of Wrap.  If err
// is nil, Wrap returns nil.
//
// If Diagnosis(err) matches one of the given allow arguments
// (see Match) the returned error will hold that diagnosis,
// otherwise the result will have no diagnosis.
//
// For example, the following code will return an error
// whose diagnosis is the error from the os.Open call
// when the file does not exist.
//
//	f, err := os.Open("non-existent-file")
//	if err != nil {
//		return errors.Wrap(err, os.IsNotExist)
//	}
//
func Wrap(err error, allow ...func(error) bool) error {
	err = WrapMsg(err, "", allow...)
	setLocation(err, 1)
	return err
}

// Wrapf returns an Error that has the given and adds the given
// formatted context message.  The diagnosis of the new error is itself.
// If err is nil, Wrapf returns nil.
func Wrapf(cause error, f string, a ...interface{}) error {
	err := WrapMsg(cause, fmt.Sprintf(f, a...))
	setLocation(err, 1)
	return err
}

// WrapFunc returns an equivalent of Wrap that
// always allows the specified diagnoses in addition
// to any passed to the returned function.
func WrapFunc(allow ...func(error) bool) func(error, ...func(error) bool) error {
	return func(err error, allow1 ...func(error) bool) error {
		var allowEither []func(error) bool
		if len(allow1) > 0 {
			// This is more efficient than using a function literal,
			// because the compiler knows that it doesn't escape.
			allowEither = make([]func(error) bool, len(allow)+len(allow1))
			copy(allowEither, allow)
			copy(allowEither[len(allow):], allow1)
		} else {
			allowEither = allow
		}
		err = Wrap(err, allowEither...)
		setLocation(err, 1)
		return err
	}
}

// WithDiagnosisf returns a new Error with the given
// diagnosis and underlying possibly nil cause. The returned
// error will have the given formatted message context.
func WithDiagnosisf(diag, cause error, f string, a ...interface{}) error {
	err := &Err{
		Diagnosis_: diag,
		Message_:   fmt.Sprintf(f, a...),
		Cause_:     cause,
	}
	err.SetLocation(1)
	return err
}

// Diagnosis returns the diagnosis of the given error.  If err does not
// implement Diagnoser or has no diagnosis, it returns err itself.
func Diagnosis(err error) error {
	var diag error
	if err, ok := err.(Diagnoser); ok {
		diag = err.Diagnosis()
	}
	if diag != nil {
		return diag
	}
	return err
}

// Cause returns the cause of the given error. If err does not
// implement Causer, it returns nil.
func Cause(err error) error {
	if cerr, ok := err.(Causer); ok {
		return cerr.Cause()
	}
	return nil
}

// callers returns the stack trace of the goroutine that called it,
// starting n entries above the caller of callers, as a space-separated list
// of filename:line-number pairs with no new lines.
func callers(n, max int) []byte {
	var b bytes.Buffer
	prev := false
	for i := 0; i < max; i++ {
		_, file, line, ok := runtime.Caller(n + 1)
		if !ok {
			return b.Bytes()
		}
		if prev {
			fmt.Fprintf(&b, " ")
		}
		fmt.Fprintf(&b, "%s:%d", file, line)
		n++
		prev = true
	}
	return b.Bytes()
}

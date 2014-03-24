// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package logging

import (
    "log"
    "os"
)

const (
    DEBUG = 10 * (iota + 1)
    INFO
    WARN
    ERROR
)

var level = WARN

func init() {
    setLevel(os.Getenv("LOGLEVEL"))
}

func setLevel(levelName string) {
    switch levelName {
    case "DEBUG":
        level = DEBUG
    case "INFO":
        level = INFO
    case "WARN":
        level = WARN
    case "ERROR":
        level = ERROR
    }
}

func Debug(args ...interface{}) {
    if level <= DEBUG {
        log.Println(args...)
    }
}

func Debugf(format string, args ...interface{}) {
    if level <= DEBUG {
        log.Printf(format, args...)
    }
}

func Info(args ...interface{}) {
    if level <= INFO {
        log.Println(args...)
    }
}

func Infof(format string, args ...interface{}) {
    if level <= INFO {
        log.Printf(format, args...)
    }
}

func Warn(args ...interface{}) {
    if level <= WARN {
        log.Println(args...)
    }
}

func Warnf(format string, args ...interface{}) {
    if level <= WARN {
        log.Printf(format, args...)
    }
}

func Error(args ...interface{}) {
    if level <= ERROR {
        log.Println(args...)
    }
}

func Errorf(format string, args ...interface{}) {
    if level <= ERROR {
        log.Printf(format, args...)
    }
}

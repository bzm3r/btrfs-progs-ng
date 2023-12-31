// Copyright (C) 2019-2022  Ambassador Labs
// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: Apache-2.0
//
// Contains code based on:
// https://github.com/datawire/dlib/blob/b09ab2e017e16d261f05fff5b3b860d645e774d4/dlog/logger_logrus.go
// https://github.com/datawire/dlib/blob/b09ab2e017e16d261f05fff5b3b860d645e774d4/dlog/logger_testing.go
// https://github.com/telepresenceio/telepresence/blob/ece94a40b00a90722af36b12e40f91cbecc0550c/pkg/log/formatter.go

package textui

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"git.lukeshu.com/go/typedsync"
	"github.com/datawire/dlib/dlog"
	"github.com/spf13/pflag"

	"git.lukeshu.com/btrfs-progs-ng/lib/maps"
)

type LogLevelFlag struct {
	Level dlog.LogLevel
}

var _ pflag.Value = (*LogLevelFlag)(nil)

// Type implements pflag.Value.
func (*LogLevelFlag) Type() string { return "loglevel" }

// Set implements pflag.Value.
func (lvl *LogLevelFlag) Set(str string) error {
	switch strings.ToLower(str) {
	case "error":
		lvl.Level = dlog.LogLevelError
	case "warn", "warning":
		lvl.Level = dlog.LogLevelWarn
	case "info":
		lvl.Level = dlog.LogLevelInfo
	case "debug":
		lvl.Level = dlog.LogLevelDebug
	case "trace":
		lvl.Level = dlog.LogLevelTrace
	default:
		return fmt.Errorf("invalid log level: %q", str)
	}
	return nil
}

// String implements fmt.Stringer (and pflag.Value).
func (lvl *LogLevelFlag) String() string {
	switch lvl.Level {
	case dlog.LogLevelError:
		return "error"
	case dlog.LogLevelWarn:
		return "warn"
	case dlog.LogLevelInfo:
		return "info"
	case dlog.LogLevelDebug:
		return "debug"
	case dlog.LogLevelTrace:
		return "trace"
	default:
		panic(fmt.Errorf("invalid log level: %#v", lvl.Level))
	}
}

type logger struct {
	parent *logger
	out    io.Writer
	lvl    dlog.LogLevel

	// only valid if parent is non-nil
	fieldKey string
	fieldVal any
}

var _ dlog.OptimizedLogger = (*logger)(nil)

func NewLogger(out io.Writer, lvl dlog.LogLevel) dlog.Logger {
	return &logger{
		out: out,
		lvl: lvl,
	}
}

// Helper implements dlog.Logger.
func (*logger) Helper() {}

// WithField implements dlog.Logger.
func (l *logger) WithField(key string, value any) dlog.Logger {
	return &logger{
		parent: l,
		out:    l.out,
		lvl:    l.lvl,

		fieldKey: key,
		fieldVal: value,
	}
}

type logWriter struct {
	log *logger
	lvl dlog.LogLevel
}

// Write implements io.Writer.
func (lw logWriter) Write(data []byte) (int, error) {
	lw.log.log(lw.lvl, func(w io.Writer) {
		_, _ = w.Write(data)
	})
	return len(data), nil
}

// StdLogger implements dlog.Logger.
func (l *logger) StdLogger(lvl dlog.LogLevel) *log.Logger {
	return log.New(logWriter{log: l, lvl: lvl}, "", 0)
}

// Log implements dlog.Logger.
func (*logger) Log(dlog.LogLevel, string) {
	panic("should not happen: optimized log methods should be used instead")
}

// UnformattedLog implements dlog.OptimizedLogger.
func (l *logger) UnformattedLog(lvl dlog.LogLevel, args ...any) {
	l.log(lvl, func(w io.Writer) {
		_, _ = printer.Fprint(w, args...)
	})
}

// UnformattedLogln implements dlog.OptimizedLogger.
func (l *logger) UnformattedLogln(lvl dlog.LogLevel, args ...any) {
	l.log(lvl, func(w io.Writer) {
		_, _ = printer.Fprintln(w, args...)
	})
}

// UnformattedLogf implements dlog.OptimizedLogger.
func (l *logger) UnformattedLogf(lvl dlog.LogLevel, format string, args ...any) {
	l.log(lvl, func(w io.Writer) {
		_, _ = printer.Fprintf(w, format, args...)
	})
}

var (
	logBufPool = typedsync.Pool[*bytes.Buffer]{
		New: func() *bytes.Buffer {
			return new(bytes.Buffer)
		},
	}
	logMu      sync.Mutex
	thisModDir string
)

func init() {
	//nolint:dogsled // I can't change the signature of the stdlib.
	_, file, _, _ := runtime.Caller(0)
	thisModDir = filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func (l *logger) log(lvl dlog.LogLevel, writeMsg func(io.Writer)) {
	// boilerplate /////////////////////////////////////////////////////////
	if lvl > l.lvl {
		return
	}
	logBuf, _ := logBufPool.Get()
	defer logBufPool.Put(logBuf)
	defer logBuf.Reset()

	// time ////////////////////////////////////////////////////////////////
	now := time.Now()
	const timeFmt = "15:04:05.0000"
	logBuf.WriteString(timeFmt)
	now.AppendFormat(logBuf.Bytes()[:0], timeFmt)

	// level ///////////////////////////////////////////////////////////////
	switch lvl {
	case dlog.LogLevelError:
		logBuf.WriteString(" ERR")
	case dlog.LogLevelWarn:
		logBuf.WriteString(" WRN")
	case dlog.LogLevelInfo:
		logBuf.WriteString(" INF")
	case dlog.LogLevelDebug:
		logBuf.WriteString(" DBG")
	case dlog.LogLevelTrace:
		logBuf.WriteString(" TRC")
	}

	// fields (early) //////////////////////////////////////////////////////
	fields := make(map[string]any)
	var fieldKeys []string
	for f := l; f.parent != nil; f = f.parent {
		if maps.HasKey(fields, f.fieldKey) {
			continue
		}
		fields[f.fieldKey] = f.fieldVal
		fieldKeys = append(fieldKeys, f.fieldKey)
	}
	sort.Slice(fieldKeys, func(i, j int) bool {
		iOrd := fieldOrd(fieldKeys[i])
		jOrd := fieldOrd(fieldKeys[j])
		if iOrd != jOrd {
			return iOrd < jOrd
		}
		return fieldKeys[i] < fieldKeys[j]
	})
	nextField := len(fieldKeys)
	for i, fieldKey := range fieldKeys {
		if fieldOrd(fieldKey) >= 0 {
			nextField = i
			break
		}
		writeField(logBuf, fieldKey, fields[fieldKey])
	}

	// message /////////////////////////////////////////////////////////////
	logBuf.WriteString(" : ")
	writeMsg(logBuf)

	// fields (late) ///////////////////////////////////////////////////////
	if nextField < len(fieldKeys) {
		logBuf.WriteString(" :")
	}
	for _, fieldKey := range fieldKeys[nextField:] {
		writeField(logBuf, fieldKey, fields[fieldKey])
	}

	// caller //////////////////////////////////////////////////////////////
	if lvl >= dlog.LogLevelDebug {
		const (
			thisModule             = "git.lukeshu.com/btrfs-progs-ng"
			thisPackage            = "git.lukeshu.com/btrfs-progs-ng/lib/textui"
			maximumCallerDepth int = 25
			minimumCallerDepth int = 3 // runtime.Callers + .log + .Log
		)
		var pcs [maximumCallerDepth]uintptr
		depth := runtime.Callers(minimumCallerDepth, pcs[:])
		frames := runtime.CallersFrames(pcs[:depth])
		for f, again := frames.Next(); again; f, again = frames.Next() {
			if !strings.HasPrefix(f.Function, thisModule+"/") {
				continue
			}
			if strings.HasPrefix(f.Function, thisPackage+".") {
				continue
			}
			if nextField == len(fieldKeys) {
				logBuf.WriteString(" :")
			}
			file := f.File[strings.LastIndex(f.File, thisModDir+"/")+len(thisModDir+"/"):]
			fmt.Fprintf(logBuf, " (from %s:%d)", file, f.Line)
			break
		}
	}

	// boilerplate /////////////////////////////////////////////////////////
	logBuf.WriteByte('\n')

	logMu.Lock()
	_, _ = l.out.Write(logBuf.Bytes())
	logMu.Unlock()
}

// fieldOrd returns the sort-position for a given log-field-key.  Lower return
// values should be positioned on the left when logging, and higher values
// should be positioned on the right; values <0 should be on the left of the log
// message, while values ≥0 should be on the right of the log message.
func fieldOrd(key string) int {
	switch key {
	// dlib ////////////////////////////////////////////////////////////////
	case "THREAD": // dgroup
		return -99
	case "dexec.pid":
		return -98
	case "dexec.stream":
		return -97
	case "dexec.data":
		return -96
	case "dexec.err":
		return -95

	// btrfs inspect rebuild-mappings scan /////////////////////////////////
	case "btrfs.inspect.rebuild-mappings.scan.dev":
		return -1

	// btrfs inspect rebuild-mappings process //////////////////////////////
	case "btrfs.inspect.rebuild-mappings.process.step":
		return -2
	case "btrfs.inspect.rebuild-mappings.process.substep":
		return -1

	// btrfs inspect rebuild-trees /////////////////////////////////////////
	case "btrfs.inspect.rebuild-trees.step":
		return -50
	// step=read-fs-data
	case "btrfs.inspect.rebuild-trees.read.substep":
		return -1
	// step=rebuild
	case "btrfs.inspect.rebuild-trees.rebuild.pass":
		return -49
	case "btrfs.inspect.rebuild-trees.rebuild.substep":
		return -48
	case "btrfs.inspect.rebuild-trees.rebuild.substep.progress":
		return -47
	// step=rebuild, substep=collect-items (1/3)
	// step=rebuild, substep=settle-items (2a/3)
	case "btrfs.inspect.rebuild-trees.rebuild.settle.item":
		return -25
	// step=rebuild, substep=process-items (2b/3)
	case "btrfs.inspect.rebuild-trees.rebuild.process.substep":
		return -26
	case "btrfs.inspect.rebuild-trees.rebuild.process.item":
		return -25
	// step=rebuild, substep=apply-augments (3/3)
	case "btrfs.inspect.rebuild-trees.rebuild.augment.tree":
		return -25
	// step=rebuild (any substep)
	case "btrfs.inspect.rebuild-trees.rebuild.want.key":
		return -9
	case "btrfs.inspect.rebuild-trees.rebuild.want.reason":
		return -8

	// btrfsutil.Graph /////////////////////////////////////////////////////
	case "btrfs.util.read-graph.step":
		return -1

	// btrfsutil.RebuiltForrest ////////////////////////////////////////////
	case "btrfs.util.rebuilt-forrest.add-tree":
		return -8
	case "btrfs.util.rebuilt-forrest.add-tree.want.key":
		return -7
	case "btrfs.util.rebuilt-forrest.add-tree.want.reason":
		return -6
	case "btrfs.util.rebuilt-tree.add-root":
		return -5
	case "btrfs.util.rebuilt-tree.index-errors":
		return -4
	case "btrfs.util.rebuilt-tree.index-inc-items":
		return -3
	case "btrfs.util.rebuilt-tree.index-exc-items":
		return -2
	case "btrfs.util.rebuilt-tree.index-nodes":
		return -1

	// other ///////////////////////////////////////////////////////////////
	case "btrfs.read-json-file":
		return -1
	default:
		return 1
	}
}

func writeField(w io.Writer, key string, val any) {
	valBuf, _ := logBufPool.Get()
	defer func() {
		// The wrapper `func()` is important to defer
		// evaluating `valBuf`, since we might re-assign it
		// below.
		valBuf.Reset()
		logBufPool.Put(valBuf)
	}()
	_, _ = printer.Fprint(valBuf, val)
	needsQuote := false
	if bytes.HasPrefix(valBuf.Bytes(), []byte(`"`)) {
		needsQuote = true
	} else {
		for _, r := range valBuf.Bytes() {
			if !(unicode.IsPrint(rune(r)) && r != ' ') {
				needsQuote = true
				break
			}
		}
	}
	if needsQuote {
		valBuf2, _ := logBufPool.Get()
		fmt.Fprintf(valBuf2, "%q", valBuf.Bytes())
		valBuf.Reset()
		logBufPool.Put(valBuf)
		valBuf = valBuf2
	}

	valStr := valBuf.Bytes()
	name := key

	switch {
	case name == "THREAD":
		name = "thread"
		switch {
		case len(valStr) == 0 || bytes.Equal(valStr, []byte("/main")):
			return
		default:
			if bytes.HasPrefix(valStr, []byte("/main/")) {
				valStr = valStr[len("/main/"):]
			} else if bytes.HasPrefix(valStr, []byte("/")) {
				valStr = valStr[len("/"):]
			}
		}
	case strings.HasSuffix(name, ".pass"):
		fmt.Fprintf(w, "/pass-%s", valStr)
		return
	case strings.HasSuffix(name, ".substep") && name != "btrfs.util.rebuilt-forrest.add-tree.substep":
		fmt.Fprintf(w, "/%s", valStr)
		return
	case strings.HasPrefix(name, "btrfs."):
		name = strings.TrimPrefix(name, "btrfs.")
		switch {
		case strings.HasPrefix(name, "inspect."):
			name = strings.TrimPrefix(name, "inspect.")
			switch {
			case strings.HasPrefix(name, "rebuild-mappings."):
				name = strings.TrimPrefix(name, "rebuild-mappings.")
				switch {
				case strings.HasPrefix(name, "scan."):
					name = strings.TrimPrefix(name, "scan.")
				case strings.HasPrefix(name, "process."):
					name = strings.TrimPrefix(name, "process.")
				}
			case strings.HasPrefix(name, "rebuild-trees."):
				name = strings.TrimPrefix(name, "rebuild-trees.")
				switch {
				case strings.HasPrefix(name, "read."):
					name = strings.TrimPrefix(name, "read.")
				case strings.HasPrefix(name, "rebuild."):
					name = strings.TrimPrefix(name, "rebuild.")
				}
			}
		case strings.HasPrefix(name, "util."):
			name = strings.TrimPrefix(name, "util.")
			switch {
			case strings.HasPrefix(name, "rebuilt-forrest."):
				name = strings.TrimPrefix(name, "rebuilt-forrest.")
			case strings.HasPrefix(name, "rebuilt-tree."):
				name = strings.TrimPrefix(name, "rebuilt-tree.")
			}
		}
	}

	fmt.Fprintf(w, " %s=%s", name, valStr)
}

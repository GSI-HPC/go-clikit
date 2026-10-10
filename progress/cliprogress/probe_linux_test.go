// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package cliprogress_test

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"

	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
)

// ioctl makes the ioctl req on f with the argument at arg.
func ioctl(t *testing.T, f *os.File, req uintptr, arg unsafe.Pointer) {
	t.Helper()
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), req, uintptr(arg)); errno != 0 {
		t.Fatalf("ioctl %#x: %v", req, errno)
	}
}

// A terminal says its size afresh each time it is asked, so that a window
// resized is drawn at its new width; one that is not the process's
// controlling terminal cannot say who its foreground is, and is taken to
// be the process's.
func TestATerminalSaysItsSize(t *testing.T) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer ptmx.Close()
	var unlock int32
	ioctl(t, ptmx, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock))
	var n uint32
	ioctl(t, ptmx, syscall.TIOCGPTN, unsafe.Pointer(&n))
	tty, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	for _, size := range []struct{ cols, rows uint16 }{{132, 43}, {80, 24}} {
		ws := struct{ Row, Col, Xpixel, Ypixel uint16 }{Row: size.rows, Col: size.cols}
		ioctl(t, tty, syscall.TIOCSWINSZ, unsafe.Pointer(&ws))
		cols, rows, err := cliprogress.TerminalSize(tty)
		if err != nil || cols != int(size.cols) || rows != int(size.rows) {
			t.Errorf("TerminalSize = %d, %d, %v; want %d, %d", cols, rows, err, size.cols, size.rows)
		}
	}
	if cliprogress.IsPipe(tty) || !cliprogress.InForeground(tty) {
		t.Errorf("a terminal: IsPipe %v, InForeground %v", cliprogress.IsPipe(tty), cliprogress.InForeground(tty))
	}
}

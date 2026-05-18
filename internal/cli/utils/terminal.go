package utils

import (
	"os"
	"os/exec"
	"runtime"
)

func DisableInput() {
	if runtime.GOOS == "windows" {
		exec.Command("stty", "echo off").Run()
	} else {
		exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
	}
}

func EnableInput() {
	if runtime.GOOS == "windows" {
		exec.Command("stty", "echo on").Run()
	} else {
		exec.Command("stty", "-F", "/dev/tty", "echo").Run()
	}
}

func Clear() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

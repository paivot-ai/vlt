package main

import (
	"bytes"
	"io"
	"os"
)

// captureStdout captures stdout output from a function call.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// captureStderr captures stderr output from a function call.
func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

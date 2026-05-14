// Package locks implements the file lock protocol from spec §3.6:
// O_EXCL create with a 30s stale timeout and bounded retry.
package locks

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Options tunes lock behaviour. Zero values use the spec defaults.
type Options struct {
	StaleAfter time.Duration // default 30s
	TotalWait  time.Duration // default 5s
	RetryEvery time.Duration // default 50ms
}

func (o Options) withDefaults() Options {
	if o.StaleAfter == 0 {
		o.StaleAfter = 30 * time.Second
	}
	if o.TotalWait == 0 {
		o.TotalWait = 5 * time.Second
	}
	if o.RetryEvery == 0 {
		o.RetryEvery = 50 * time.Millisecond
	}
	return o
}

// Acquire returns a release function. Caller must invoke release() exactly
// once; using defer is recommended.
func Acquire(path string, opts Options) (release func() error, err error) {
	opts = opts.withDefaults()
	deadline := time.Now().Add(opts.TotalWait)

	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			// Write metadata inside the lock for debuggability.
			host, _ := os.Hostname()
			fmt.Fprintf(f, "pid=%d host=%s ts=%s\n", os.Getpid(), host, time.Now().UTC().Format(time.RFC3339))
			f.Close()
			return func() error { return os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		// Lock present — check stale.
		if info, statErr := os.Stat(path); statErr == nil {
			if time.Since(info.ModTime()) > opts.StaleAfter {
				_ = os.Remove(path)
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("ledger busy: lock at %s held by another process", path)
		}
		time.Sleep(opts.RetryEvery)
	}
}

package legacy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/locks"
)

type ApplyOpts struct {
	ArchiveOriginals bool
	// Force suppresses the safety check that rejects shrinking ledgers.
	Force bool
	// BackupPrefix prefixes backup directory names, e.g. "legacy-to-v1-".
	BackupPrefix string
	// Clock allows tests to inject deterministic timestamps for the backup dir.
	Clock func() time.Time
}

func (o ApplyOpts) now() time.Time {
	if o.Clock != nil {
		return o.Clock()
	}
	return time.Now().UTC()
}

func Apply(plan Plan, opts ApplyOpts) error {
	if !plan.HasChanges() {
		return nil
	}
	ledgerDir := filepath.Join(plan.TargetDir, "ledger")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		return err
	}
	release, err := locks.Acquire(filepath.Join(metaDir(plan.TargetDir), "lock"), locks.Options{})
	if err != nil {
		return err
	}
	defer release()

	stamp := opts.BackupPrefix + opts.now().Format("20060102-150405")
	backupDir := filepath.Join(metaDir(plan.TargetDir), "backups", stamp)

	for _, c := range plan.Changes {
		if c.Action == ActionNoop {
			continue
		}
		full := filepath.Join(plan.TargetDir, c.OutputPath)
		if c.Action == ActionReplace {
			if err := backupFile(backupDir, plan.TargetDir, c.OutputPath); err != nil {
				return err
			}
		}
		if err := writeAtomic(full, c.NewBytes); err != nil {
			return err
		}
	}

	if err := ensureGitignore(plan.TargetDir); err != nil {
		return err
	}

	if opts.ArchiveOriginals {
		if err := archiveOriginals(plan.TargetDir); err != nil {
			return err
		}
	}
	return nil
}

func backupFile(backupDir, root, rel string) error {
	src := filepath.Join(root, rel)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	dst := filepath.Join(backupDir, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func ensureGitignore(target string) error {
	required := []string{
		".ldgr/lock",
		".ldgr/backups/",
		".ldgr/import-errors.jsonl",
		".ldgr/legacy/",
	}
	path := filepath.Join(target, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	out := existing
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	for _, line := range required {
		present := false
		for _, l := range strings.Split(existing, "\n") {
			if strings.TrimSpace(l) == line {
				present = true
				break
			}
		}
		if !present {
			out += line + "\n"
		}
	}
	if out == existing {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

func archiveOriginals(target string) error {
	legacyDir := filepath.Join(metaDir(target), "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		return err
	}
	for _, rel := range []string{"agent-tickets.jsonl", "agent-worklog.jsonl", "goal.json"} {
		src := filepath.Join(target, rel)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		dst := filepath.Join(legacyDir, rel)
		if err := os.Rename(src, dst); err != nil {
			if cpErr := copyFile(src, dst); cpErr != nil {
				return fmt.Errorf("archive %s: rename=%v copy=%v", rel, err, cpErr)
			}
			if rmErr := os.Remove(src); rmErr != nil {
				return rmErr
			}
		}
	}
	return nil
}

func metaDir(target string) string {
	return filepath.Join(target, ".ldgr")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

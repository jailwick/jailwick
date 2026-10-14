package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/paths"
)

// A start, restart or stop of a project's sandbox holds the project's lock, whichever process
// runs it (jw, the app): a second one is refused meanwhile (ErrBusy), and the app shows the one
// under way (OpOf) instead of reading running or stopped from containers that come and go. The
// lock is the operating system's, so it goes with a process that ends any way it can.

// Operations on a sandbox, as the lock records them.
const (
	OpStarting   = "starting"
	OpRestarting = "restarting"
	OpStopping   = "stopping"
)

// Op is an operation under way: what, and who runs it.
type Op struct {
	Name    string    `json:"op"`
	By      string    `json:"by"` // the program: jw, jailwick-app
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
}

// ErrBusy refuses an operation while another runs on the project.
type ErrBusy struct {
	Project string
	Op      Op
}

func (e *ErrBusy) Error() string {
	if e.Op.Name == "" {
		return fmt.Sprintf("%s is busy with another start, restart or stop", e.Project)
	}
	return fmt.Sprintf("%s is already %s (%s, pid %d)", e.Project, e.Op.Name, e.Op.By, e.Op.PID)
}

// opFiles are the project's lock (held while an operation runs) and what runs (readable while
// the lock is held: on Windows a locked file can't be read).
func opFiles(project string) (lock, info string) {
	dir := filepath.Join(paths.StateDir(), "locks")
	return filepath.Join(dir, project+".lock"), filepath.Join(dir, project+".json")
}

// lockOp takes the project's lock for op; release gives it back.
func lockOp(project, op string) (release func(), err error) {
	lock, info := opFiles(project)
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if ok, err := tryLock(f); err != nil || !ok {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		cur, _ := readOp(info)
		return nil, &ErrBusy{Project: project, Op: cur}
	}
	data, _ := json.Marshal(Op{Name: op, By: filepath.Base(os.Args[0]), PID: os.Getpid(), Started: time.Now()})
	_ = os.WriteFile(info, data, 0o644)
	return func() {
		_ = os.Remove(info)
		_ = unlock(f)
		_ = f.Close()
	}, nil
}

// OpOf returns the operation under way on project, from any process; ok is false for none.
func OpOf(project string) (op Op, ok bool) {
	lock, info := opFiles(project)
	f, err := os.OpenFile(lock, os.O_RDWR, 0)
	if err != nil {
		return Op{}, false // never locked
	}
	defer f.Close()
	if free, err := tryLock(f); err != nil || free {
		if free {
			_ = unlock(f)
		}
		return Op{}, false
	}
	op, err = readOp(info)
	return op, err == nil
}

// OpsUnderWay returns the operations under way, by project, from any process.
func OpsUnderWay() map[string]Op {
	_, info := opFiles("x")
	entries, _ := os.ReadDir(filepath.Dir(info))
	out := map[string]Op{}
	for _, e := range entries {
		if p, ok := strings.CutSuffix(e.Name(), ".json"); ok {
			if op, ok := OpOf(p); ok {
				out[p] = op
			}
		}
	}
	return out
}

func readOp(info string) (Op, error) {
	var op Op
	data, err := os.ReadFile(info)
	if err != nil {
		return op, err
	}
	if err := json.Unmarshal(data, &op); err != nil || op.Name == "" {
		return Op{}, errors.New("no operation recorded")
	}
	return op, nil
}

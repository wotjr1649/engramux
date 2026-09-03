// Package schedule registers, removes and reads back the Task Scheduler entry
// that starts engramux-service at logon (spec 5.5).
//
// # Why schtasks and not the COM API
//
// ITaskService is the native route and it needs CGO or a hand-written COM
// vtable shim. Every shipped binary is CGO_ENABLED=0 (AGENTS.md), so the COM
// route would cost a boundary this project holds for a reason. schtasks.exe
// ships with Windows, takes and emits the same XML the COM API does, and is
// what a person would type to check the result by hand.
//
// The cost is that schtasks reports failure in the machine's display language.
// Nothing here parses its output: a non-zero exit from a query is
// [ErrNotRegistered] and the message is passed through verbatim for the human
// reading it, which is the whole reason [Unregister] asks before it deletes
// rather than interpreting a delete that failed.
//
// # The encoding is a measurement, not a preference
//
//   - schtasks /create /xml accepts a UTF-8 file whose declaration says
//     encoding="UTF-16".
//   - schtasks /query /xml emits exactly that shape back: the declaration says
//     UTF-16 and the bytes are UTF-8, with no BOM.
//
// Both were observed against a real registration. The declaration is written
// out because that is what round-trips, and it is why [parse] installs a
// CharsetReader: encoding/xml refuses a document declaring an encoding it does
// not know before it looks at a single byte, so xml.Unmarshal on the readback
// fails outright without one.
//
// # Absence means the default
//
// Windows normalises away any element whose value equals its default. A
// document that registers <RunLevel>LeastPrivilege</RunLevel> and
// <Enabled>true</Enabled> reads back with neither. So the parse fills those
// defaults in rather than reporting a gap, and a caller that treated absence as
// misconfiguration would report problems that do not exist. Every field of
// [Task] below says which of the two it is.
package schedule

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// TaskName is the registration a real install uses. Tests must not touch it.
//
// The leading backslash is the Task Scheduler Library root. schtasks accepts
// the name without it and echoes it back with it, so writing it here is what
// makes the name this package passes and the name Windows reports the same
// string.
const TaskName = `\Engramux`

// namespace is the Task Scheduler schema, and version is the schema version
// the elements below belong to.
const (
	namespace = "http://schemas.microsoft.com/windows/2004/02/mit/task"
	version   = "1.3"
)

var (
	// ErrNotRegistered means schtasks would not report on the task.
	//
	// Almost always that is because nothing has registered it. It can also
	// be a query that failed for another reason, and the two cannot be told
	// apart here: schtasks says which in the machine's display language, and
	// matching a localised string is worse than not matching one. The
	// wrapped error carries schtasks' own words for the person reading them.
	ErrNotRegistered = errors.New("schedule: no such scheduled task")

	errTaskName = errors.New("schedule: task name is not a plain Task Scheduler path")
)

// Task is one registered task, with the elements Windows normalised away
// filled back in.
type Task struct {
	// Command is Actions/Exec/Command - the binary the task runs.
	Command string
	// UserID is Principals/Principal/UserId, which reads back as a SID.
	// The trigger's own UserId is not this: Windows rewrites that one to
	// DOMAIN\user, so the two never compare equal.
	UserID string
	// LogonType is Principals/Principal/LogonType.
	LogonType string
	// RunLevel is Principals/Principal/RunLevel. Absent in the readback,
	// which means LeastPrivilege - the value this package registers.
	RunLevel string
	// HasLogonTrigger reports whether Triggers holds a LogonTrigger.
	HasLogonTrigger bool
	// Enabled is Settings/Enabled. Absent means true.
	Enabled bool
	// Hidden is Settings/Hidden. Absent means false.
	Hidden bool
	// DisallowStartIfOnBatteries is Settings/DisallowStartIfOnBatteries.
	// Absent means true, which is why this package writes false explicitly.
	DisallowStartIfOnBatteries bool
	// StopIfGoingOnBatteries is Settings/StopIfGoingOnBatteries. Absent
	// means true, for the same reason.
	StopIfGoingOnBatteries bool
	// ExecutionTimeLimit is Settings/ExecutionTimeLimit. Empty means the
	// element is absent, which spec 5.5 refuses to rely on.
	ExecutionTimeLimit string
	// MultipleInstancesPolicy is Settings/MultipleInstancesPolicy.
	MultipleInstancesPolicy string
	// RestartInterval and RestartCount are Settings/RestartOnFailure. An
	// empty interval means there is no restart policy at all.
	RestartInterval string
	RestartCount    int
}

// document is the XML both directions use: [Register] marshals it and [parse]
// unmarshals the readback into it.
//
// One struct rather than two because the two shapes differ only in what is
// absent, and absence is the thing this package has to get right. Pointers mark
// every element whose absence means something other than its zero value.
type document struct {
	XMLName          xml.Name `xml:"http://schemas.microsoft.com/windows/2004/02/mit/task Task"`
	Version          string   `xml:"version,attr"`
	RegistrationInfo struct {
		Description string `xml:"Description,omitempty"`
	} `xml:"RegistrationInfo"`
	Triggers struct {
		LogonTrigger *struct {
			UserID string `xml:"UserId,omitempty"`
		} `xml:"LogonTrigger"`
	} `xml:"Triggers"`
	Principals struct {
		Principal struct {
			ID        string `xml:"id,attr"`
			UserID    string `xml:"UserId,omitempty"`
			LogonType string `xml:"LogonType,omitempty"`
			RunLevel  string `xml:"RunLevel,omitempty"`
		} `xml:"Principal"`
	} `xml:"Principals"`
	Settings struct {
		MultipleInstancesPolicy    string `xml:"MultipleInstancesPolicy,omitempty"`
		DisallowStartIfOnBatteries *bool  `xml:"DisallowStartIfOnBatteries"`
		StopIfGoingOnBatteries     *bool  `xml:"StopIfGoingOnBatteries"`
		Hidden                     *bool  `xml:"Hidden"`
		Enabled                    *bool  `xml:"Enabled"`
		ExecutionTimeLimit         string `xml:"ExecutionTimeLimit,omitempty"`
		RestartOnFailure           *struct {
			Interval string `xml:"Interval"`
			Count    int    `xml:"Count"`
		} `xml:"RestartOnFailure"`
	} `xml:"Settings"`
	Actions struct {
		Context string `xml:"Context,attr"`
		Exec    struct {
			Command string `xml:"Command"`
		} `xml:"Exec"`
	} `xml:"Actions"`
}

// Register registers name to run exe at this user's logon, replacing any
// existing registration under that name.
//
// Replacing rather than refusing is spec 5.5's upgrade path: drain, stop,
// replace, start. A user who moves the binary and runs this again must not have
// to remove the old entry first.
//
// The principal is the interactive user, at LeastPrivilege. That matters most
// when the shell running this is itself elevated: the task must not inherit the
// elevation, and writing the principal out is what stops it.
func Register(ctx context.Context, name, exe string) error {
	if err := checkName(name); err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("schedule: current user: %w", err)
	}

	doc, err := render(exe, u.Uid)
	if err != nil {
		return err
	}

	// A directory of its own, removed on the way out. The document carries
	// the user's SID, which is not a secret (spec 2 puts one SID inside the
	// trust boundary) and is still not something to leave in %TEMP%.
	dir, err := os.MkdirTemp("", "engramux-schedule-")
	if err != nil {
		return fmt.Errorf("schedule: create a temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "task.xml")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		return fmt.Errorf("schedule: write %s: %w", path, err)
	}

	if out, err := run(ctx, "/create", "/xml", path, "/tn", name, "/f"); err != nil {
		return fmt.Errorf("schedule: register %s: %w: %.400s", name, err, out)
	}
	return nil
}

// Unregister removes name. A name that is not registered is not an error, so
// running it twice is not one either.
//
// The check is a query rather than a tolerated failure from the delete: a
// delete that failed because the task was absent and a delete that failed
// because it could not be removed exit the same way and differ only in a
// localised message. Swallowing the second to be idempotent about the first
// would report success for a task that is still there.
func Unregister(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if _, err := query(ctx, name); errors.Is(err, ErrNotRegistered) {
		return nil
	}
	if out, err := run(ctx, "/delete", "/tn", name, "/f"); err != nil {
		return fmt.Errorf("schedule: remove %s: %w: %.400s", name, err, out)
	}
	return nil
}

// Query reads name back from Windows.
//
// It answers what is registered, not what this package would have registered:
// every value comes off the running system, which is what makes it worth
// reporting at all. A task that is not registered returns [ErrNotRegistered].
func Query(ctx context.Context, name string) (Task, error) {
	if err := checkName(name); err != nil {
		return Task{}, err
	}
	raw, err := query(ctx, name)
	if err != nil {
		return Task{}, err
	}
	return parse(raw)
}

// query returns the readback document's bytes.
//
// Split from [Query] so that a test can assert on what Windows actually sent -
// specifically that the elements [parse] supplies defaults for are the ones
// Windows really left out. Without that, a default could stop being applied and
// nothing would notice.
func query(ctx context.Context, name string) ([]byte, error) {
	out, err := run(ctx, "/query", "/tn", name, "/xml")
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %.400s", ErrNotRegistered, name, out)
	}
	return out, nil
}

// parse turns a readback document into a [Task], filling in the defaults for
// the elements Windows normalises away.
func parse(raw []byte) (Task, error) {
	d := xml.NewDecoder(bytes.NewReader(raw))
	// The document declares UTF-16 and the bytes are UTF-8 (see the package
	// comment). encoding/xml rejects any declared encoding it does not know
	// unless this is set, so without it the readback does not decode at all.
	// Handing the reader straight back is correct precisely because the
	// declaration is not describing the bytes.
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }

	var doc document
	if err := d.Decode(&doc); err != nil {
		return Task{}, fmt.Errorf("schedule: decode the task registration: %w", err)
	}

	t := Task{
		Command:                 doc.Actions.Exec.Command,
		UserID:                  doc.Principals.Principal.UserID,
		LogonType:               doc.Principals.Principal.LogonType,
		HasLogonTrigger:         doc.Triggers.LogonTrigger != nil,
		ExecutionTimeLimit:      doc.Settings.ExecutionTimeLimit,
		MultipleInstancesPolicy: doc.Settings.MultipleInstancesPolicy,
		// Each of these five is "what Windows sent, or the default it
		// normalised away". LeastPrivilege and Enabled are the ones this
		// package writes and never gets back; the battery pair is the
		// opposite case, written as false precisely because their
		// default is true.
		RunLevel:                   or(doc.Principals.Principal.RunLevel, "LeastPrivilege"),
		Enabled:                    orBool(doc.Settings.Enabled, true),
		Hidden:                     orBool(doc.Settings.Hidden, false),
		DisallowStartIfOnBatteries: orBool(doc.Settings.DisallowStartIfOnBatteries, true),
		StopIfGoingOnBatteries:     orBool(doc.Settings.StopIfGoingOnBatteries, true),
	}
	if r := doc.Settings.RestartOnFailure; r != nil {
		t.RestartInterval, t.RestartCount = r.Interval, r.Count
	}
	return t, nil
}

// or returns s, or def when the element was absent.
func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// orBool returns *p, or def when the element was absent.
func orBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// render builds the registration document for exe running as sid.
//
// Everything spec 5.5 fixes is a literal here rather than a parameter. There is
// one registration shape and a caller choosing, say, its own
// MultipleInstancesPolicy would be choosing whether I-09's second half holds.
func render(exe, sid string) ([]byte, error) {
	var doc document
	doc.XMLName = xml.Name{Space: namespace, Local: "Task"}
	doc.Version = version
	doc.RegistrationInfo.Description =
		"Engramux captures Claude Code and Codex hook events. One service per Windows user."

	// The trigger names the user as well as the principal. Without a UserId
	// a logon trigger fires for every account on the machine, and I-01 is
	// one process per Windows user.
	doc.Triggers.LogonTrigger = &struct {
		UserID string `xml:"UserId,omitempty"`
	}{UserID: sid}

	doc.Principals.Principal.ID = "Author"
	doc.Principals.Principal.UserID = sid
	doc.Principals.Principal.LogonType = "InteractiveToken"
	// Written even though Windows drops it. It is the difference between
	// "this task is not elevated" and "nobody said", and the shell doing the
	// registering may well be elevated.
	doc.Principals.Principal.RunLevel = "LeastPrivilege"

	doc.Settings.MultipleInstancesPolicy = "IgnoreNew"
	doc.Settings.Hidden = ptr(true)
	doc.Settings.Enabled = ptr(true)
	// PT0S means run indefinitely. Microsoft documents the COM default as
	// stopping the task 72 hours after it starts and the XML element is
	// optional, documenting no default for absence - so omitting it leaves
	// the behaviour genuinely unresolved (spec 5.5).
	doc.Settings.ExecutionTimeLimit = "PT0S"
	// Both default to true, and both would take a logon-triggered service
	// down on a laptop: one stops it starting on battery, the other kills it
	// when the charger is unplugged.
	doc.Settings.DisallowStartIfOnBatteries = ptr(false)
	doc.Settings.StopIfGoingOnBatteries = ptr(false)
	doc.Settings.RestartOnFailure = &struct {
		Interval string `xml:"Interval"`
		Count    int    `xml:"Count"`
	}{Interval: "PT1M", Count: 3}

	doc.Actions.Context = "Author"
	doc.Actions.Exec.Command = exe

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("schedule: encode the task registration: %w", err)
	}
	// Declared UTF-16 over UTF-8 bytes, which is measured rather than
	// preferred - see the package comment.
	return append([]byte(`<?xml version="1.0" encoding="UTF-16"?>`+"\r\n"), body...), nil
}

func ptr[T any](v T) *T { return &v }

// run invokes schtasks and returns its combined output.
//
// Combined, because schtasks does not keep to one stream and the message is for
// a person either way. The bytes are in the console code page rather than
// UTF-8 on a localised machine, and they are passed through unchanged: they
// render correctly on the console that is going to print them, and re-encoding
// them would need to know a code page nothing here knows.
func run(ctx context.Context, args ...string) ([]byte, error) {
	// #nosec G204 -- args are this package's own literals plus a task name
	// checkName has restricted and a path this package just wrote. Nothing
	// goes through a shell: exec passes argv directly.
	cmd := exec.CommandContext(ctx, "schtasks.exe", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}
	return out, nil
}

// checkName restricts a task name to a plain Task Scheduler path.
//
// It is not a courtesy check. The name is an argv element handed to
// schtasks.exe, so a name beginning with "/" would be read as another flag -
// "/delete" is eight characters - and one containing a quote would not survive
// Windows' own argv round trip intact. Restricting the charset removes both
// shapes rather than blacklisting the characters that reach them today.
func checkName(name string) error {
	rest, ok := strings.CutPrefix(name, `\`)
	if !ok || rest == "" {
		return fmt.Errorf("%w: %.64q", errTaskName, name)
	}
	for _, r := range rest {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '\\', r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("%w: %.64q", errTaskName, name)
		}
	}
	return nil
}

// End asks Windows to stop the task's running instance, and a task that is not
// registered or not running is not an error.
//
// # It returns before the process is gone, and that is the whole trap
//
// `schtasks /end` requests a stop and returns; the process is still exiting.
// The row AGENTS.md carries is what happens to a caller that believes it: an
// `/end` followed straight by a [Run] leaves **nothing** running, because the
// new instance loses the pipe race to the one still dying (I-09 working) and
// then the old one finishes dying too. What is left is no service at all and a
// log line that reads like a singleton conflict rather than like an empty
// machine.
//
// So this package deliberately does not offer a stop-and-wait. Waiting needs to
// observe the thing the caller actually cares about, and that is not a task
// state - a service that has stopped answering its pipe is the only evidence
// that matters, and this package cannot see a pipe. [cmd/engramux]'s update
// command is where the wait lives, against `status`.
//
// Polling [Query] for a state is not the wait either: a task that has only just
// been asked to stop still reads as running, and one that has stopped reads the
// same whether its process died cleanly or was never up.
func End(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if _, err := query(ctx, name); errors.Is(err, ErrNotRegistered) {
		return nil
	}
	// A task with no running instance answers with an error and a message
	// saying so, which is not a failure to stop it - it is already stopped.
	// The exit code does not distinguish that from a real refusal, so the
	// caller gets it back and decides; what makes that safe is that the
	// caller's own wait is what establishes the service is down.
	if out, err := run(ctx, "/end", "/tn", name); err != nil {
		return fmt.Errorf("schedule: end %s: %w: %.400s", name, err, out)
	}
	return nil
}

// Run starts the task now, without waiting for its trigger.
//
// It is what an installation uses to bring the service up: starting it through
// the task means the running process is the one the machine will have after a
// logon, rather than a bare child of the installer that no logon reproduces.
//
// # Two things it does not do, and one of them is a trap
//
// It does not wait. `schtasks /run` returns as soon as the request is accepted,
// so a caller that needs the service to be *up* has to observe that separately -
// [Query]'s last result is not it either, since a task that has only just been
// asked to run has not produced one yet. The installer waits for the endpoint
// file instead, which is the thing it actually needs.
//
// It does not stop anything first, and must not. A `/end` followed by a `/run`
// leaves nothing running: `/end` returns before the process is gone, the new
// instance loses the pipe race to the old one and exits with an access denial
// on the name (which is I-09 working), and then the old one finishes dying.
// AGENTS.md carries that row. Running while one is already up is harmless for
// the same reason - the second instance loses the same race and exits - so this
// is safe to call on a machine that is already running one.
func Run(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if out, err := run(ctx, "/run", "/tn", name); err != nil {
		return fmt.Errorf("schedule: run %s: %w: %.400s", name, err, out)
	}
	return nil
}

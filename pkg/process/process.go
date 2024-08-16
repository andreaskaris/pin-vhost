package process

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strconv"

	"golang.org/x/sys/unix"
	"k8s.io/utils/cpuset"
)

const (
	statusFile                      = "status"
	pinModeNumericRegularExpression = `^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$`
	pinModeRegularExpression        = `^first$|^last$|` + pinModeNumericRegularExpression
	singleCPURegularExpression      = `^[0-9]+$`
	firstCPURegularExpression       = `^([0-9]+)[^0-9]`
	lastCPURegularExpression        = `[^0-9]([0-9]+)$`
)

var (
	pinModeRegex        = regexp.MustCompile(pinModeRegularExpression)
	pinModeNumericRegex = regexp.MustCompile(pinModeNumericRegularExpression)
	singleCPURegex      = regexp.MustCompile(singleCPURegularExpression)
	firstCPURegex       = regexp.MustCompile(firstCPURegularExpression)
	lastCPURegex        = regexp.MustCompile(lastCPURegularExpression)
)

// Instance represents a process instance.
type Instance struct {
	procDirectory       string
	pinMode             string
	discoveryMode       bool
	procNameFilterRegex *regexp.Regexp
}

// New returns a new process instance.
func New(discoveryMode bool, pinMode, procNameFilterRegularExpression string) (*Instance, error) {
	if !discoveryMode {
		if err := validatePinMode(pinMode); err != nil {
			return nil, fmt.Errorf("must provide a valid pin-mode when discovery mode is off, %q", err)
		}
	} else {
		if pinMode != "" {
			return nil, fmt.Errorf("cannot provide a pin-mode in discovery-mode")
		}
	}

	return &Instance{
		procDirectory:       getProcDirectory(),
		pinMode:             pinMode,
		discoveryMode:       discoveryMode,
		procNameFilterRegex: regexp.MustCompile(procNameFilterRegularExpression),
	}, nil
}

// GetProcDirectory is a getter for procDirectory.
func (i *Instance) GetProcDirectory() string {
	return i.procDirectory
}

// PinAll scans directory /proc or /host/proc for processes that match procNameFilterRegex. Matching processes are
// pinned to pinMode, or their details are printed if in discovery mode.
func (i *Instance) PinAll() {
	entries, err := os.ReadDir(i.procDirectory)
	if err != nil {
		log.Fatalf("Could not read from proc directory %q, err: %q", i.procDirectory, err)
	}
	var pid int
	for _, entry := range entries {
		if entry.IsDir() {
			if pid, err = strconv.Atoi(entry.Name()); err == nil {
				if err := i.Pin(pid); err != nil {
					log.Printf("Warning: %s", err)
				}
			}
		}
	}
}

// Pin pins a process with pid. It retrieves the process name and CPUs allowed list from /proc/<pid>/status. Then, it
// checks if the process name matches the process name filter regex. If discovery mode is configured, only print values.
// Otherwise, pin the process.
func (i *Instance) Pin(pid int) error {
	// Get process attributes. If we can't, skip (directory for this process might be missing [process killed?]).
	procName, procCPUsAllowedList, err := i.getProcessAttributes(pid)
	if err != nil {
		return err
	}
	// If the process name does not match the user provided filter, skip.
	if !i.procNameFilterRegex.Match(procName) {
		return nil
	}
	// If this is discovery mode, only print the process attributes, but do not pin.
	if i.discoveryMode {
		log.Printf("PID: %d, Name: %s, cpus_allowed_list: %s\n", pid, procName, procCPUsAllowedList)
		return nil
	}
	// If this is not discovery mode, pin the process.
	if err := pinProcess(pid, procCPUsAllowedList, i.pinMode); err != nil {
		return fmt.Errorf("could not pin process; PID: %d, Name: %s, PinMode: %s, err: %q",
			pid, procName, i.pinMode, err)
	}
	return nil
}

// getProcessAttributes takes the process pid, parses file /proc/<pid>/status and returns the process name and the
// Cpus_allowed_list if the file exists. It returns an error if file /proc/<pid>/status does not exist.
func (i *Instance) getProcessAttributes(pid int) ([]byte, []byte, error) {
	var procName []byte
	var procCPUsAllowedList []byte

	f, err := os.Open(getStatusFile(i.procDirectory, pid))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.Equal(line[:5], []byte{'N', 'a', 'm', 'e', ':'}) {
			procName = bytes.TrimSpace(line[5:])
		}
		if bytes.Equal(line[:18],
			[]byte{'C', 'p', 'u', 's', '_',
				'a', 'l', 'l', 'o', 'w', 'e', 'd', '_',
				'l', 'i', 's', 't', ':'}) {
			procCPUsAllowedList = bytes.TrimSpace(line[18:])
		}
		if procName != nil && procCPUsAllowedList != nil {
			return procName, procCPUsAllowedList, nil
		}
	}
	return nil, nil, fmt.Errorf("no file name found for pid %d", pid)
}

/* Helpers */

// getProcDirectory checks if /host/proc exists, in that case we'll be looking there for process information (important
// for containerization).
func getProcDirectory() string {
	procDirectory := "/proc"
	hostProc := path.Join("/host", procDirectory)
	if _, err := os.Stat(hostProc); err == nil {
		procDirectory = hostProc
	}
	return procDirectory
}

// validatePinMode returns true if the pin mode is supported.
func validatePinMode(pinMode string) error {
	if !pinModeRegex.MatchString(pinMode) {
		return fmt.Errorf("invalid pin-mode %q", pinMode)
	}
	return nil
}

// Join /proc/<pid>/status.
func getStatusFile(procDirectory string, pid int) string {
	return path.Join(procDirectory, fmt.Sprintf("%d", pid), statusFile)
}

// pinProcess takes the process ID, the current Cpus_allowed_list of the process, as well as the user provided
// pinMode. The pinMode can be first (first CPU of current process' Cpus_allowed_list), last (last CPU of current
// process' Cpus_allowed_list) or an explicit new Cpus_allowed_list for the process. An error indicates a parsin issue,
// or that the CPU affinity could not be set for the process.
func pinProcess(pid int, currentProcCPUsAllowedList []byte, pinMode string) error {
	newProcCPUsAllowedList, needsApply, err := getPinSet(currentProcCPUsAllowedList, pinMode)
	if err != nil {
		return err
	}
	if !needsApply {
		return nil
	}

	var cpuMask unix.CPUSet
	cpus, err := cpuset.Parse(newProcCPUsAllowedList)
	if err != nil {
		return err
	}
	for _, cpu := range cpus.List() {
		cpuMask.Set(cpu)
	}
	log.Printf("Pinning pid %d. Configured pin-mode: %q. Currrent cpus_allowed_list: %q. New CPU set: %q",
		pid, pinMode, currentProcCPUsAllowedList, newProcCPUsAllowedList)
	return unix.SchedSetaffinity(pid, &cpuMask)
}

// getPinSet takes the currentProcCPUsAllowedList and the new pinMode. The pinMode can be first (first CPU of current
// process' Cpus_allowed_list), last (last CPU of current process' Cpus_allowed_list) or an explicit new
// Cpus_allowed_list for the process. An error indicates that an invalid pinMode was provides, or that
// currentProcCPUsAllowedList could not be parsed. The boolean return value indicates if the change must be applied (true),
// or if nothing changed and the change need not be applied (false).
func getPinSet(currentProcCPUsAllowedList []byte, pinMode string) (string, bool, error) {
	// If pinMode is numeric, return that one.
	// Todo: if a numeric expression e.g. 2-3 or 2,3 is used, we currently reapply every time.
	if pinModeNumericRegex.MatchString(pinMode) {
		return pinMode, true, nil
	}
	// Otherwise, for both 'first' or 'last', return the current affinity if it contains only a single CPU.
	// In this case, there is no change, so signal that nothing new needs to be applied.
	if singleCPURegex.Match(currentProcCPUsAllowedList) {
		return string(currentProcCPUsAllowedList), false, nil
	}
	// Otherwise, if pinMode is 'first', extract the first CPU.
	if pinMode == "first" {
		sMatches := firstCPURegex.FindSubmatch(currentProcCPUsAllowedList)
		if len(sMatches) != 2 {
			return "", false, fmt.Errorf("pinMode 'first' could not find a valid match in currentProcCPUsAllowedList %q",
				currentProcCPUsAllowedList)
		}
		return string(sMatches[1]), true, nil
	}
	// Respectively, if it's last, return the last CPU.
	if pinMode == "last" {
		sMatches := lastCPURegex.FindSubmatch(currentProcCPUsAllowedList)
		if len(sMatches) != 2 {
			return "", false, fmt.Errorf("pinMode 'first' could not find a valid match in currentProcCPUsAllowedList %q",
				currentProcCPUsAllowedList)
		}
		return string(sMatches[1]), true, nil
	}
	// Finally, if none of the above matches, return an error.
	return "", false, fmt.Errorf("getPinSet was provided with invalid pinMode: %q", pinMode) // Should never happen.
}

//go:build windows

package testenv

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const pseudoConsoleWaitTimeout = 0x00000102

// RunPseudoConsole owns an interactive Windows child in a ConPTY. It waits
// until the requested UI identity is rendered, sends q, and stores the raw
// terminal transcript in the scenario evidence directory.
func (s *Scenario) RunPseudoConsole(command *exec.Cmd, name, readyMarker string) (transcript []byte, resultErr error) {
	return s.RunPseudoConsoleSteps(command, name, readyMarker)
}

func (s *Scenario) RunPseudoConsoleSteps(command *exec.Cmd, name, readyMarker string, steps ...ConsoleStep) (transcript []byte, resultErr error) {
	s.t.Helper()
	if command == nil || command.Path == "" || len(command.Args) == 0 {
		return nil, errors.New("interactive console command is incomplete")
	}
	logFile, err := os.Create(filepath.Join(s.Output, name+".log"))
	if err != nil {
		return nil, err
	}
	s.files = append(s.files, logFile)

	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	if err := windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	defer closeWindowsHandle(&inputRead)
	defer closeWindowsHandle(&inputWrite)
	if err := windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY output pipe: %w", err)
	}
	defer closeWindowsHandle(&outputRead)
	defer closeWindowsHandle(&outputWrite)

	var pseudoConsole windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: 300, Y: 60}, inputRead, outputWrite, 0, &pseudoConsole); err != nil {
		return nil, fmt.Errorf("create Windows pseudo console: %w", err)
	}
	defer func() {
		if pseudoConsole != 0 {
			windows.ClosePseudoConsole(pseudoConsole)
		}
	}()

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("allocate ConPTY process attributes: %w", err)
	}
	defer attributes.Delete()
	// The pseudoconsole attribute value is the HPCON handle itself, represented
	// as an unsafe.Pointer without converting directly from uintptr.
	pseudoConsoleValue := *(*unsafe.Pointer)(unsafe.Pointer(&pseudoConsole))
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, pseudoConsoleValue, unsafe.Sizeof(pseudoConsole)); err != nil {
		return nil, fmt.Errorf("bind pseudo console to process: %w", err)
	}
	startup := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			// Null standard handles keep console output on the ConPTY channels.
			Flags: windows.STARTF_USESTDHANDLES,
		},
		ProcThreadAttributeList: attributes.List(),
	}
	applicationName, err := windows.UTF16PtrFromString(command.Path)
	if err != nil {
		return nil, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(command.Args))
	if err != nil {
		return nil, err
	}
	workingDirectory := command.Dir
	if workingDirectory == "" {
		workingDirectory, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	workingDirectoryPtr, err := windows.UTF16PtrFromString(workingDirectory)
	if err != nil {
		return nil, err
	}
	env := command.Env
	if env == nil {
		env = os.Environ()
	}
	environmentBlock, err := pseudoConsoleEnvironment(env)
	if err != nil {
		return nil, err
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create ConPTY process job: %w", err)
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, fmt.Errorf("configure ConPTY process job: %w", err)
	}
	processInfo := windows.ProcessInformation{}
	defer func() {
		if processInfo.Process != 0 {
			if resultErr != nil {
				resultErr = errors.Join(resultErr, windows.TerminateJobObject(job, 1))
				_, waitErr := windows.WaitForSingleObject(processInfo.Process, 5000)
				if waitErr != nil {
					resultErr = errors.Join(resultErr, waitErr)
				}
			}
			windows.CloseHandle(processInfo.Process)
		}
		if processInfo.Thread != 0 {
			windows.CloseHandle(processInfo.Thread)
		}
	}()
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	if err := windows.CreateProcess(applicationName, commandLine, nil, nil, false, flags, &environmentBlock[0], workingDirectoryPtr, &startup.StartupInfo, &processInfo); err != nil {
		return nil, fmt.Errorf("start interactive console: %w", err)
	}
	_ = closeWindowsHandle(&inputRead)
	_ = closeWindowsHandle(&outputWrite)
	if err := windows.AssignProcessToJobObject(job, processInfo.Process); err != nil {
		return nil, fmt.Errorf("assign interactive console job: %w", err)
	}
	s.register("interactive-console", command.Path, int(processInfo.ProcessId))
	if _, err := windows.ResumeThread(processInfo.Thread); err != nil {
		return nil, fmt.Errorf("resume interactive console: %w", err)
	}
	_ = windows.CloseHandle(processInfo.Thread)
	processInfo.Thread = 0

	reader := os.NewFile(uintptr(outputRead), name+"-conpty-output")
	outputRead = 0
	var output bytes.Buffer
	var outputMu sync.Mutex
	readDone := make(chan error, 1)
	go func() {
		defer reader.Close()
		buffer := make([]byte, 8192)
		for {
			count, readErr := reader.Read(buffer)
			if count > 0 {
				if _, writeErr := logFile.Write(buffer[:count]); writeErr != nil {
					readDone <- writeErr
					return
				}
				outputMu.Lock()
				output.Write(buffer[:count])
				outputMu.Unlock()
			}
			if readErr != nil {
				readDone <- readErr
				return
			}
		}
	}()

	readyDeadline := time.Now().Add(20 * time.Second)
	readerFinished := false
	for time.Now().Before(readyDeadline) {
		outputMu.Lock()
		ready := strings.Contains(output.String(), readyMarker)
		outputMu.Unlock()
		if ready {
			break
		}
		select {
		case <-readDone:
			readerFinished = true
			transcript = currentTranscript(&outputMu, &output)
			return transcript, fmt.Errorf("interactive console exited before rendering %q", readyMarker)
		case <-time.After(50 * time.Millisecond):
		}
	}
	outputMu.Lock()
	ready := strings.Contains(output.String(), readyMarker)
	outputMu.Unlock()
	if !ready {
		transcript = currentTranscript(&outputMu, &output)
		return transcript, fmt.Errorf("interactive console did not render %q before timeout", readyMarker)
	}
	for _, step := range steps {
		if err := windows.WriteFile(inputWrite, []byte(step.Input), nil, nil); err != nil {
			return currentTranscript(&outputMu, &output), err
		}
		timeout := step.Timeout
		if timeout <= 0 {
			timeout = 20 * time.Second
		}
		deadline := time.Now().Add(timeout)
		for step.WaitFor != "" {
			outputMu.Lock()
			found := strings.Contains(output.String(), step.WaitFor)
			outputMu.Unlock()
			if found {
				break
			}
			if time.Now().After(deadline) {
				return currentTranscript(&outputMu, &output), fmt.Errorf("console did not render %q", step.WaitFor)
			}
			select {
			case <-s.Context.Done():
				return currentTranscript(&outputMu, &output), s.Context.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	if err := windows.WriteFile(inputWrite, []byte("q"), nil, nil); err != nil {
		transcript = currentTranscript(&outputMu, &output)
		return transcript, fmt.Errorf("send console quit key: %w", err)
	}
	waitResult, waitErr := windows.WaitForSingleObject(processInfo.Process, 15000)
	if waitErr != nil || waitResult != windows.WAIT_OBJECT_0 {
		_ = windows.WriteFile(inputWrite, []byte{3}, nil, nil)
		waitResult, waitErr = windows.WaitForSingleObject(processInfo.Process, 5000)
	}
	if waitErr != nil || waitResult != windows.WAIT_OBJECT_0 {
		transcript = currentTranscript(&outputMu, &output)
		return transcript, fmt.Errorf("interactive console did not exit after q: wait=%d err=%v", waitResult, waitErr)
	}
	windows.ClosePseudoConsole(pseudoConsole)
	pseudoConsole = 0
	if !readerFinished {
		select {
		case <-readDone:
			readerFinished = true
		case <-time.After(2 * time.Second):
			transcript = currentTranscript(&outputMu, &output)
			return transcript, errors.New("interactive console transcript did not close")
		}
	}
	transcript = currentTranscript(&outputMu, &output)
	if !strings.Contains(string(transcript), readyMarker) {
		return transcript, fmt.Errorf("interactive console transcript omitted %q", readyMarker)
	}
	if err := logFile.Sync(); err != nil {
		return transcript, err
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(processInfo.Process, &exitCode); err != nil {
		return transcript, err
	}
	if exitCode != 0 {
		return transcript, fmt.Errorf("interactive console exited with code %d", exitCode)
	}
	return transcript, nil
}

func pseudoConsoleEnvironment(values []string) ([]uint16, error) {
	copyValues := append([]string(nil), values...)
	for _, value := range copyValues {
		if strings.ContainsRune(value, '\x00') {
			return nil, errors.New("console environment contains NUL")
		}
	}
	sort.Slice(copyValues, func(i, j int) bool {
		return strings.ToUpper(environmentName(copyValues[i])) < strings.ToUpper(environmentName(copyValues[j]))
	})
	var result []uint16
	for _, value := range copyValues {
		result = append(result, utf16.Encode([]rune(value+"\x00"))...)
	}
	return append(result, 0), nil
}

func environmentName(value string) string {
	index := strings.IndexByte(value, '=')
	if index == 0 {
		if next := strings.IndexByte(value[1:], '='); next >= 0 {
			index = next + 1
		}
	}
	if index < 0 {
		return value
	}
	return value[:index]
}

func closeWindowsHandle(handle *windows.Handle) error {
	if *handle == 0 {
		return nil
	}
	err := windows.CloseHandle(*handle)
	*handle = 0
	return err
}

func currentTranscript(lock *sync.Mutex, output *bytes.Buffer) []byte {
	lock.Lock()
	defer lock.Unlock()
	return append([]byte(nil), output.Bytes()...)
}

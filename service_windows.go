//go:build windows

package main

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/kardianos/service"

	"golang.org/x/sys/windows"
)

/*
	========================
	Kernel32 bindings
	========================
*/

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSetInformationProcess = kernel32.NewProc("SetInformationProcess")
)

/*
	========================
	Process Information Classes
	========================
*/

const (
	ProcessMemoryPriority    = 33
	ProcessIoPriority        = 33
	ProcessPowerThrottling   = 4
)

/*
	========================
	IO priority constants
	========================
*/

const (
	IoPriorityVeryLow = 0
)

/*
	========================
	Memory priority struct
	========================
*/

type memoryPriorityInformation struct {
	MemoryPriority uint32
}

/*
	========================
	EcoQoS struct
	========================
*/

type processPowerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

const (
	POWER_THROTTLING_EXECUTION_SPEED = 0x1
)

/*
	========================
	Helpers
	========================
*/

func setProcessPriority() error {
	return windows.SetPriorityClass(
		windows.CurrentProcess(),
		windows.IDLE_PRIORITY_CLASS,
	)
}

func setIOPriorityLow() error {
	h := windows.CurrentProcess()

	// Windows expects ProcessInformationClass = 33 for IO priority
	var ioPriority uint32 = IoPriorityVeryLow

	r1, _, err := procSetInformationProcess.Call(
		uintptr(h),
		uintptr(ProcessIoPriority),
		uintptr(unsafe.Pointer(&ioPriority)),
		unsafe.Sizeof(ioPriority),
	)

	if r1 == 0 {
		return fmt.Errorf("SetInformationProcess(IO_PRIORITY) failed: %w", err)
	}
	return nil
}

func setMemoryPriorityLow() error {
	h := windows.CurrentProcess()

	info := memoryPriorityInformation{
		MemoryPriority: 1, // lowest
	}

	r1, _, err := procSetInformationProcess.Call(
		uintptr(h),
		uintptr(ProcessMemoryPriority),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)

	if r1 == 0 {
		return fmt.Errorf("SetInformationProcess(MEMORY_PRIORITY) failed: %w", err)
	}
	return nil
}

func enableEfficiencyMode() error {
	h := windows.CurrentProcess()

	state := processPowerThrottlingState{
		Version:     1,
		ControlMask: POWER_THROTTLING_EXECUTION_SPEED,
		StateMask:   POWER_THROTTLING_EXECUTION_SPEED,
	}

	r1, _, err := procSetInformationProcess.Call(
		uintptr(h),
		uintptr(ProcessPowerThrottling),
		uintptr(unsafe.Pointer(&state)),
		unsafe.Sizeof(state),
	)

	if r1 == 0 {
		return fmt.Errorf("SetInformationProcess(POWER_THROTTLING) failed: %w", err)
	}
	return nil
}

/*
	========================
	Main API
	========================
*/

func setLowestSystemImpact() error {
	if err := setProcessPriority(); err != nil {
		return err
	}
	if err := setIOPriorityLow(); err != nil {
		return err
	}
	if err := setMemoryPriorityLow(); err != nil {
		return err
	}
	if err := enableEfficiencyMode(); err != nil {
		return err
	}
	return nil
}

func serviceAction(srvAction *string) error {
	cfg := &service.Config{
		Name:        "template_hosts",
		DisplayName: "template_hosts",
		Description: "templates the hosts file",
	}

	s, err := service.New(&program{}, cfg)
	if err != nil {
		return err
	}

	if *srvAction == "main" {
		return s.Run()
	}
	return service.Control(s, *srvAction)
}

type program struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	p.ctx, p.cancel = context.WithCancel(context.Background())
	go p.run()
	return nil
}
func (p *program) run() {
	err := setLowestSystemImpact()
	if err != nil {
		fmt.Println(err)
	}

	start(p.ctx)
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func srvMain() error {
	s := "main"
	return serviceAction(&s)

}


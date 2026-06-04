//go:build windows

package main

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/kardianos/service"

	"golang.org/x/sys/windows"
)

var (
	ntdll = windows.NewLazySystemDLL("ntdll.dll")

	procNtSetInformationProcess = ntdll.NewProc("NtSetInformationProcess")
)

/*
	========================
	Process Information Classes
	(from Windows internals)
	========================
*/

const (
	ProcessMemoryPriority = 39
	ProcessIoPriority     = 33
	ProcessPowerThrottling = 4
)

/*
	========================
	IO Priority
	========================
*/

type processIoPriority uint32

const (
	IoPriorityVeryLow processIoPriority = 0
	IoPriorityLow     processIoPriority = 1
	IoPriorityNormal  processIoPriority = 2
)

/*
	========================
	Memory Priority
	========================
*/

type memoryPriorityInformation struct {
	MemoryPriority uint32
}

/*
	========================
	EcoQoS / Power throttling
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

func ntSetProcessInfo(class uint32, data unsafe.Pointer, size uintptr) error {
	h := windows.CurrentProcess()

	r1, _, err := procNtSetInformationProcess.Call(
		uintptr(h),
		uintptr(class),
		uintptr(data),
		size,
	)

	// NTSTATUS success = 0
	if r1 != 0 {
		return fmt.Errorf("NtSetInformationProcess failed: %v (ntstatus=%x)", err, r1)
	}
	return nil
}

/*
	========================
	Features
	========================
*/

func setLowIO() error {
	io := IoPriorityVeryLow
	return ntSetProcessInfo(
		ProcessIoPriority,
		unsafe.Pointer(&io),
		unsafe.Sizeof(io),
	)
}

func setLowMemory() error {
	info := memoryPriorityInformation{
		MemoryPriority: 1,
	}
	return ntSetProcessInfo(
		ProcessMemoryPriority,
		unsafe.Pointer(&info),
		unsafe.Sizeof(info),
	)
}

func setEfficiencyMode() error {
	state := processPowerThrottlingState{
		Version:     1,
		ControlMask: POWER_THROTTLING_EXECUTION_SPEED,
		StateMask:   POWER_THROTTLING_EXECUTION_SPEED,
	}

	// NOTE: This one is still via NtSetInformationProcess on modern builds
	return ntSetProcessInfo(
		ProcessPowerThrottling,
		unsafe.Pointer(&state),
		unsafe.Sizeof(state),
	)
}

func setLowestSystemImpact() error {
	if err := windows.SetPriorityClass(windows.CurrentProcess(), windows.IDLE_PRIORITY_CLASS); err != nil {
		return err
	}
	if err := setLowIO(); err != nil {
		return err
	}
	if err := setLowMemory(); err != nil {
		return err
	}
	if err := setEfficiencyMode(); err != nil {
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


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
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSetProcessInformation = kernel32.NewProc("SetProcessInformation")
)

const (
	// From the documented PROCESS_INFORMATION_CLASS order:
	// ProcessMemoryPriority = 0
	// ProcessPowerThrottling = 4
	processMemoryPriorityClass  = 0
	processPowerThrottlingClass = 4
)

const (
	memoryPriorityVeryLow = 1

	processPowerThrottlingCurrentVersion = 1
	processPowerThrottlingExecutionSpeed  = 0x1
)

type memoryPriorityInformation struct {
	MemoryPriority uint32
}

type processPowerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

func setProcessInformation(class uint32, info unsafe.Pointer, size uintptr) error {
	h := windows.CurrentProcess()

	r1, _, err := procSetProcessInformation.Call(
		uintptr(h),
		uintptr(class),
		uintptr(info),
		size,
	)

	if r1 == 0 {
		return fmt.Errorf("SetProcessInformation failed: %w", err)
	}
	return nil
}

func setLowestPriorityClass() error {
	return windows.SetPriorityClass(windows.CurrentProcess(), windows.IDLE_PRIORITY_CLASS)
}

func setLowestMemoryPriority() error {
	info := memoryPriorityInformation{
		MemoryPriority: memoryPriorityVeryLow,
	}
	return setProcessInformation(
		processMemoryPriorityClass,
		unsafe.Pointer(&info),
		unsafe.Sizeof(info),
	)
}

func enableEfficiencyMode() error {
	info := processPowerThrottlingState{
		Version:     processPowerThrottlingCurrentVersion,
		ControlMask: processPowerThrottlingExecutionSpeed,
		StateMask:   processPowerThrottlingExecutionSpeed,
	}
	return setProcessInformation(
		processPowerThrottlingClass,
		unsafe.Pointer(&info),
		unsafe.Sizeof(info),
	)
}

func setLowestSystemImpact() error {
	if err := setLowestPriorityClass(); err != nil {
		return err
	}
	if err := setLowestMemoryPriority(); err != nil {
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


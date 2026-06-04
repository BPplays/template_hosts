//go:build windows

package main

import (
	"context"
	"fmt"

	"github.com/kardianos/service"

	"golang.org/x/sys/windows"
)

func setLowestPriority() error {
	h := windows.CurrentProcess()

	return windows.SetPriorityClass(h, windows.IDLE_PRIORITY_CLASS)
}

func makeService(srvAction *string) error {
	cfg := &service.Config{
		Name:        "template_hosts",
		DisplayName: "template_hosts",
		Description: "templates the hosts file",
	}

	s, err := service.New(&program{}, cfg)
	if err != nil {
		return err
	}

	return service.Control(s, *srvAction)
}

type program struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}
func (p *program) run() {
	err := setLowestPriority()
	if err != nil {
		fmt.Println(err)
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	start(p.ctx)
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}


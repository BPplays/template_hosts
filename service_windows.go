//go:build windows

package main

import (

	"github.com/kardianos/service"
)

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

type program struct{}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}
func (p *program) run() {
	main()
}

func (p *program) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	return nil
}


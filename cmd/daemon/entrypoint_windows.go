//go:build windows

package main

import (
	"context"

	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "TunnerseDaemon"

func runEntrypoint() error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if isService {
		return svc.Run(windowsServiceName, &windowsService{})
	}
	return runServer(context.Background())
}

type windowsService struct{}

func (s *windowsService) Execute(args []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)

	changes <- svc.Status{State: svc.StartPending}
	go func() {
		errCh <- runServer(ctx)
	}()

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				changes <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-errCh; err != nil {
					return false, 1
				}
				return false, 0
			default:
			}
		case err := <-errCh:
			if err != nil {
				return false, 1
			}
			return false, 0
		}
	}
}

package main

import (
	"context"
	"errors"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/ipc"
)

// App is the Wails backend for the zwan desktop client. It is a thin control
// surface over the SYSTEM engine service, reached via a named pipe (client/ipc).
type App struct {
	ctx context.Context
}

// NewApp creates the App.
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// ServiceUp reports whether the engine service is reachable.
func (a *App) ServiceUp() bool {
	_, err := ipc.Status()
	return err == nil
}

// Connect asks the service to join and bring up a network.
func (a *App) Connect(server, token, name string, useRelay bool) (*engine.Status, error) {
	resp, err := ipc.Connect(ipc.ConnectArgs{Server: server, Token: token, Name: name, UseRelay: useRelay})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return resp.Status, errors.New(resp.Error)
	}
	return resp.Status, nil
}

// Disconnect tears the current connection down.
func (a *App) Disconnect() (*engine.Status, error) {
	resp, err := ipc.Disconnect()
	if err != nil {
		return nil, err
	}
	return resp.Status, nil
}

// Status returns the current connection status from the service.
func (a *App) Status() (*engine.Status, error) {
	resp, err := ipc.Status()
	if err != nil {
		return nil, err
	}
	return resp.Status, nil
}
